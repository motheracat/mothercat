import { buildApiUrl } from "@/lib/api/config";
import { apiRoutes } from "@/lib/api/routes";
import {
  parseAuthSession,
  type LoginCredentials,
  type PasswordChangePayload,
  type PasswordCompletionPayload,
  type PasswordResetRequest,
} from "@/lib/auth/types";
import { mapAuthError } from "@/lib/auth/map-auth-error";

type ApiErrorPayload = {
  error?: {
    code?: string;
    message?: string;
    request_id?: string;
  };
};

export class AuthApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
    readonly requestId?: string,
    readonly retryAfterSeconds?: number,
  ) {
    super(message);
    this.name = "AuthApiError";
  }
}

export class AuthNetworkError extends Error {
  constructor() {
    super("Network request failed.");
    this.name = "AuthNetworkError";
  }
}

function parseRetryAfter(response: Response, payload: unknown): number | undefined {
  const header = response.headers.get("Retry-After");
  if (header) {
    const seconds = Number(header);
    if (Number.isFinite(seconds)) return Math.max(0, Math.ceil(seconds));
    const dateMilliseconds = Date.parse(header);
    if (Number.isFinite(dateMilliseconds)) return Math.max(0, Math.ceil((dateMilliseconds - Date.now()) / 1000));
  }

  if (typeof payload !== "object" || payload === null) return undefined;
  const body = payload as {
    retry_after_seconds?: unknown;
    retryAfterSeconds?: unknown;
    retry_after?: unknown;
    error?: { retry_after_seconds?: unknown; retryAfterSeconds?: unknown; retry_after?: unknown };
  };
  for (const candidate of [
    body.retry_after_seconds,
    body.retryAfterSeconds,
    body.retry_after,
    body.error?.retry_after_seconds,
    body.error?.retryAfterSeconds,
    body.error?.retry_after,
  ]) {
    if (typeof candidate === "number" && Number.isFinite(candidate) && candidate >= 0) {
      return Math.ceil(candidate);
    }
  }
  return undefined;
}

async function requestApi(
  path: string,
  method: "GET" | "POST",
  body?: Record<string, string>,
): Promise<unknown> {
  let response: Response;
  const controller = new AbortController();
  let timedOut = false;
  const timeout = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, 10_000);

  try {
    response = await fetch(buildApiUrl(path), {
      method,
      headers: {
        Accept: "application/json",
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      credentials: "include",
      cache: "no-store",
      signal: controller.signal,
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
  } catch {
    clearTimeout(timeout);
    throw new AuthNetworkError();
  }

  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    payload = null;
  } finally {
    clearTimeout(timeout);
  }

  if (timedOut) throw new AuthNetworkError();

  if (!response.ok) {
    const apiError = (payload as ApiErrorPayload | null)?.error;
    throw new AuthApiError(
      typeof apiError?.message === "string" ? apiError.message : "The request could not be completed.",
      response.status,
      typeof apiError?.code === "string" ? apiError.code : "request_failed",
      typeof apiError?.request_id === "string" ? apiError.request_id : undefined,
      response.status === 429 ? parseRetryAfter(response, payload) : undefined,
    );
  }

  if (payload === null) {
    throw new Error("The authentication service returned an invalid response.");
  }

  return payload;
}

export function formatAuthError(error: unknown, fallback: string): string {
  if (error instanceof AuthNetworkError) {
    return mapAuthError(error).message;
  }

  if (error instanceof AuthApiError) {
    const safeMessages: Record<string, string> = {
      invalid_credentials: "Your staff ID or password is incorrect.",
      unauthorized: "Your session has expired. Sign in again.",
      invalid_session: "Your session has expired. Sign in again.",
      invalid_request: "Some information is missing or invalid. Review the fields and try again.",
      invalid_token: "This link is invalid, expired, or already used. Request a new link.",
      weak_password: "Use 14–72 bytes and include at least three of uppercase, lowercase, numbers, and symbols.",
      rate_limited: "Too many attempts. Wait a few minutes before trying again.",
      email_delivery_disabled: "Recovery email delivery is temporarily unavailable. Contact your administrator.",
      service_unavailable: "The authentication service is temporarily unavailable. Try again later.",
      internal_error: "The request could not be completed. Try again later.",
    };
    const message = safeMessages[error.code] ?? fallback;
    return error.status >= 500 && error.requestId ? `${message} Reference: ${error.requestId}` : message;
  }

  return fallback;
}

export async function getClientSession() {
  try {
    return parseAuthSession(await requestApi(apiRoutes.auth.session, "GET"));
  } catch (error) {
    if (error instanceof AuthApiError && error.status === 401) {
      return null;
    }
    throw error;
  }
}

export async function login(credentials: LoginCredentials) {
  const payload = await requestApi(apiRoutes.auth.login, "POST", {
    staff_id: credentials.staffId,
    password: credentials.password,
  });
  return parseAuthSession(payload);
}

export async function forgotPassword(data: PasswordResetRequest): Promise<void> {
  await requestApi(apiRoutes.auth.forgotPassword, "POST", { staff_id: data.staffId });
}

export async function resetPassword(data: PasswordCompletionPayload): Promise<void> {
  await requestApi(apiRoutes.auth.resetPassword, "POST", {
    token: data.token,
    password: data.password,
    confirm_password: data.confirmPassword,
  });
}

export async function activateAccount(data: PasswordCompletionPayload): Promise<void> {
  await requestApi(apiRoutes.auth.activate, "POST", {
    token: data.token,
    password: data.password,
    confirm_password: data.confirmPassword,
  });
}

export async function changePassword(data: PasswordChangePayload): Promise<void> {
  await requestApi(apiRoutes.auth.changePassword, "POST", {
    current_password: data.currentPassword,
    password: data.password,
    confirm_password: data.confirmPassword,
  });
}

export async function logout(): Promise<void> {
  await requestApi(apiRoutes.auth.logout, "POST");
}
