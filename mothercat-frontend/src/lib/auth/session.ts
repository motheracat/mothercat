import "server-only";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { isDevOpen } from "@/lib/auth/mode";
import type { AuthSession } from "@/lib/auth/types";

export const SESSION_COOKIE_NAME = "mothercat_session";

const DEV_SESSION: AuthSession = {
  userId: "dev-user",
  staffId: "DEV-001",
  displayName: "Development User",
  role: "Super Admin",
  permissions: ["*"],
  isActive: true,
};

export async function getCurrentSession(): Promise<AuthSession | null> {
  if (isDevOpen()) {
    return DEV_SESSION;
  }

  const cookieStore = await cookies();
  const sessionValue = cookieStore.get(SESSION_COOKIE_NAME)?.value;

  if (!sessionValue) {
    return null;
  }

  try {
    const session = JSON.parse(sessionValue) as Partial<AuthSession>;
    if (session.userId && session.displayName && session.staffId) {
      return {
        userId: session.userId,
        staffId: session.staffId,
        displayName: session.displayName,
        role: session.role ?? "Staff",
        permissions: session.permissions ?? [],
        isActive: session.isActive ?? true,
      };
    }
  } catch {
    return null;
  }

  return null;
}

export async function requireAuth(redirectPath = "/dashboard") {
  const session = await getCurrentSession();

  if (!session) {
    redirect(`/login?redirect=${encodeURIComponent(redirectPath)}`);
  }

  return session;
}

export async function setSessionCookie(session: AuthSession) {
  const cookieStore = await cookies();
  const isSecure = process.env.NODE_ENV === "production";

  cookieStore.set(SESSION_COOKIE_NAME, JSON.stringify(session), {
    httpOnly: true,
    sameSite: "lax",
    secure: isSecure,
    path: "/",
    maxAge: 60 * 60 * 8,
  });
}

export async function clearSessionCookie() {
  const cookieStore = await cookies();
  cookieStore.delete(SESSION_COOKIE_NAME);
}
