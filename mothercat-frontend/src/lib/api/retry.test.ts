import { afterEach, describe, expect, it, vi } from "vitest";

import { fetchWithRetry } from "@/lib/api/retry";

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("fetchWithRetry", () => {
  it("retries transient GET responses before returning them", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(null, { status: 503 }))
      .mockResolvedValueOnce(new Response("ok", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const request = fetchWithRetry("/api/v1/staff");
    await vi.advanceTimersByTimeAsync(300);

    await expect(request).resolves.toMatchObject({ status: 200 });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("retries network failures for reads", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn()
      .mockRejectedValueOnce(new TypeError("fetch failed"))
      .mockResolvedValueOnce(new Response("ok", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const request = fetchWithRetry("/api/v1/staff");
    await vi.advanceTimersByTimeAsync(300);

    await expect(request).resolves.toMatchObject({ status: 200 });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not automatically retry unsafe requests", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 503 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchWithRetry("/api/v1/staff", { method: "POST" }))
      .resolves.toMatchObject({ status: 503 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("allows an explicitly opted-in unsafe request to retry transient failures", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(null, { status: 503 }))
      .mockResolvedValueOnce(new Response("ok", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const request = fetchWithRetry(
      "/api/v1/auth/login",
      { method: "POST", body: JSON.stringify({ staff_id: "12345" }) },
      { retryUnsafeRequests: true },
    );
    await vi.advanceTimersByTimeAsync(300);

    await expect(request).resolves.toMatchObject({ status: 200 });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
