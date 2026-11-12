import { afterEach, describe, expect, it, vi } from "vitest";

import { proxyApiRequest } from "@/lib/api/proxy";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("proxyApiRequest", () => {
  it("removes stale compression headers from the decompressed upstream response", async () => {
    const body = JSON.stringify({ staff: [] });
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(body, {
        status: 200,
        headers: {
          "content-encoding": "gzip",
          "content-length": "999",
          "content-type": "application/json",
        },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await proxyApiRequest(
      new Request("https://frontend.example/api/v1/staff"),
      ["staff"],
      "https://api.example",
    );

    expect(response.status).toBe(200);
    expect(response.headers.has("content-encoding")).toBe(false);
    expect(response.headers.has("content-length")).toBe(false);
    expect(await response.text()).toBe(body);
  });

  it("checks the browser origin and forwards mutations with the API origin", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ staff: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await proxyApiRequest(
      new Request("https://frontend.example/api/v1/staff", {
        method: "POST",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
          cookie: "mothercat_session=session-value",
          host: "frontend.example",
          origin: "https://frontend.example",
        },
        body: JSON.stringify({ staff_id: "1234", full_name: "Test User" }),
      }),
      ["staff"],
      "https://api.example",
    );

    expect(response.status).toBe(200);
    const [url, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(url.toString()).toBe("https://api.example/api/v1/staff");
    expect(new Headers(init.headers).get("origin")).toBe("https://api.example");
    expect(new Headers(init.headers).get("cookie")).toBe("mothercat_session=session-value");
    expect(init.cache).toBe("no-store");
  });

  it("allows same-origin PUT requests required by Access role and assignment APIs", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({
        role_id: "auditor",
        permission_keys: ["audit.read"],
      }), { status: 200, headers: { "Content-Type": "application/json" } }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await proxyApiRequest(
      new Request("https://frontend.example/api/v1/roles/auditor/permissions", {
        method: "PUT",
        headers: {
          "content-type": "application/json",
          cookie: "mothercat_session=session-value",
          host: "frontend.example",
          origin: "https://frontend.example",
        },
        body: JSON.stringify({ permission_keys: ["audit.read"] }),
      }),
      ["roles", "auditor", "permissions"],
      "https://api.example",
    );

    expect(response.status).toBe(200);
    const [url, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(init.method).toBe("PUT");
    expect(url.toString()).toBe("https://api.example/api/v1/roles/auditor/permissions");
    expect(new Headers(init.headers).get("origin")).toBe("https://api.example");
  });

  it("forwards the If-Match header required for versioned task updates", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ task: { id: "task-uuid" } }), {
        status: 200,
        headers: { "Content-Type": "application/json", ETag: '"2"' },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await proxyApiRequest(
      new Request("https://frontend.example/api/v1/tasks/task-uuid", {
        method: "PATCH",
        headers: {
          "content-type": "application/json",
          cookie: "mothercat_session=session-value",
          host: "frontend.example",
          origin: "https://frontend.example",
          "if-match": '"1"',
        },
        body: JSON.stringify({ title: "Updated task" }),
      }),
      ["tasks", "task-uuid"],
      "https://api.example",
    );

    expect(response.status).toBe(200);
    const [, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(new Headers(init.headers).get("if-match")).toBe('"1"');
  });

  it("forwards only valid System Monitor verification headers", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const request = (mfa: string, action: string) => new Request(
      "https://frontend.example/api/v1/system-monitor/endpoints/pause",
      {
        method: "POST",
        headers: {
          "content-type": "application/json",
          cookie: "mothercat_session=session-value",
          host: "frontend.example",
          origin: "https://frontend.example",
          "x-system-monitor-mfa": mfa,
          "x-system-monitor-action": action,
        },
        body: JSON.stringify({ method: "GET", route: "/api/v1/example" }),
      },
    );

    await proxyApiRequest(request("123456", "confirm"), ["system-monitor", "endpoints", "pause"], "https://api.example");
    const [, validInit] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(new Headers(validInit.headers).get("x-system-monitor-mfa")).toBe("123456");
    expect(new Headers(validInit.headers).get("x-system-monitor-action")).toBe("confirm");

    await proxyApiRequest(request("12345x", "arbitrary"), ["system-monitor", "endpoints", "pause"], "https://api.example");
    const [, invalidInit] = fetchMock.mock.calls[1] as [URL, RequestInit];
    expect(new Headers(invalidInit.headers).has("x-system-monitor-mfa")).toBe(false);
    expect(new Headers(invalidInit.headers).has("x-system-monitor-action")).toBe(false);
  });

  it("allows only the bounded SIWES application upload route to exceed the JSON body limit", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ application: {} }), { status: 201 }));
    vi.stubGlobal("fetch", fetchMock);
    const body = new Uint8Array(20 * 1024);
    const response = await proxyApiRequest(
      new Request("https://frontend.example/api/v1/opportunities/123e4567-e89b-12d3-a456-426614174000/siwes-applications", {
        method: "POST",
        headers: {
          "content-type": "multipart/form-data; boundary=test-boundary",
          "content-length": String(body.byteLength),
          cookie: "mothercat_session=session-value",
          host: "frontend.example",
          origin: "https://frontend.example",
        },
        body,
      }),
      ["opportunities", "123e4567-e89b-12d3-a456-426614174000", "siwes-applications"],
      "https://api.example",
    );

    expect(response.status).toBe(201);
    const [, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(new Headers(init.headers).get("content-type")).toBe("multipart/form-data; boundary=test-boundary");
    expect(init.body).toBeInstanceOf(Uint8Array);
  });

  it("rejects SIWES uploads above the configured body limit", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const body = new Uint8Array(10 * 1024 * 1024 + 64 * 1024 + 1);
    const response = await proxyApiRequest(
      new Request("https://frontend.example/api/v1/opportunities/123e4567-e89b-12d3-a456-426614174000/siwes-applications", {
        method: "POST",
        headers: {
          "content-type": "multipart/form-data; boundary=test-boundary",
          "content-length": String(body.byteLength),
          host: "frontend.example",
          origin: "https://frontend.example",
        },
        body,
      }),
      ["opportunities", "123e4567-e89b-12d3-a456-426614174000", "siwes-applications"],
      "https://api.example",
    );

    expect(response.status).toBe(413);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects cross-origin mutations without contacting the backend", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    const response = await proxyApiRequest(
      new Request("https://frontend.example/api/v1/departments", {
        method: "POST",
        headers: {
          "content-type": "application/json",
          host: "frontend.example",
          origin: "https://attacker.example",
        },
        body: JSON.stringify({ name: "Engineering" }),
      }),
      ["departments"],
      "https://api.example",
    );

    expect(response.status).toBe(403);
    expect(await response.json()).toMatchObject({ error: { code: "invalid_origin" } });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("does not forward unrelated browser cookies", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ staff: [] }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    await proxyApiRequest(
      new Request("https://frontend.example/api/v1/staff", {
        headers: {
          accept: "application/json",
          cookie: "analytics=tracking; mothercat_session=session-value; theme=dark",
          host: "frontend.example",
        },
      }),
      ["staff"],
      "https://api.example",
    );

    const [, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(new Headers(init.headers).get("cookie")).toBe("mothercat_session=session-value");
  });

  it("rejects path traversal segments before contacting the backend", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const response = await proxyApiRequest(
      new Request("https://frontend.example/api/v1/staff"),
      ["..", "auth", "session"],
      "https://api.example",
    );

    expect(response.status).toBe(400);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("blocks oversized upload bodies before forwarding them", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const oversizedBody = new Uint8Array(5 * 1024 * 1024 + 64 * 1024 + 1);

    const response = await proxyApiRequest(
      new Request("https://frontend.example/api/v1/staff/imports", {
        method: "POST",
        headers: {
          "content-type": "multipart/form-data; boundary=test",
          "content-length": String(oversizedBody.byteLength),
          host: "frontend.example",
          origin: "https://frontend.example",
        },
        body: oversizedBody,
      }),
      ["staff", "imports"],
      "https://api.example",
    );

    expect(response.status).toBe(413);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("forwards profile picture bodies larger than the JSON limit", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const body = new Uint8Array(20 * 1024);
    const response = await proxyApiRequest(
      new Request("https://frontend.example/api/v1/staff/123e4567-e89b-12d3-a456-426614174000/profile-picture", {
        method: "POST",
        headers: {
          "content-type": "multipart/form-data; boundary=test",
          "content-length": String(body.byteLength),
          cookie: "mothercat_session=session-value",
          host: "frontend.example",
          origin: "https://frontend.example",
        },
        body,
      }),
      ["staff", "123e4567-e89b-12d3-a456-426614174000", "profile-picture"],
      "https://api.example",
    );

    expect(response.status).toBe(200);
    const [, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(init.body).toEqual(body);
    expect(new Headers(init.headers).get("content-type")).toBe("multipart/form-data; boundary=test");
    expect(new Headers(init.headers).get("cookie")).toBe("mothercat_session=session-value");
  });

  it("rejects profile picture bodies larger than 5 MiB plus the multipart allowance", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const body = new Uint8Array(5 * 1024 * 1024 + 64 * 1024 + 1);
    const response = await proxyApiRequest(
      new Request("https://frontend.example/api/v1/staff/123e4567-e89b-12d3-a456-426614174000/profile-picture", {
        method: "POST",
        headers: {
          "content-type": "multipart/form-data; boundary=test",
          "content-length": String(body.byteLength),
          host: "frontend.example",
          origin: "https://frontend.example",
        },
        body,
      }),
      ["staff", "123e4567-e89b-12d3-a456-426614174000", "profile-picture"],
      "https://api.example",
    );

    expect(response.status).toBe(413);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("keeps the JSON body limit at 16 KiB", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const body = new Uint8Array(16 * 1024 + 1);
    const response = await proxyApiRequest(
      new Request("https://frontend.example/api/v1/staff", {
        method: "POST",
        headers: {
          "content-type": "application/json",
          "content-length": String(body.byteLength),
          host: "frontend.example",
          origin: "https://frontend.example",
        },
        body,
      }),
      ["staff"],
      "https://api.example",
    );

    expect(response.status).toBe(413);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("keeps the larger CSV import limit", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("{}", { status: 201 }));
    vi.stubGlobal("fetch", fetchMock);
    const body = new Uint8Array(20 * 1024);
    const response = await proxyApiRequest(
      new Request("https://frontend.example/api/v1/staff/imports", {
        method: "POST",
        headers: {
          "content-type": "multipart/form-data; boundary=test",
          "content-length": String(body.byteLength),
          host: "frontend.example",
          origin: "https://frontend.example",
        },
        body,
      }),
      ["staff", "imports"],
      "https://api.example",
    );

    expect(response.status).toBe(201);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
