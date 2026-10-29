"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";

import { getClientSession } from "@/lib/auth/session-client";
import { login } from "@/lib/auth/client";
import { getSafeRedirectPath } from "@/lib/auth/redirect";

function LoginPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    try {
      const session = getClientSession();
      if (session) {
        const redirectPath = getSafeRedirectPath(searchParams.get("redirect") ?? searchParams.get("next"));
        router.replace(redirectPath || "/dashboard");
      }
    } catch {
      // Ignore
    }
  }, [router, searchParams]);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    event.stopPropagation();

    const formData = new FormData(event.currentTarget);
    const staffId = String(formData.get("staffId") ?? "").trim();
    const password = String(formData.get("password") ?? "").trim();

    if (!staffId || !password) {
      setError("Please enter both your staff ID and password.");
      setIsSubmitting(false);
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      await login({ staffId, password });
      const redirectPath = getSafeRedirectPath(searchParams.get("redirect") ?? searchParams.get("next")) || "/dashboard";
      window.location.href = redirectPath;
    } catch (submissionError) {
      setError("sign-in failed");
      setIsSubmitting(false);
    }
  }

  return (
    <div className="rounded-xl border border-slate-200 bg-slate-50 p-8 shadow-sm">
      <h2 className="font-serif text-4xl text-[#0f1b33]">Sign in</h2>
      <p className="mt-2 text-sm text-slate-600">Use your staff ID and password to continue.</p>

      <form onSubmit={handleSubmit} className="mt-8 space-y-5">
        <div>
          <label htmlFor="staffId" className="mb-2 block text-sm font-medium text-[#0f1b33]">Staff ID</label>
          <input id="staffId" name="staffId" autoComplete="username" className="w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none ring-0 transition focus:border-blue-500" placeholder="Enter staff ID" />
        </div>
        <div>
          <label htmlFor="password" className="mb-2 block text-sm font-medium text-[#0f1b33]">Password</label>
          <input id="password" name="password" type="password" autoComplete="current-password" className="w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none ring-0 transition focus:border-blue-500" placeholder="Enter password" />
        </div>

        {error ? <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p> : null}

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
