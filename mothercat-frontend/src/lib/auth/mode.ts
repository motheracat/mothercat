import "server-only";

export function isDevOpen(): boolean {
  return (process.env.AUTH_MODE ?? "strict") === "dev-open";
}

export function isStrict(): boolean {
  return !isDevOpen();
}
