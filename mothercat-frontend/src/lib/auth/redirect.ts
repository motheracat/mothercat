export function isSafeRedirectPath(path: string | null | undefined): path is string {
  if (!path) {
    return false;
  }
  // Only allow relative paths, block open redirects
  if (path.startsWith("//")) {
    return false;
  }
  if (path.includes("://")) {
    return false;
  }
  if (!path.startsWith("/")) {
    return false;
  }
  return true;
}

export function getSafeRedirectPath(path: string | null | undefined, fallback = "/dashboard"): string {
  if (isSafeRedirectPath(path)) {
    return path;
  }
  return fallback;
}
