import { afterEach, describe, expect, it, vi } from "vitest";

import {
  AuditApiError,
  formatAuditError,
  getAuditEvent,
  listAuditEvents,
} from "@/lib/audit/api";

const event = {
  id: "00000000-0000-4000-8000-000000000001",
  actor_type: "user",
  actor_id: "account-1",
  action: "staff.updated",
  resource_type: "staff_member",
  resource_id: "staff-1",
  occurred_at: "2026-11-04T10:00:00Z",
  details: { field: "job_title" },
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("Audit API", () => {
  it("requests filtered, paginated events with browser session credentials", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      events: [event],
      page: { limit: 25, offset: 50, total: 76 },
    }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listAuditEvents({
      limit: 25,
      offset: 50,
      actorId: "account-1",
      action: "staff.updated",
      resourceType: "staff_member",
      resourceId: "staff-1",
      from: "2026-11-01T00:00:00.000Z",
      to: "2026-11-04T00:00:00.000Z",
    })).resolves.toEqual({
      events: [event],
      total: 76,
      limit: 25,
      offset: 50,
    });

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe(
      "/api/v1/audit-events?limit=25&offset=50&actor_id=account-1&action=staff.updated&resource_type=staff_member&resource_id=staff-1&from=2026-11-01T00%3A00%3A00.000Z&to=2026-11-04T00%3A00%3A00.000Z",
    );
    expect(init.credentials).toBe("include");
    expect(init.cache).toBe("no-store");
    expect(new Headers(init.headers).get("Accept")).toBe("application/json");
  });

  it("fetches one event by encoded ID", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ event }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getAuditEvent(event.id)).resolves.toEqual(event);
    expect(fetchMock).toHaveBeenCalledWith(
      `/api/v1/audit-events/${event.id}`,
      expect.objectContaining({ credentials: "include", cache: "no-store" }),
    );
  });

  it("preserves backend authorization code and request ID", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { code: "permission_denied", request_id: "request-123" },
    }), { status: 403 })));

    await expect(listAuditEvents({ limit: 25, offset: 0 })).rejects.toMatchObject({
      status: 403,
      code: "permission_denied",
      requestId: "request-123",
    } satisfies Partial<AuditApiError>);
  });

  it("explains that development-open mode still requires a real backend session", () => {
    const error = new AuditApiError("unauthorized", 401, "unauthorized");
    expect(formatAuditError(error, "Unable to load audit events.", true)).toContain(
      "Development-open mode uses a mock workspace session.",
    );
    expect(formatAuditError(error, "Unable to load audit events.")).toBe(
      "Your session has expired. Sign in again.",
    );
  });

  it("explains when the frontend cannot reach the backend", () => {
    const error = new AuditApiError("upstream unavailable", 503, "upstream_unavailable");
    expect(formatAuditError(error, "Unable to load audit events.")).toBe(
      "The audit API isn't reachable. Start the backend and verify API_BASE_URL, then retry.",
    );
  });

  it("rejects malformed event response data", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      event: { ...event, details: "not-an-object" },
    }), { status: 200 })));

    await expect(getAuditEvent(event.id)).rejects.toMatchObject({
      status: 502,
      code: "invalid_response",
    });
  });
});
