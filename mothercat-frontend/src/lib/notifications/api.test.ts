import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { listNotifications, markNotificationRead, replaceNotificationPreferences } from "@/lib/notifications/api";

const fetchMock = vi.fn<typeof fetch>();

function jsonResponse(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("notification API client", () => {
  it("lists notifications and accepts optional resource/read fields", async () => {
    fetchMock.mockResolvedValue(jsonResponse({
      notifications: [{
        id: "notification-uuid",
        event_type: "task.assigned",
        title: "Task assigned",
        body: "A task was assigned to you.",
        created_at: "2026-11-07T12:00:00Z",
      }],
      page: { limit: 20, offset: 0, total: 1 },
    }));

    const result = await listNotifications({ unread: true, limit: 20, offset: 0 });

    expect(result.notifications[0]).toMatchObject({
      id: "notification-uuid",
      event_type: "task.assigned",
      resource: null,
      read_at: null,
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/notifications?unread=true&limit=20&offset=0",
      expect.objectContaining({ credentials: "include", cache: "no-store" }),
    );
  });

  it("handles a 204 mark-read response without trying to parse JSON", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));

    await expect(markNotificationRead("notification-uuid")).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/notifications/notification-uuid/read",
      expect.objectContaining({ method: "POST", credentials: "include" }),
    );
  });

  it("replaces the full set of notification preferences", async () => {
    const preferences = [{ event_type: "task.assigned" as const, enabled: false }];
    fetchMock.mockResolvedValue(jsonResponse({ preferences }));

    await expect(replaceNotificationPreferences(preferences)).resolves.toEqual(preferences);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/notifications/preferences",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ preferences }),
      }),
    );
  });
});
