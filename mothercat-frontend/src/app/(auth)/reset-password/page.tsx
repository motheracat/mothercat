"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";

import { resetPassword } from "@/lib/auth/client";

function ResetPasswordPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const formData = new FormData(event.currentTarget);
    const password = String(formData.get("password") ?? "");
    const confirmPassword = String(formData.get("confirmPassword") ?? "");
    const token = searchParams.get("token") ?? "";

    if (!password || !confirmPassword) {
      setError("Please enter and confirm your new password.");
      return;
    }

    if (password.length < 8) {
      setError("Password must be at least 8 characters long.");
      return;
    }

    if (password !== confirmPassword) {
      setError("Passwords do not match.");
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      await resetPassword({ token, password, confirmPassword });
      router.push("/login");
    } catch (submissionError) {
      setError(submissionError instanceof Error ? submissionError.message : "Unable to reset password.");
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <div className="rounded-xl border border-slate-200 bg-slate-50 p-8 shadow-sm">
      <h2 className="font-serif text-4xl text-[#0f1b33]">Reset password</h2>
      <p className="mt-2 text-sm text-slate-600">Choose a new password that meets the security requirements.</p>

      <form onSubmit={handleSubmit} className="mt-8 space-y-5">
        <div>
          <label htmlFor="password" className="mb-2 block text-sm font-medium text-[#0f1b33]">New password</label>
          <input id="password" name="password" type="password" autoComplete="new-password" className="w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none transition focus:border-blue-500" placeholder="Create a password" />
        </div>
        <div>
          <label htmlFor="confirmPassword" className="mb-2 block text-sm font-medium text-[#0f1b33]">Confirm password</label>
          <input id="confirmPassword" name="confirmPassword" type="password" autoComplete="new-password" className="w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none transition focus:border-blue-500" placeholder="Confirm password" />
        </div>

        {error ? <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p> : null}

        <button type="submit" disabled={isSubmitting} className="w-full rounded-md bg-[#1f5fd0] px-4 py-3 text-sm font-semibold text-white shadow-[0_10px_30px_rgba(31,95,208,0.2)] transition hover:bg-[#2f74e6] disabled:cursor-not-allowed disabled:opacity-70">
          {isSubmitting ? "Updating..." : "Update password"}
        </button>

        <div className="text-center text-sm text-slate-600">
          <Link href="/login" className="font-medium text-blue-600">Back to sign in</Link>
        </div>
      </form>
    </div>
  );
}

export default function ResetPasswordPage() {
  return (
    <Suspense fallback={<div className="rounded-xl border border-slate-200 bg-slate-50 p-8 shadow-sm">Loading...</div>}>
      <ResetPasswordPageContent />
    </Suspense>
  );
}
