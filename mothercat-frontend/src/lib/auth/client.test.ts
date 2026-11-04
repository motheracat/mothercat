import { afterEach, describe, expect, it, vi } from "vitest";

import { login } from "@/lib/auth/client";

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("auth API client", () => {
  it("retries a transient login failure before returning the session", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(null, { status: 503 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: {
          id: "user-1",
          staff_id: "12345",
          display_name: "Test User",
          email: "test@example.com",
          role: "Staff",
          roles: ["Staff"],
          permissions: ["staff.read"],
        },
      }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const request = login({ staffId: "12345", password: "example-password" });
    await vi.advanceTimersByTimeAsync(300);

    await expect(request).resolves.toMatchObject({
      userId: "user-1",
      staffId: "12345",
      displayName: "Test User",
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
