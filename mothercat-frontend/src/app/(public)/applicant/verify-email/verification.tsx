"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useState } from "react";

type VerificationState = "ready" | "verifying" | "verified" | "invalid" | "unavailable";

export default function ApplicantEmailVerification() {
  const searchParams = useSearchParams();
  const token = searchParams.get("token") ?? "";
  const [state, setState] = useState<VerificationState>(token ? "ready" : "invalid");

  async function verifyEmail() {
    if (!token) {
      setState("invalid");
      return;
    }
    setState("verifying");
    try {
      const response = await fetch("/api/v1/auth/applicant/verify-email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ token }),
      });
      if (response.ok) {
        setState("verified");
      } else if (response.status === 400) {
        setState("invalid");
      } else {
        setState("unavailable");
      }
    } catch {
      setState("unavailable");
    }
  }

  const message: Record<VerificationState, string> = {
    ready: "Select the button below to verify this email address.",
    verifying: "Confirming your email address…",
    verified: "Your email address has been verified. You can now sign in to your applicant account.",
    invalid: "This verification link is missing a token or is not valid.",
    unavailable: "We couldn’t verify your email right now. You can request another verification email and try again.",
  };

  return (
    <main className="mx-auto max-w-xl px-4 py-20 text-[#0f1b33] sm:px-6">
      <section className="rounded-xl border border-slate-200 bg-white p-8 shadow-sm">
        <p className="text-sm font-semibold uppercase tracking-wide text-blue-700">Applicant account</p>
        <h1 className="mt-3 font-serif text-4xl">Email verification</h1>
        <p role="status" aria-live="polite" className="mt-5 text-sm leading-6 text-slate-600">
          {message[state]}
        </p>
        {(state === "ready" || state === "unavailable") && token ? (
          <button
            type="button"
            className="mt-6 min-h-11 rounded-md bg-[#071535] px-5 text-sm font-semibold text-white hover:bg-[#1f5fd0]"
            onClick={() => void verifyEmail()}
            disabled={state === "verifying"}
          >
            {state === "ready" ? "Verify email" : "Try again"}
          </button>
        ) : null}
        <div className="mt-6">
          <Link href="/opportunities" className="text-sm font-semibold text-blue-700 underline underline-offset-4">
            Back to opportunities
          </Link>
        </div>
      </section>
    </main>
  );
}
