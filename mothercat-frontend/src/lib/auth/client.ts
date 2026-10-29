import { buildApiUrl } from "@/lib/api/config";
import { apiRoutes } from "@/lib/api/routes";
import type { AccountActivationPayload, AuthSession, LoginCredentials, PasswordResetPayload, PasswordResetRequest } from "@/lib/auth/types";

const SESSION_COOKIE_NAME = "mothercat_session";

function canRunClientAuthentication() {
  return typeof window !== "undefined";
}

function isDevelopmentRuntime() {
  if (typeof process !== "undefined" && process.env.NEXT_PUBLIC_AUTH_MODE === "dev-open") {
    return true;
  }
  if (typeof window !== "undefined") {
    const hostname = window.location.hostname;
    if (hostname === "localhost" || hostname === "127.0.0.1") {
      return true;
    }
  }
  return false;
}

function persistSession(session: AuthSession) {
  if (!canRunClientAuthentication()) {
    return;
  }

  const expires = new Date(Date.now() + 8 * 60 * 60 * 1000).toUTCString();
  const secure = window.location.protocol === "https:" ? "; Secure" : "";
  document.cookie = `${SESSION_COOKIE_NAME}=${encodeURIComponent(JSON.stringify(session))}; expires=${expires}; path=/; SameSite=Lax${secure}`;
}

function clearClientSession() {
  if (!canRunClientAuthentication()) {
    return;
  }

  document.cookie = `${SESSION_COOKIE_NAME}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/; SameSite=Lax`;
}

async function parseApiError(response: Response, fallbackMessage: string) {
  try {
    const payload = (await response.json()) as { message?: string; error?: string };
    return payload.message ?? payload.error ?? fallbackMessage;
  } catch {
    return fallbackMessage;
  }
}

function getDevSession(staffId: string): AuthSession {
  return {
    userId: "dev-user",
    staffId,
    displayName: `${staffId.replace(/-/g, " ")}`.trim() || "Development User",
    role: "Super Admin",
    permissions: ["*"],
    isActive: true,
  };
}

export async function login(credentials: LoginCredentials): Promise<AuthSession> {
  const fallbackSession = getDevSession(credentials.staffId);

  if (isDevelopmentRuntime()) {
    persistSession(fallbackSession);
    return fallbackSession;
  }

  try {
    const response = await fetch(buildApiUrl(apiRoutes.auth.login), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify(credentials),
    });

    const payload = (await response.json()) as { data?: AuthSession; message?: string };

    if (!response.ok) {
      // Per requirements: generic message
      throw new Error("sign-in failed");
    }

    const session = payload.data ?? fallbackSession;
    persistSession(session);
    return session;
  } catch (error) {
    if (isDevelopmentRuntime()) {
      persistSession(fallbackSession);
      return fallbackSession;
    }

    throw error instanceof Error ? error : new Error("sign-in failed");
  }
}

export async function forgotPassword(data: PasswordResetRequest): Promise<{ success: boolean; message: string }> {
  if (isDevelopmentRuntime()) {
    return {
      success: true,
      message: `Reset instructions were sent to ${data.staffId}.`,
    };
  }

  try {
    const response = await fetch(buildApiUrl(apiRoutes.auth.forgotPassword), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify(data),
    });

    if (!response.ok) {
      const message = await parseApiError(response, "Unable to send reset instructions.");
      throw new Error(message);
    }

    return {
      success: true,
      message: "Reset instructions were sent successfully.",
    };
  } catch (error) {
    if (isDevelopmentRuntime()) {
      return {
        success: true,
        message: `Reset instructions were sent to ${data.staffId}.`,
      };
    }

    throw error instanceof Error ? error : new Error("Unable to send reset instructions.");
  }
}

export async function resetPassword(data: PasswordResetPayload): Promise<{ success: boolean; message: string }> {
  if (isDevelopmentRuntime()) {
    return {
      success: true,
      message: "Your password was reset successfully.",
    };
  }

  try {
    const response = await fetch(buildApiUrl(apiRoutes.auth.resetPassword), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify(data),
    });

    if (!response.ok) {
      const message = await parseApiError(response, "Unable to reset password.");
      throw new Error(message);
    }

    return {
      success: true,
      message: "Your password was reset successfully.",
    };
  } catch (error) {
    if (isDevelopmentRuntime()) {
      return {
        success: true,
        message: "Your password was reset successfully.",
      };
    }

    throw error instanceof Error ? error : new Error("Unable to reset password.");
  }
}

export async function activateAccount(data: AccountActivationPayload): Promise<{ success: boolean; message: string }> {
  if (isDevelopmentRuntime()) {
    return {
      success: true,
      message: "Your account was activated successfully.",
    };
  }

  try {
    const response = await fetch(buildApiUrl(apiRoutes.auth.activate), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify(data),
    });

    if (!response.ok) {
      const message = await parseApiError(response, "Unable to activate the account.");
      throw new Error(message);
    }

    return {
      success: true,
      message: "Your account was activated successfully.",
    };
  } catch (error) {
    if (isDevelopmentRuntime()) {
      return {
        success: true,
        message: "Your account was activated successfully.",
      };
    }

    throw error instanceof Error ? error : new Error("Unable to activate the account.");
  }
}

export async function logout() {
  try {
    await fetch(buildApiUrl(apiRoutes.auth.logout), {
      method: "POST",
      credentials: "include",
    });
  } catch {
    // Ignore request failures here; the UI still clears its local session state.
  } finally {
    clearClientSession();
  }
}
