"use client";

import { useState, type FormEvent } from "react";
import { ArrowRight, CheckCircle2, Mail, Sparkles } from "lucide-react";

export function NewsletterSignup() {
  const [email, setEmail] = useState("");
  const [message, setMessage] = useState("");
  const [isError, setIsError] = useState(false);

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = email.trim();
    const isValid = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmed);

    if (!isValid) {
      setIsError(true);
      setMessage("Please enter a valid work or personal email address.");
      return;
    }

    setIsError(false);
    setMessage(`Thanks! ${trimmed} has been added to our project updates list.`);
    setEmail("");
  }

  return (
    <aside className="rounded-2xl border border-slate-200 bg-[#071535] p-6 text-white shadow-[0_18px_36px_rgba(7,21,53,0.18)] sm:p-7">
      <div className="inline-flex items-center gap-2 rounded-full border border-white/15 bg-white/5 px-3 py-1 text-xs font-semibold uppercase tracking-[0.14em] text-blue-100">
        <Sparkles size={12} aria-hidden="true" />
        Newsletter
      </div>
      <h2 className="mt-4 font-serif text-3xl text-white">Stay in the loop</h2>
      <p className="mt-3 text-sm leading-6 text-slate-200">
        Receive updates on project milestones, operational insights, and new opportunities across our portfolio.
      </p>

      <form onSubmit={handleSubmit} className="mt-6 space-y-3">
        <label htmlFor="newsletter-email" className="sr-only">
          Email address
        </label>
        <div className="flex items-center gap-2 rounded-lg border border-white/15 bg-white/5 px-3 py-2 text-slate-100 focus-within:border-blue-300 focus-within:ring-2 focus-within:ring-blue-200/70">
          <Mail size={16} aria-hidden="true" className="text-blue-200" />
          <input
            id="newsletter-email"
            type="email"
            name="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            placeholder="Enter your email"
            className="w-full bg-transparent text-sm text-white placeholder:text-slate-300 focus:outline-none"
            aria-describedby="newsletter-message"
          />
        </div>

        <button
          type="submit"
          className="inline-flex w-full items-center justify-center gap-2 rounded-lg bg-white px-4 py-3 text-sm font-semibold text-[#071535] transition hover:bg-slate-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white focus-visible:ring-offset-2 focus-visible:ring-offset-[#071535]"
        >
          Join updates
          <ArrowRight size={16} aria-hidden="true" />
        </button>
      </form>

      {message ? (
        <p
          id="newsletter-message"
          className={`mt-4 inline-flex items-center gap-2 text-sm ${isError ? "text-red-200" : "text-emerald-200"}`}
        >
          {!isError && <CheckCircle2 size={15} aria-hidden="true" />}
          {message}
        </p>
      ) : null}
    </aside>
  );
}
