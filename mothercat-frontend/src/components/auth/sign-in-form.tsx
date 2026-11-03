"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";

import { PasswordInput } from "@/components/auth/password-input";
import { formatRetryCountdown, mapAuthError, type AuthErrorResult } from "@/lib/auth/map-auth-error";
import type { LoginCredentials } from "@/lib/auth/types";

type FieldErrors = {
  staffId?: string;
  password?: string;
};

export function SignInForm({ onLogin }: { onLogin: (credentials: LoginCredentials) => Promise<void> }) {
  const [staffId, setStaffId] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<AuthErrorResult | null>(null);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [retrySeconds, setRetrySeconds] = useState<number | null>(null);
  const [copyStatus, setCopyStatus] = useState("");
  const passwordRef = useRef<HTMLInputElement>(null);
  const staffIdRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (retrySeconds === null) return;
    const timer = window.setTimeout(() => {
      if (retrySeconds <= 1) {
        setRetrySeconds(null);
        setError(null);
      } else {
        setRetrySeconds(retrySeconds - 1);
      }
    }, 1000);
    return () => window.clearTimeout(timer);
  }, [retrySeconds]);

  useEffect(() => {
    if (!copyStatus) return;
    const timer = window.setTimeout(() => setCopyStatus(""), 1800);
    return () => window.clearTimeout(timer);
  }, [copyStatus]);

  function clearFeedback() {
    setError(null);
    setFieldErrors({});
    setRetrySeconds(null);
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSubmitting || (retrySeconds !== null && retrySeconds > 0)) return;

    const normalizedStaffId = staffId.trim();
    const nextFieldErrors: FieldErrors = {};
    if (!normalizedStaffId) {
      nextFieldErrors.staffId = mapAuthError({ kind: "validation", field: "staffId" }).message;
    } else if (!/^\d{7}$/.test(normalizedStaffId)) {
      nextFieldErrors.staffId = mapAuthError({ kind: "validation", field: "staffIdFormat" }).message;
    }

    if (!password) {
      nextFieldErrors.password = mapAuthError({ kind: "validation", field: "password" }).message;
    }

    if (Object.keys(nextFieldErrors).length > 0) {
      setFieldErrors(nextFieldErrors);
      setError(null);
      setRetrySeconds(null);
      if (nextFieldErrors.staffId) {
        staffIdRef.current?.focus();
      } else {
        passwordRef.current?.focus();
      }
      return;
    }

    setIsSubmitting(true);
    setError(null);
    setFieldErrors({});

    try {
      await onLogin({ staffId: normalizedStaffId, password });
    } catch (failure) {
      const mapped = mapAuthError(failure);
      setError(mapped);
      if (mapped.kind === "invalid_credentials") {
        setPassword("");
        window.requestAnimationFrame(() => passwordRef.current?.focus());
      } else if (mapped.kind === "rate_limited") {
        setRetrySeconds(mapped.retryAfterSeconds ?? 60);
      }
    } finally {
      setIsSubmitting(false);
    }
  }

  async function copyReference() {
    if (!error?.reference) return;
    try {
      await navigator.clipboard.writeText(error.reference);
      setCopyStatus("Copied");
    } catch {
      setCopyStatus("Copy failed");
    }
  }

  const invalidCredentials = error?.kind === "invalid_credentials";
  const isRateLimited = retrySeconds !== null && retrySeconds > 0;

  return (
    <form onSubmit={handleSubmit} noValidate className="sign-in-form">
      <div>
        <label htmlFor="staffId" className="field-label">Staff ID</label>
        <div className={`sign-in-id-input${fieldErrors.staffId || invalidCredentials ? " has-error" : ""}`}>
          <span className="sign-in-id-prefix" aria-hidden="true">ID:</span>
          <input
            ref={staffIdRef}
            id="staffId"
            name="staffId"
            type="text"
            inputMode="numeric"
            autoComplete="username"
            maxLength={7}
            pattern="[0-9]{7}"
            required
            value={staffId}
            onChange={(event) => {
              setStaffId(event.target.value.replace(/\D/g, "").slice(0, 7));
              clearFeedback();
            }}
            aria-invalid={Boolean(fieldErrors.staffId || invalidCredentials)}
            aria-describedby={invalidCredentials ? "sign-in-error" : fieldErrors.staffId ? "staff-id-error" : undefined}
            className="sign-in-id-field"
            placeholder="1234567"
          />
        </div>
        {fieldErrors.staffId ? <p id="staff-id-error" className="form-field-error">{fieldErrors.staffId}</p> : null}
      </div>

      {error && error.kind !== "validation" ? (
        <div id="sign-in-error" role="alert" aria-live="assertive" className="sign-in-error">
          <p className="font-medium">Sign-in failed</p>
          <p className="mt-1">{error.message}{error.kind === "rate_limited" ? ` ${formatRetryCountdown(error.retryAfterSeconds ?? 60)}.` : ""}</p>
          {error.reference ? (
            <div className="sign-in-reference">
              <span title={error.reference}>Reference: {error.reference.slice(0, 12)}…</span>
              <button type="button" onClick={copyReference} className="sign-in-copy">{copyStatus || "Copy"}</button>
            </div>
          ) : null}
        </div>
      ) : null}

      <div>
        <PasswordInput
          id="sign-in-password"
          name="password"
          label="Password"
          value={password}
          onChange={(value) => {
            setPassword(value);
            clearFeedback();
          }}
          autoComplete="current-password"
          inputRef={passwordRef}
          ariaInvalid={Boolean(fieldErrors.password || invalidCredentials)}
          ariaDescribedBy={invalidCredentials ? "sign-in-error" : fieldErrors.password ? "password-error" : undefined}
        />
        {fieldErrors.password ? <p id="password-error" className="form-field-error">{fieldErrors.password}</p> : null}
        <div className="mt-2 flex justify-end">
          <Link href="/forgot-password" className="text-sm font-medium text-[var(--accent)] hover:underline">Forgot password?</Link>
        </div>
      </div>

      <div className="sign-in-submit">
        <button type="submit" disabled={isSubmitting || isRateLimited} className="button-primary sign-in-button">
          {isSubmitting ? <><span aria-hidden="true" className="button-spinner" />Signing in…</> : "Sign in"}
        </button>
        {isRateLimited ? <p aria-live="polite" className="sign-in-countdown">You can try again in {formatRetryCountdown(retrySeconds)}.</p> : null}
      </div>
    </form>
  );
}
