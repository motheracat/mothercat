"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";

import { PasswordInput } from "@/components/auth/password-input";
import { activateAccount, formatAuthError } from "@/lib/auth/client";
import { validateNewPassword } from "@/lib/auth/password";

function ActivatePageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [token] = useState(() => searchParams.get("token") ?? "");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (!token) return;
    const url = new URL(window.location.href);
    url.searchParams.delete("token");
    window.history.replaceState(window.history.state, "", `${url.pathname}${url.search}${url.hash}`);
  }, [token]);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const formData = new FormData(event.currentTarget);
    const password = String(formData.get("password") ?? "");
    const confirmPassword = String(formData.get("confirmPassword") ?? "");
    if (!token) {
      setError("This activation link is missing its token. Contact your administrator.");
      return;
    }

    const passwordError = validateNewPassword(password, confirmPassword);
    if (passwordError) {
      setError(passwordError);
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      await activateAccount({ token, password, confirmPassword });
      router.push("/login?activated=1");
    } catch (submissionError) {
      setError(formatAuthError(submissionError, "Unable to activate the account."));
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <div className="rounded-xl border border-slate-200 bg-slate-50 p-8 shadow-sm">
      <h2 className="font-serif text-4xl text-[#0f1b33]">Activate account</h2>
      <p className="mt-2 text-sm text-slate-600">Set your password to complete the account activation process.</p>

      <form onSubmit={handleSubmit} className="mt-8 space-y-5">
        <div>
          <PasswordInput
            id="password"
            name="password"
            label="New password"
            value={password}
            onChange={setPassword}
            autoComplete="new-password"
            showStrength
          />
        </div>
        <div>
          <PasswordInput
            id="confirmPassword"
            name="confirmPassword"
            label="Confirm password"
            value={confirmPassword}
            onChange={setConfirmPassword}
            autoComplete="new-password"
          />
        </div>

        {error ? <p role="alert" className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p> : null}

        <button type="submit" disabled={isSubmitting} className="w-full rounded-md bg-[#1f5fd0] px-4 py-3 text-sm font-semibold text-white shadow-[0_10px_30px_rgba(31,95,208,0.2)] transition hover:bg-[#2f74e6] disabled:cursor-not-allowed disabled:opacity-70">
          {isSubmitting ? "Activating..." : "Activate account"}
        </button>

        <div className="text-center text-sm text-slate-600">
          <Link href="/login" className="font-medium text-blue-600">Back to sign in</Link>
        </div>
      </form>
    </div>
  );
}

export default function ActivatePage() {
  return (
    <Suspense fallback={<div className="rounded-xl border border-slate-200 bg-slate-50 p-8 shadow-sm">Loading...</div>}>
      <ActivatePageContent />
    </Suspense>
  );
}
