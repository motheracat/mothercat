"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";

import { formatAuthError, getClientSession, login } from "@/lib/auth/client";
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
          ? "Your account is activated. Sign in to continue."
          : null;
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

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
          setError(formatAuthError(sessionError, "Unable to verify your current session."));
        }
      });

    return () => {
      active = false;
    };
  }, [redirectPath, router]);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    event.stopPropagation();

    const form = event.currentTarget;
    const formData = new FormData(form);
    const staffId = String(formData.get("staffId") ?? "").trim();
    const password = String(formData.get("password") ?? "");

    if (!staffId || !password) {
      form.reportValidity();
      setError("Please enter both your staff ID and password.");
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      await login({ staffId, password });
      router.replace(redirectPath);
    } catch (submissionError) {
      setError(formatAuthError(submissionError, "Sign in failed. Please check your staff ID and password."));
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <div className="rounded-xl border border-slate-200 bg-slate-50 p-8 shadow-sm">
      <h2 className="font-serif text-4xl text-[#0f1b33]">Sign in</h2>
      <p className="mt-2 text-sm text-slate-600">Use your staff ID and password to continue.</p>
      {notice ? <p role="status" className="mt-4 rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{notice}</p> : null}

      <form onSubmit={handleSubmit} noValidate className="mt-8 space-y-5">
        <div>
          <label htmlFor="staffId" className="mb-2 block text-sm font-medium text-[#0f1b33]">Staff ID</label>
          <input id="staffId" name="staffId" autoComplete="username" required className="w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none ring-0 transition focus:border-blue-500" placeholder="Enter staff ID" />
        </div>
        <div>
          <label htmlFor="password" className="mb-2 block text-sm font-medium text-[#0f1b33]">Password</label>
          <input id="password" name="password" type="password" autoComplete="current-password" required className="w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none ring-0 transition focus:border-blue-500" placeholder="Enter password" />
        </div>

        {error ? <p role="alert" className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p> : null}

        <div className="flex items-center justify-between">
          <Link href="/forgot-password" className="text-sm font-medium text-blue-600">Forgot password?</Link>
        </div>

        <button type="submit" disabled={isSubmitting} className="w-full rounded-md bg-[#1f5fd0] px-4 py-3 text-sm font-semibold !text-white shadow-[0_10px_30px_rgba(31,95,208,0.2)] transition hover:bg-[#2f74e6] disabled:cursor-not-allowed disabled:opacity-70">
          {isSubmitting ? "Signing in..." : "Sign in"}
        </button>
      </form>
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
