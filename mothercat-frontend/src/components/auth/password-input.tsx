"use client";

import { Eye, EyeOff } from "lucide-react";
import { useRef, useState, type RefObject } from "react";

import { getPasswordStrength } from "@/lib/auth/password";

export function PasswordInput({
  id,
  name,
  label,
  value,
  onChange,
  autoComplete = "new-password",
  required = true,
  showStrength = false,
  inputRef,
  ariaInvalid,
  ariaDescribedBy,
}: {
  id: string;
  name: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  autoComplete?: string;
  required?: boolean;
  showStrength?: boolean;
  inputRef?: RefObject<HTMLInputElement | null>;
  ariaInvalid?: boolean;
  ariaDescribedBy?: string;
}) {
  const [visible, setVisible] = useState(false);
  const strength = getPasswordStrength(value);
  const internalRef = useRef<HTMLInputElement>(null);

  return (
    <div>
      <label htmlFor={id} className="field-label">{label}</label>
      <div className="password-input-wrap">
        <input
          ref={inputRef ?? internalRef}
          id={id}
          name={name}
          type={visible ? "text" : "password"}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          autoComplete={autoComplete}
          required={required}
          aria-invalid={ariaInvalid}
          aria-describedby={ariaDescribedBy}
          className="form-input password-input"
        />
        <button
          type="button"
          className="password-visibility"
          aria-label={visible ? "Hide password" : "Show password"}
          aria-pressed={visible}
          onClick={() => setVisible((current) => !current)}
        >
          {visible ? <EyeOff size={17} aria-hidden="true" /> : <Eye size={17} aria-hidden="true" />}
        </button>
      </div>
      {showStrength ? <PasswordStrengthIndicator password={value} strength={strength} /> : null}
    </div>
  );
}

export function PasswordStrengthIndicator({
  password,
  strength = getPasswordStrength(password),
}: {
  password: string;
  strength?: ReturnType<typeof getPasswordStrength>;
}) {
  const score = (strength.lengthValid ? 1 : 0) + strength.characterClasses;
  const status = strength.valid ? "Meets password requirements" : "Does not yet meet password requirements";
  const barColor = strength.valid ? "var(--success-text)" : score >= 3 ? "var(--warning-text)" : "var(--danger-text)";

  return (
    <div className="password-strength">
      <div
        className="password-strength-meter"
        role="progressbar"
        aria-label="Password requirement progress"
        aria-valuemin={0}
        aria-valuemax={5}
        aria-valuenow={score}
      >
        {Array.from({ length: 5 }, (_, index) => (
          <span key={index} style={{ backgroundColor: index < score ? barColor : "var(--neutral-bg)" }} />
        ))}
      </div>
      <p className={strength.valid ? "password-strength-valid" : "password-strength-help"}>{status}</p>
      <p className="password-strength-help">
        {strength.byteLength} / 72 bytes · {strength.characterClasses} / 4 character types · minimum 14 bytes
      </p>
    </div>
  );
}
