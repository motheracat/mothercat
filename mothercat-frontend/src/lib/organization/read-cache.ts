const defaultTtlMs = 5 * 60_000;
const maxEntries = 100;
const storageKey = "mothercat:organization-read-cache:v1";

type CacheEntry = {
  value: unknown;
  expiresAt: number;
};

const entries = new Map<string, CacheEntry>();
const inFlight = new Map<string, Promise<unknown>>();
let generation = 0;
let restored = false;

function isClient(): boolean {
  return typeof window !== "undefined";
}

function removeExpiredEntries(now: number): void {
  for (const [key, entry] of entries) {
    if (entry.expiresAt <= now) entries.delete(key);
  }
}

function restoreEntries(): void {
  if (!isClient() || restored) return;
  restored = true;

  try {
    const serialized = window.sessionStorage.getItem(storageKey);
    if (!serialized) return;

    const stored: unknown = JSON.parse(serialized);
    if (typeof stored !== "object" || stored === null || Array.isArray(stored)) {
      window.sessionStorage.removeItem(storageKey);
      return;
    }

    const now = Date.now();
    for (const [key, value] of Object.entries(stored)) {
      if (
        typeof value === "object" &&
        value !== null &&
        "expiresAt" in value &&
        typeof value.expiresAt === "number" &&
        value.expiresAt > now &&
        "value" in value
      ) {
        entries.set(key, value as CacheEntry);
      }
    }
    removeExpiredEntries(now);
    while (entries.size > maxEntries) {
      const oldestKey = entries.keys().next().value;
      if (oldestKey === undefined) break;
      entries.delete(oldestKey);
    }
    persistEntries();
  } catch {
    entries.clear();
    try {
      window.sessionStorage.removeItem(storageKey);
    } catch {
      // Storage can be unavailable in restricted browser contexts.
    }
  }
}

function persistEntries(): void {
  if (!isClient()) return;
  try {
    window.sessionStorage.setItem(storageKey, JSON.stringify(Object.fromEntries(entries)));
  } catch {
    // Keep the in-memory cache available if storage is disabled or full.
  }
}

export function getCachedRead<T>(key: string): T | undefined {
  if (!isClient()) return undefined;
  restoreEntries();
  const entry = entries.get(key);
  if (!entry) return undefined;
  if (entry.expiresAt <= Date.now()) {
    entries.delete(key);
    return undefined;
  }
  entries.delete(key);
  entries.set(key, entry);
  return entry.value as T;
}

export function cacheRead<T>(
  key: string,
  load: () => Promise<T>,
  ttlMs = defaultTtlMs,
): Promise<T> {
  if (!isClient()) return load();

  const cached = getCachedRead<T>(key);
  if (cached !== undefined) return Promise.resolve(cached);

  const existingRequest = inFlight.get(key);
  if (existingRequest) return existingRequest as Promise<T>;

  const requestGeneration = generation;
  const request = load()
    .then((value) => {
      if (requestGeneration === generation) {
        const now = Date.now();
        removeExpiredEntries(now);
        entries.delete(key);
        entries.set(key, { value, expiresAt: now + ttlMs });
        while (entries.size > maxEntries) {
          const oldestKey = entries.keys().next().value;
          if (oldestKey === undefined) break;
          entries.delete(oldestKey);
        }
        persistEntries();
      }
      return value;
    })
    .finally(() => {
      if (inFlight.get(key) === request) inFlight.delete(key);
    });

  inFlight.set(key, request);
  return request;
}

export function invalidateReadCache(): void {
  generation += 1;
  entries.clear();
  inFlight.clear();
  restored = true;
  if (isClient()) {
    try {
      window.sessionStorage.removeItem(storageKey);
    } catch {
      // Cache invalidation still applies to in-memory entries if storage is unavailable.
    }
  }
}

export function awaitReadWithSignal<T>(promise: Promise<T>, signal?: AbortSignal): Promise<T> {
  if (!signal) return promise;
  if (signal.aborted) return Promise.reject(new DOMException("The operation was aborted.", "AbortError"));

  return new Promise<T>((resolve, reject) => {
    const abort = () => {
      signal.removeEventListener("abort", abort);
      reject(new DOMException("The operation was aborted.", "AbortError"));
    };
    signal.addEventListener("abort", abort, { once: true });
    promise.then(
      (value) => {
        signal.removeEventListener("abort", abort);
        resolve(value);
      },
      (error: unknown) => {
        signal.removeEventListener("abort", abort);
        reject(error);
      },
    );
  });
}
