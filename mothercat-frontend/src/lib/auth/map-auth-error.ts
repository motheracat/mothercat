export type AuthErrorKind =
  | "invalid_credentials"
  | "rate_limited"
  | "account_locked"
  | "validation"
  | "network"
  | "server_error";

export type AuthErrorResult = {
  kind: AuthErrorKind;
  message: string;
  reference?: string;
  retryAfterSeconds?: number;
};

type AuthErrorLike = {
  status?: number;
  code?: string;
  requestId?: string;
  retryAfterSeconds?: number;
  name?: string;
  kind?: string;
  field?: string;
};

function isRecord(value: unknown): value is AuthErrorLike {
  return typeof value === "object" && value !== null;
}

function validRetryAfter(value: unknown): number | undefined {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) return undefined;
  return Math.ceil(value);
}

function validationMessage(field: string | undefined): string {
  switch (field) {
    case "staffId":
      return "Enter your staff ID.";
    case "staffIdFormat":
      return "Enter the 7-digit number after ID:.";
    case "password":
      return "Enter your password.";
    default:
      return "Check the highlighted fields and try again.";
  }
}

export function mapAuthError(error: unknown): AuthErrorResult {
  if (isRecord(error) && error.kind === "validation") {
    return {
      kind: "validation",
      message: validationMessage(error.field),
    };
  }

  if (
    error instanceof TypeError ||
    (isRecord(error) && ["AbortError", "TimeoutError", "AuthNetworkError"].includes(error.name ?? ""))
  ) {
    return {
      kind: "network",
      message: "Can't reach the server. Check your connection and try again.",
    };
  }

  const details = isRecord(error) ? error : {};

  if (details.status === 401 || details.code === "invalid_credentials") {
    return {
      kind: "invalid_credentials",
      message: "Your staff ID or password is incorrect.",
    };
  }

  if (details.status === 429 || details.code === "rate_limited") {
    return {
      kind: "rate_limited",
      message: "Too many attempts. Try again in",
      retryAfterSeconds: validRetryAfter(details.retryAfterSeconds) ?? 60,
    };
  }

  if (
    details.status === 403 ||
    details.code === "account_locked" ||
    details.code === "inactive" ||
    details.code === "account_inactive"
  ) {
    return {
      kind: "account_locked",
      message: "This account can't sign in right now. Contact your administrator.",
    };
  }

  if (typeof details.status === "number" && details.status >= 500) {
    return {
      kind: "server_error",
      message: "Something went wrong on our side. Please try again.",
      ...(typeof details.requestId === "string" && details.requestId.length > 0
        ? { reference: details.requestId }
        : {}),
    };
  }

  return {
    kind: "server_error",
    message: "Something went wrong on our side. Please try again.",
    ...(typeof details.requestId === "string" && details.requestId.length > 0
      ? { reference: details.requestId }
      : {}),
  };
}

export function formatRetryCountdown(seconds: number): string {
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return `${String(minutes).padStart(2, "0")}:${String(remainder).padStart(2, "0")}`;
}
