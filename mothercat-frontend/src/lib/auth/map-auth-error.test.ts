import { describe, expect, it } from "vitest";

import { formatRetryCountdown, mapAuthError } from "@/lib/auth/map-auth-error";

describe("mapAuthError", () => {
  it("maps invalid credentials without exposing backend references", () => {
    expect(mapAuthError({ status: 401, code: "invalid_credentials", requestId: "private-id" })).toEqual({
      kind: "invalid_credentials",
      message: "Your staff ID or password is incorrect.",
    });
  });

  it("maps rate limits and normalizes retry duration", () => {
    expect(mapAuthError({ status: 429, code: "rate_limited", retryAfterSeconds: 61.2 })).toEqual({
      kind: "rate_limited",
      message: "Too many attempts. Try again in",
      retryAfterSeconds: 62,
    });
    expect(formatRetryCountdown(65)).toBe("01:05");
  });

  it.each(["account_locked", "inactive"])("maps %s to a generic account message", (code) => {
    expect(mapAuthError({ status: 403, code, requestId: "private-id" })).toEqual({
      kind: "account_locked",
      message: "This account can't sign in right now. Contact your administrator.",
    });
  });

  it("maps client-side validation errors", () => {
    expect(mapAuthError({ kind: "validation", field: "staffId" })).toEqual({
      kind: "validation",
      message: "Enter your staff ID.",
    });
    expect(mapAuthError({ kind: "validation", field: "staffIdFormat" }).message).toBe(
      "Enter the 4- or 5-digit number after ID:.",
    );
    expect(mapAuthError({ kind: "validation", field: "password" }).message).toBe("Enter your password.");
  });

  it("maps network errors without exposing backend text", () => {
    expect(mapAuthError(new TypeError("fetch failed"))).toEqual({
      kind: "network",
      message: "Can't reach the server. Check your connection and try again.",
    });
  });

  it("shows a reference only for server errors", () => {
    expect(mapAuthError({ status: 503, requestId: "server-reference" })).toEqual({
      kind: "server_error",
      message: "Something went wrong on our side. Please try again.",
      reference: "server-reference",
    });
    expect(mapAuthError({ status: 401, requestId: "private-reference" }).reference).toBeUndefined();
    expect(mapAuthError({ status: 429, requestId: "private-reference" }).reference).toBeUndefined();
  });

  it("maps unknown errors to a generic server error", () => {
    expect(mapAuthError(new Error("backend secret"))).toEqual({
      kind: "server_error",
      message: "Something went wrong on our side. Please try again.",
    });
  });
});
