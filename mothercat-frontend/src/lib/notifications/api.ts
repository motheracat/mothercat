import { buildApiUrl } from "@/lib/api/config";
import { apiRoutes } from "@/lib/api/routes";
import { fetchWithRetry } from "@/lib/api/retry";

export const notificationEventTypes = [
  "task.assigned",
  "task.submitted",
  "task.approved",
  "task.changes_requested",
  "task.rejected",
  "task.comment_added",
  "task.due_soon",
  "task.overdue",
] as const;
export const notificationsUpdatedEvent = "mothercat:notifications-updated";
export type NotificationEventType = (typeof notificationEventTypes)[number];

export type AppNotification = {
  id: string;
  event_type: NotificationEventType;
  title: string;
  body: string;
  resource: { type: string; id: string } | null;
  created_at: string;
  read_at: string | null;
};

export type NotificationPreference = {
  event_type: NotificationEventType;
  enabled: boolean;
};

export class NotificationApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "NotificationApiError";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

async function request(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body !== undefined) headers.set("Content-Type", "application/json");
  let response: Response;
  try {
    response = await fetchWithRetry(buildApiUrl(path), {
      ...init,
      headers,
      credentials: "include",
      cache: "no-store",
    });
  } catch {
    throw new NotificationApiError("Could not reach notifications. Check your connection and try again.", 0, "network_error");
  }
  if (!response.ok) {
    let payload: unknown;
    try {
      payload = await response.json();
    } catch {
      payload = null;
    }
    const error = isRecord(payload) && isRecord(payload.error) ? payload.error : undefined;
    throw new NotificationApiError(
      typeof error?.message === "string" ? error.message : "The request could not be completed.",
      response.status,
      typeof error?.code === "string" ? error.code : "request_failed",
      typeof error?.request_id === "string" ? error.request_id : response.headers.get("X-Request-ID") ?? undefined,
    );
  }
  return response;
}

async function readJson(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    throw new NotificationApiError("Notifications returned an invalid response.", 502, "invalid_response");
  }
}

function parseNotification(value: unknown): AppNotification {
  if (
    !isRecord(value) ||
    typeof value.id !== "string" ||
    typeof value.event_type !== "string" ||
    !notificationEventTypes.includes(value.event_type as NotificationEventType) ||
    typeof value.title !== "string" ||
    typeof value.body !== "string" ||
    typeof value.created_at !== "string" ||
    (value.read_at !== undefined && value.read_at !== null && typeof value.read_at !== "string")
  ) {
    throw new NotificationApiError("Notifications returned an invalid item.", 502, "invalid_response");
  }
  let resource: AppNotification["resource"] = null;
  if (value.resource !== undefined && value.resource !== null) {
    if (!isRecord(value.resource) || typeof value.resource.type !== "string" || typeof value.resource.id !== "string") {
      throw new NotificationApiError("Notifications returned an invalid resource.", 502, "invalid_response");
    }
    resource = { type: value.resource.type, id: value.resource.id };
  }
  return {
    id: value.id,
    event_type: value.event_type as NotificationEventType,
    title: value.title,
    body: value.body,
    resource,
    created_at: value.created_at,
    read_at: typeof value.read_at === "string" ? value.read_at : null,
  };
}

export async function listNotifications(
  params: { unread?: boolean; limit?: number; offset?: number } = {},
  signal?: AbortSignal,
): Promise<{ notifications: AppNotification[]; page: { limit: number; offset: number; total: number } }> {
  const query = new URLSearchParams({
    unread: String(params.unread ?? false),
    limit: String(params.limit ?? 50),
    offset: String(params.offset ?? 0),
  });
  const response = await request(`${apiRoutes.notifications.list}?${query}`, { signal });
  const payload = await readJson(response);
  if (
    !isRecord(payload) ||
    !Array.isArray(payload.notifications) ||
    !isRecord(payload.page) ||
    typeof payload.page.limit !== "number" ||
    typeof payload.page.offset !== "number" ||
    typeof payload.page.total !== "number"
  ) {
    throw new NotificationApiError("Notifications returned an invalid list.", 502, "invalid_response");
  }
  return {
    notifications: payload.notifications.map(parseNotification),
    page: { limit: payload.page.limit, offset: payload.page.offset, total: payload.page.total },
  };
}

