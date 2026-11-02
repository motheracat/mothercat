"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { changePassword, formatAuthError } from "@/lib/auth/client";
import { validateNewPassword } from "@/lib/auth/password";

export default function AccountPage() {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const formData = new FormData(event.currentTarget);
    const currentPassword = String(formData.get("currentPassword") ?? "");
    const password = String(formData.get("password") ?? "");
    const confirmPassword = String(formData.get("confirmPassword") ?? "");
    const passwordError = validateNewPassword(password, confirmPassword);

    if (!currentPassword) {
      setError("Enter your current password.");
      return;
    }

    if (passwordError) {
      setError(passwordError);
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      await changePassword({ currentPassword, password, confirmPassword });
      router.replace("/login?passwordChanged=1");
    } catch (submissionError) {
      setError(formatAuthError(submissionError, "Unable to change your password."));
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <div className="max-w-2xl rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
      <h2 className="font-serif text-2xl text-[#0f1b33]">Password &amp; security</h2>
      <p className="mt-2 text-sm text-slate-600">Change your password. You will need to sign in again after it is updated.</p>

      <form onSubmit={handleSubmit} className="mt-6 space-y-5">
        <div>
          <label htmlFor="currentPassword" className="mb-2 block text-sm font-medium text-[#0f1b33]">Current password</label>
          <input id="currentPassword" name="currentPassword" type="password" autoComplete="current-password" required className="w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none transition focus:border-blue-500" />
        </div>
        <div>
          <label htmlFor="password" className="mb-2 block text-sm font-medium text-[#0f1b33]">New password</label>
          <input id="password" name="password" type="password" autoComplete="new-password" required className="w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none transition focus:border-blue-500" />
        </div>
        <div>
          <label htmlFor="confirmPassword" className="mb-2 block text-sm font-medium text-[#0f1b33]">Confirm new password</label>
          <input id="confirmPassword" name="confirmPassword" type="password" autoComplete="new-password" required className="w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none transition focus:border-blue-500" />
        </div>

        <p className="text-sm text-slate-600">Use 14–72 bytes and include at least three of uppercase letters, lowercase letters, numbers, and symbols.</p>
        {error ? <p role="alert" className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p> : null}

        <button type="submit" disabled={isSubmitting} className="rounded-md bg-[#1f5fd0] px-5 py-3 text-sm font-semibold text-white transition hover:bg-[#2f74e6] disabled:cursor-not-allowed disabled:opacity-70">
          {isSubmitting ? "Updating..." : "Change password"}
        </button>
      </form>
    </div>
  );
}
