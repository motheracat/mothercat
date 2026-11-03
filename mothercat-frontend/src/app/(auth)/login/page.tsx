"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";

import { getClientSession, login } from "@/lib/auth/client";
import { SignInForm } from "@/components/auth/sign-in-form";
import { mapAuthError, type AuthErrorResult } from "@/lib/auth/map-auth-error";
import { getSafeRedirectPath } from "@/lib/auth/redirect";

function LoginPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const redirectPath = getSafeRedirectPath(searchParams.get("redirect") ?? searchParams.get("next"));
  const notice =
    searchParams.get("passwordChanged") === "1"
      ? "Your password was changed. Sign in again with your new password."
      : searchParams.get("passwordReset") === "1"
        ? "Your password was reset. Sign in with your new password."
        : searchParams.get("activated") === "1"
          ? "Your account is activated. Sign in to cWe couldn't find your staff record. Refresh your profile or contact an administrator.ontinue."
          : null;
  const [sessionError, setSessionError] = useState<AuthErrorResult | null>(null);

  useEffect(() => {
    let active = true;
    void getClientSession()
      .then((session) => {
        if (active && session) {
          router.replace(redirectPath);
        }
      })
      .catch((sessionError: unknown) => {
        if (active) {
          setSessionError(mapAuthError(sessionError));
        }
      });

    return () => {
      active = false;
    };
  }, [redirectPath, router]);

  return (
    <div className="rounded-xl border border-slate-200 bg-slate-50 p-8 shadow-sm">
      <h2 className="font-serif text-4xl text-[#0f1b33]">Sign in</h2>
      <p className="mt-2 text-sm text-slate-600">Use your staff ID and password to continue.</p>
      {notice ? <p role="status" className="mt-4 rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{notice}</p> : null}
      {sessionError ? (
        <p role="status" className="mt-5 text-sm text-[var(--muted)]">{sessionError.message}</p>
      ) : null}
      <div className="mt-8">
        <SignInForm
          onLogin={async (credentials) => {
            await login(credentials);
            router.replace(redirectPath);
          }}
        />
      </div>
    </div>
  );
}

export default function LoginPage() {
  return (
    <Suspense fallback={<div className="rounded-xl border border-slate-200 bg-slate-50 p-8 shadow-sm">Loading...</div>}>
      <LoginPageContent />
    </Suspense>
  );
}
