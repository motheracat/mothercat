import { afterEach, describe, expect, it, vi } from "vitest";

import {
  cacheRead,
  getCachedRead,
  invalidateReadCache,
} from "@/lib/organization/read-cache";

afterEach(() => {
  invalidateReadCache();
  window.sessionStorage.removeItem("mothercat:organization-read-cache:v1");
  vi.useRealTimers();
});

describe("organization read cache", () => {
  it("expires cached reads after their time-to-live", async () => {
    vi.useFakeTimers();
    const load = vi.fn()
      .mockResolvedValueOnce("first")
      .mockResolvedValueOnce("second");

    await expect(cacheRead("test", load, 1_000)).resolves.toBe("first");
    await expect(cacheRead("test", load, 1_000)).resolves.toBe("first");
    expect(load).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(1_001);
    await expect(cacheRead("test", load, 1_000)).resolves.toBe("second");
    expect(load).toHaveBeenCalledTimes(2);
  });

  it("does not restore data from a request invalidated while it was pending", async () => {
    let finishRequest!: (value: string) => void;
    const pending = cacheRead("test", () => new Promise((resolve) => { finishRequest = resolve; }));

    invalidateReadCache();
    finishRequest("old session data");
    await pending;

    expect(getCachedRead("test")).toBeUndefined();
  });

  it("reuses a cached read after the page module is reloaded", async () => {
    const firstLoad = vi.fn().mockResolvedValue({ value: "kept for this tab" });
    await expect(cacheRead("persisted", firstLoad)).resolves.toEqual({ value: "kept for this tab" });
    expect(window.sessionStorage.getItem("mothercat:organization-read-cache:v1")).toContain("kept for this tab");

    vi.resetModules();
    const { cacheRead: cacheAfterReload } = await import("@/lib/organization/read-cache");
    const secondLoad = vi.fn().mockResolvedValue({ value: "fresh request" });

    await expect(cacheAfterReload("persisted", secondLoad)).resolves.toEqual({ value: "kept for this tab" });
    expect(secondLoad).not.toHaveBeenCalled();
  });
});