export async function getUnreadNotificationCount(signal?: AbortSignal): Promise<number> {
  const payload = await readJson(await request(apiRoutes.notifications.unreadCount, { signal }));
  if (!isRecord(payload) || typeof payload.unread_count !== "number" || !Number.isSafeInteger(payload.unread_count) || payload.unread_count < 0) {
    throw new NotificationApiError("Notifications returned an invalid unread count.", 502, "invalid_response");
  }
  return payload.unread_count;
}

export async function markNotificationRead(notificationId: string): Promise<void> {
  await request(apiRoutes.notifications.read(encodeURIComponent(notificationId)), { method: "POST" });
}

export async function markAllNotificationsRead(): Promise<number> {
  const payload = await readJson(await request(apiRoutes.notifications.readAll, { method: "POST" }));
  if (!isRecord(payload) || typeof payload.marked_read !== "number") {
    throw new NotificationApiError("Notifications returned an invalid mark-all response.", 502, "invalid_response");
  }
  return payload.marked_read;
}

export async function getNotificationPreferences(signal?: AbortSignal): Promise<NotificationPreference[]> {
  const payload = await readJson(await request(apiRoutes.notifications.preferences, { signal }));
  if (!isRecord(payload) || !Array.isArray(payload.preferences)) {
    throw new NotificationApiError("Notifications returned invalid preferences.", 502, "invalid_response");
  }
  const preferences = payload.preferences.map((item): NotificationPreference => {
    if (
      !isRecord(item) ||
      typeof item.event_type !== "string" ||
      !notificationEventTypes.includes(item.event_type as NotificationEventType) ||
      typeof item.enabled !== "boolean"
    ) {
      throw new NotificationApiError("Notifications returned invalid preferences.", 502, "invalid_response");
    }
    return { event_type: item.event_type as NotificationEventType, enabled: item.enabled };
  });
  return notificationEventTypes.map((eventType) => (
    preferences.find((item) => item.event_type === eventType) ?? { event_type: eventType, enabled: true }
  ));
}

export async function replaceNotificationPreferences(preferences: NotificationPreference[]): Promise<NotificationPreference[]> {
  const payload = await readJson(await request(apiRoutes.notifications.preferences, {
    method: "PUT",
    body: JSON.stringify({ preferences }),
  }));
  if (!isRecord(payload) || !Array.isArray(payload.preferences)) {
    throw new NotificationApiError("Notifications returned invalid preferences.", 502, "invalid_response");
  }
  return payload.preferences.map((item): NotificationPreference => {
    if (
      !isRecord(item) ||
      typeof item.event_type !== "string" ||
      !notificationEventTypes.includes(item.event_type as NotificationEventType) ||
      typeof item.enabled !== "boolean"
    ) {
      throw new NotificationApiError("Notifications returned invalid preferences.", 502, "invalid_response");
    }
    return { event_type: item.event_type as NotificationEventType, enabled: item.enabled };
  });
}

export function formatNotificationError(error: unknown, fallback: string): string {
  if (!(error instanceof NotificationApiError)) return fallback;
  if (error.code === "origin_not_allowed" || error.code === "invalid_origin") {
    return "This action was blocked by the site security check. Refresh and try again.";
  }
  if (error.code === "unauthorized" || error.status === 401) return "Your session expired. Sign in again.";
  if (error.code === "permission_denied" || error.status === 403) return "You don’t have permission to manage notifications.";
  if (error.status === 503) return "Notifications are temporarily unavailable. Try again later.";
  return error.requestId ? `${fallback} (Request ID: ${error.requestId})` : fallback;
}
