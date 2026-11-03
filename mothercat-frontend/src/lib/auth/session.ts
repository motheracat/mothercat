import "server-only";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { isDevOpen } from "@/lib/auth/mode";
import { parseAuthSession, type AuthSession } from "@/lib/auth/types";
import { env } from "@/lib/env";

export const SESSION_COOKIE_NAME = "mothercat_session";

const DEV_SESSION: AuthSession = {
  userId: "dev-user",
  staffId: "DEV-001",
  displayName: "Development User",
  email: "",
  role: "Super Admin",
  roles: ["Super Admin"],
  permissions: ["*"],
  isActive: true,
};

export async function getCurrentSession(): Promise<AuthSession | null> {
  if (isDevOpen()) {
    return DEV_SESSION;
  }

  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get(SESSION_COOKIE_NAME);

  if (!sessionCookie) {
    return null;
  }

  if (!env.API_BASE_URL) {
    throw new Error("API_BASE_URL is not configured. Set it to the organization API origin to use authenticated pages.");
  }

  const apiBaseUrl = env.API_BASE_URL.replace(/\/+$/, "");
  const response = await fetch(`${apiBaseUrl}/api/v1/auth/session`, {
    headers: {
      Accept: "application/json",
      Cookie: `${SESSION_COOKIE_NAME}=${sessionCookie.value}`,
    },
    cache: "no-store",
  });

  if (response.status === 401) {
    return null;
  }

  if (!response.ok) {
    throw new Error(`The authentication service could not verify the session (HTTP ${response.status}).`);
  }

  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    throw new Error("The authentication service returned an invalid session response.");
  }

  return parseAuthSession(payload);
}

export async function requireAuth(redirectPath = "/dashboard") {
  const session = await getCurrentSession();

  if (!session) {
    redirect(`/login?redirect=${encodeURIComponent(redirectPath)}`);
  }

  return session;
}
