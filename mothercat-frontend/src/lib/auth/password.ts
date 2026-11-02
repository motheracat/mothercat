export function validateNewPassword(password: string, confirmation: string): string | null {
  const byteLength = new TextEncoder().encode(password).length;
  if (byteLength < 14 || byteLength > 72) {
    return "Password must be 14–72 bytes long.";
  }

  const characterClasses = [
    /\p{Lu}/u.test(password),
    /\p{Ll}/u.test(password),
    /\p{Nd}/u.test(password),
    /[^\p{Lu}\p{Ll}\p{Nd}]/u.test(password),
  ].filter(Boolean).length;

  if (characterClasses < 3) {
    return "Password must include at least three of uppercase letters, lowercase letters, numbers, and symbols.";
  }

  if (password !== confirmation) {
    return "Passwords do not match.";
  }

  return null;
}
