"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { PasswordInput } from "@/components/auth/password-input";
import { PageHeader } from "@/components/workspace/ui";
import { changePassword, formatAuthError } from "@/lib/auth/client";
import { validateNewPassword } from "@/lib/auth/password";

export default function AccountPage() {
  const router = useRouter();
  const [currentPassword, setCurrentPassword] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
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
    <>
      <PageHeader title="Password & security" description="Manage your password and account security." />
      <section className="surface-card mx-auto w-full max-w-[560px] p-5 sm:p-6">
        <h2 className="text-sm font-semibold text-[var(--text)]">Change password</h2>
        <p className="mt-2 text-sm leading-5 text-[var(--muted)]">You will need to sign in again after your password is updated.</p>

        <form onSubmit={handleSubmit} className="mt-6 space-y-5">
          <div>
            <PasswordInput
              id="currentPassword"
              name="currentPassword"
              label="Current password"
              value={currentPassword}
              onChange={setCurrentPassword}
              autoComplete="current-password"
            />
          </div>
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
              label="Confirm new password"
              value={confirmPassword}
              onChange={setConfirmPassword}
              autoComplete="new-password"
            />
          </div>

          <p className="text-xs leading-5 text-[var(--muted)]">Use 14–72 bytes and include at least three of uppercase letters, lowercase letters, numbers, and symbols.</p>
          {error ? <p role="alert" className="field-error">{error}</p> : null}

          <button type="submit" disabled={isSubmitting} className="button-primary">
            {isSubmitting ? "Updating..." : "Change password"}
          </button>
        </form>
      </section>
    </>
  );
}
