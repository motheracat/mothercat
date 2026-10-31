"use client";

import type { AuthSession } from "@/lib/auth/types";

const SESSION_COOKIE_NAME = "mothercat_session";

export function getClientSession(): AuthSession | null {
  if (typeof document === "undefined") {
    return null;
  }
  const cookies = document.cookie.split(";");
  for (let i = 0; i < cookies.length; i++) {
    const cookie = cookies[i].trim();
    if (cookie.startsWith(SESSION_COOKIE_NAME + "=")) {
      try {
        const value = decodeURIComponent(cookie.substring(SESSION_COOKIE_NAME.length + 1));
        const session = JSON.parse(value) as AuthSession;
        if (session.userId && session.displayName && session.staffId) {
          return session;
        }
      } catch {
        return null;
      }
    }
  }
  return null;
}
