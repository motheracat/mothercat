const maxJsonBodyBytes = 16 * 1024;
const maxImportBodyBytes = 5 * 1024 * 1024 + 64 * 1024;
const hopByHopHeaders = new Set([
  "connection",
  "content-encoding",
  "content-length",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
]);

function isSameOriginRequest(request: Request): boolean {
  const rawOrigin = request.headers.get("origin");
  const host = request.headers.get("host");
  if (!rawOrigin || !host) return false;

  let origin: URL;
  try {
    origin = new URL(rawOrigin);
  } catch {
    return false;
  }

  const requestProtocol = new URL(request.url).protocol.slice(0, -1);
  return (
    origin.host.toLowerCase() === host.toLowerCase() &&
    origin.protocol === `${requestProtocol}:` &&
    (origin.pathname === "/" || origin.pathname === "") &&
    origin.search === "" &&
    origin.hash === "" &&
    origin.username === "" &&
    origin.password === ""
  );
}

async function readBoundedBody(request: Request, limit: number): Promise<Uint8Array<ArrayBuffer> | null> {
  if (!request.body) return new Uint8Array(new ArrayBuffer(0));

  const declaredLength = request.headers.get("content-length");
  if (declaredLength !== null) {
    const parsedLength = Number(declaredLength);
    if (!Number.isSafeInteger(parsedLength) || parsedLength < 0) {
      throw new TypeError("Invalid request content length.");
    }
    if (parsedLength > limit) return null;
  }

  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > limit) {
        await reader.cancel();
        return null;
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }

  const body = new Uint8Array(new ArrayBuffer(size));
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return body;
}

function errorResponse(status: number, code: string): Response {
  return Response.json(
    { error: { code, request_id: crypto.randomUUID() } },
    { status, headers: { "Cache-Control": "no-store" } },
  );
}

export async function proxyApiRequest(
  request: Request,
  pathSegments: string[],
  apiBaseUrl: string,
): Promise<Response> {
  const method = request.method.toUpperCase();
  if (!["GET", "POST", "PATCH"].includes(method)) {
    return errorResponse(405, "method_not_allowed");
  }
  if (pathSegments.length === 0 || pathSegments.some((segment) => segment === "")) {
    return errorResponse(404, "not_found");
  }
  if (pathSegments.some((segment) => !/^[a-zA-Z0-9_-]+$/.test(segment))) {
    return errorResponse(400, "invalid_api_path");
  }

  const isMutation = method !== "GET";
  if (isMutation && !isSameOriginRequest(request)) {
    return errorResponse(403, "invalid_origin");
  }

  let upstreamOrigin: URL;
  try {
    upstreamOrigin = new URL(apiBaseUrl);
  } catch {
    return errorResponse(503, "upstream_unavailable");
  }
  if (
    (upstreamOrigin.protocol !== "https:" && upstreamOrigin.hostname !== "localhost" && upstreamOrigin.hostname !== "127.0.0.1") ||
    upstreamOrigin.username !== "" ||
    upstreamOrigin.password !== ""
  ) {
    return errorResponse(503, "upstream_unavailable");
  }

  const upstreamUrl = new URL(
    `/api/v1/${pathSegments.map((segment) => encodeURIComponent(segment)).join("/")}`,
    upstreamOrigin,
  );
  upstreamUrl.search = new URL(request.url).search;

  const headers = new Headers();
  const accept = request.headers.get("accept");
  const contentType = request.headers.get("content-type");
  const cookieHeader = request.headers.get("cookie");
  const sessionCookies = cookieHeader?.split(";")
    .map((cookie) => cookie.trim())
    .filter((cookie) => cookie.startsWith("mothercat_session=")) ?? [];
  if (accept) headers.set("accept", accept);
  if (contentType) headers.set("content-type", contentType);
  if (sessionCookies.length > 1) return errorResponse(400, "invalid_session_cookie");
  if (sessionCookies.length === 1) headers.set("cookie", sessionCookies[0]);
  if (isMutation) {
    // The browser Origin is checked above; the backend receives its own trusted API origin.
    headers.set("origin", upstreamOrigin.origin);
  }

  let body: Uint8Array<ArrayBuffer> | undefined;
  if (method !== "GET") {
    const bodyLimit = pathSegments[0] === "staff" && pathSegments[1] === "imports"
      ? maxImportBodyBytes
      : maxJsonBodyBytes;
    try {
      body = (await readBoundedBody(request, bodyLimit)) ?? undefined;
    } catch {
      return errorResponse(400, "invalid_request");
    }
    if (body === undefined) {
      return errorResponse(413, "request_too_large");
    }
  }

  const controller = new AbortController();
  const abortOnDisconnect = () => controller.abort();
  request.signal.addEventListener("abort", abortOnDisconnect, { once: true });
  let timedOut = false;
  const timeout = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, pathSegments[0] === "staff" && pathSegments[1] === "imports" ? 30_000 : 15_000);

  try {
    const upstreamResponse = await fetch(upstreamUrl, {
      method,
      headers,
      body,
      cache: "no-store",
      redirect: "manual",
      signal: controller.signal,
    });

    if (upstreamResponse.status >= 300 && upstreamResponse.status < 400) {
      return errorResponse(502, "unexpected_upstream_redirect");
    }

    const responseHeaders = new Headers();
    upstreamResponse.headers.forEach((value, key) => {
      if (key.toLowerCase() !== "set-cookie" && !hopByHopHeaders.has(key.toLowerCase())) {
        responseHeaders.append(key, value);
      }
    });
    const headersWithCookies = upstreamResponse.headers as Headers & { getSetCookie?: () => string[] };
    const setCookies = headersWithCookies.getSetCookie?.() ?? [];
    if (setCookies.length > 0) {
      for (const setCookie of setCookies) responseHeaders.append("set-cookie", setCookie);
    } else {
      const setCookie = upstreamResponse.headers.get("set-cookie");
      if (setCookie) responseHeaders.set("set-cookie", setCookie);
    }
    responseHeaders.set("Cache-Control", "no-store");

    const responseBody = [204, 205, 304].includes(upstreamResponse.status) ? null : upstreamResponse.body;
    return new Response(responseBody, {
      status: upstreamResponse.status,
      statusText: upstreamResponse.statusText,
      headers: responseHeaders,
    });
  } catch {
    return errorResponse(timedOut ? 504 : 503, timedOut ? "upstream_timeout" : "upstream_unavailable");
  } finally {
    clearTimeout(timeout);
    request.signal.removeEventListener("abort", abortOnDisconnect);
  }
}
