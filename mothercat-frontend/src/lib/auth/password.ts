export type PasswordStrength = {
  byteLength: number;
  lengthValid: boolean;
  characterClasses: number;
  valid: boolean;
};

export function getPasswordStrength(password: string): PasswordStrength {
  const byteLength = new TextEncoder().encode(password).length;
  const lengthValid = byteLength >= 14 && byteLength <= 72;
  const characterClasses = [
    /\p{Lu}/u.test(password),
    /\p{Ll}/u.test(password),
    /\p{Nd}/u.test(password),
    /[^\p{Lu}\p{Ll}\p{Nd}]/u.test(password),
  ].filter(Boolean).length;

  return {
    byteLength,
    lengthValid,
    characterClasses,
    valid: lengthValid && characterClasses >= 3,
  };
}

export function validateNewPassword(password: string, confirmation: string): string | null {
  const strength = getPasswordStrength(password);

  if (!strength.lengthValid) {
    return "Password must be 14–72 bytes long.";
  }

  if (strength.characterClasses < 3) {
    return "Password must include at least three of uppercase letters, lowercase letters, numbers, and symbols.";
  }

  if (password !== confirmation) {
    return "Passwords do not match.";
  }

  return null;
}
