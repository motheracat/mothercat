const maxAttempts = 3;
const initialRetryDelayMs = 300;
const retryableStatuses = new Set([502, 503, 504]);

type RetryOptions = {
  retryUnsafeRequests?: boolean;
  timeoutMs?: number;
};

function waitBeforeRetry(delayMs: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException("The request was aborted.", "AbortError"));
      return;
    }

    const timeout = setTimeout(() => {
      signal?.removeEventListener("abort", abort);
      resolve();
    }, delayMs);
    const abort = () => {
      clearTimeout(timeout);
      signal?.removeEventListener("abort", abort);
      reject(new DOMException("The request was aborted.", "AbortError"));
    };
    signal?.addEventListener("abort", abort, { once: true });
  });
}

export async function fetchWithRetry(
  input: RequestInfo | URL,
  init: RequestInit = {},
  options: RetryOptions = {},
): Promise<Response> {
  const method = (init.method ?? (input instanceof Request ? input.method : "GET")).toUpperCase();
  const canRetry = method === "GET" || options.retryUnsafeRequests === true;
  const attempts = canRetry ? maxAttempts : 1;
  const signal = init.signal ?? (input instanceof Request ? input.signal : undefined);

  for (let attempt = 0; attempt < attempts; attempt += 1) {
    if (signal?.aborted) {
      throw new DOMException("The request was aborted.", "AbortError");
    }

    const controller = new AbortController();
    const abortFromCaller = () => controller.abort();
    signal?.addEventListener("abort", abortFromCaller, { once: true });
    const timeout = options.timeoutMs === undefined
      ? undefined
      : setTimeout(() => controller.abort(), options.timeoutMs);

    try {
      const response = await fetch(input, { ...init, signal: controller.signal });
      if (!canRetry || !retryableStatuses.has(response.status) || attempt === attempts - 1) {
        return response;
      }
      await response.body?.cancel();
    } catch (error) {
      if (signal?.aborted || attempt === attempts - 1) {
        throw error;
      }
    } finally {
      if (timeout !== undefined) clearTimeout(timeout);
      signal?.removeEventListener("abort", abortFromCaller);
    }

    await waitBeforeRetry(initialRetryDelayMs * 2 ** attempt, signal);
  }

  throw new Error("The request could not be completed.");
}
