"use client";

import Link from "next/link";
import { useState } from "react";

import { forgotPassword } from "@/lib/auth/client";

export default function ForgotPasswordPage() {
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const formData = new FormData(event.currentTarget);
    const staffId = String(formData.get("staffId") ?? "").trim();

    if (!staffId) {
      setError("Please enter your staff ID.");
      setMessage(null);
      return;
    }

    setIsSubmitting(true);
    setError(null);
    setMessage(null);

    try {
      const result = await forgotPassword({ staffId });
      setMessage(result.message);
    } catch (submissionError) {
      setError(submissionError instanceof Error ? submissionError.message : "Unable to send reset instructions.");
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <div className="rounded-xl border border-slate-200 bg-slate-50 p-8 shadow-sm">
      <h2 className="font-serif text-4xl text-[#0f1b33]">Forgot password</h2>
      <p className="mt-2 text-sm text-slate-600">Enter your staff ID to receive reset instructions.</p>

      <form onSubmit={handleSubmit} className="mt-8 space-y-5">
        <div>
          <label htmlFor="staffId" className="mb-2 block text-sm font-medium text-[#0f1b33]">Staff ID</label>
          <input id="staffId" name="staffId" className="w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none transition focus:border-blue-500" placeholder="Enter staff ID" />
        </div>

        {error ? <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p> : null}
        {message ? <p className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{message}</p> : null}

        <button type="submit" disabled={isSubmitting} className="w-full rounded-md bg-[#1f5fd0] px-4 py-3 text-sm font-semibold text-white shadow-[0_10px_30px_rgba(31,95,208,0.2)] transition hover:bg-[#2f74e6] disabled:cursor-not-allowed disabled:opacity-70">
          {isSubmitting ? "Sending..." : "Send reset link"}
        </button>

        <div className="text-center text-sm text-slate-600">
          <Link href="/login" className="font-medium text-blue-600">Back to sign in</Link>
        </div>
      </form>
    </div>
  );
}
