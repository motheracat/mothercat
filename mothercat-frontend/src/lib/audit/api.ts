import { buildApiUrl } from "@/lib/api/config";
import { apiRoutes } from "@/lib/api/routes";

export type AuditEvent = {
  id: string;
  actor_type: string;
  actor_id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  occurred_at: string;
  details: Record<string, unknown>;
};

export type AuditEventFilter = {
  actorId?: string;
  action?: string;
  resourceType?: string;
  resourceId?: string;
  from?: string;
  to?: string;
  limit: number;
  offset: number;
};

export type AuditEventPage = {
  events: AuditEvent[];
  total: number;
  limit: number;
  offset: number;
};

export class AuditApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "AuditApiError";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

async function request(path: string, signal?: AbortSignal): Promise<unknown> {
  let response: Response;
  try {
    response = await fetch(buildApiUrl(path), {
      headers: { Accept: "application/json" },
      credentials: "include",
      cache: "no-store",
      signal,
    });
  } catch {
    if (signal?.aborted) throw new DOMException("The request was aborted.", "AbortError");
    throw new AuditApiError(
      "Could not reach the audit service. Check your connection and try again.",
      0,
      "network_error",
    );
  }

  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    if (response.ok) {
      throw new AuditApiError("The audit service returned an invalid response.", response.status, "invalid_response");
    }
    payload = null;
  }
  if (!response.ok) {
    const error = isRecord(payload) && isRecord(payload.error) ? payload.error : undefined;
    throw new AuditApiError(
      error && typeof error.message === "string"
        ? error.message
        : response.status === 401
          ? "Your session has expired. Sign in again."
          : response.status === 403
            ? "Your account is not permitted to view audit events."
            : "The request could not be completed.",
      response.status,
      error && typeof error.code === "string" ? error.code : "request_failed",
      error && typeof error.request_id === "string" ? error.request_id : undefined,
    );
  }
  return payload;
}

function parseAuditEvent(value: unknown): AuditEvent {
  if (
    !isRecord(value) ||
    typeof value.id !== "string" ||
    typeof value.actor_type !== "string" ||
    typeof value.actor_id !== "string" ||
    typeof value.action !== "string" ||
    typeof value.resource_type !== "string" ||
    typeof value.resource_id !== "string" ||
    typeof value.occurred_at !== "string" ||
    !isRecord(value.details)
  ) {
    throw new AuditApiError("The audit service returned an invalid event.", 502, "invalid_response");
  }
  return {
    id: value.id,
    actor_type: value.actor_type,
    actor_id: value.actor_id,
    action: value.action,
    resource_type: value.resource_type,
    resource_id: value.resource_id,
    occurred_at: value.occurred_at,
    details: value.details,
  };
}

export async function listAuditEvents(
  filter: AuditEventFilter,
  signal?: AbortSignal,
): Promise<AuditEventPage> {
  const query = new URLSearchParams({
    limit: String(filter.limit),
    offset: String(filter.offset),
  });
  if (filter.actorId) query.set("actor_id", filter.actorId);
  if (filter.action) query.set("action", filter.action);
  if (filter.resourceType) query.set("resource_type", filter.resourceType);
  if (filter.resourceId) query.set("resource_id", filter.resourceId);
  if (filter.from) query.set("from", filter.from);
  if (filter.to) query.set("to", filter.to);

  const payload = await request(`${apiRoutes.audit.list}?${query}`, signal);
  if (
    !isRecord(payload) ||
    !Array.isArray(payload.events) ||
    !isRecord(payload.page) ||
    typeof payload.page.limit !== "number" ||
    typeof payload.page.offset !== "number" ||
    typeof payload.page.total !== "number"
  ) {
    throw new AuditApiError("The audit service returned an invalid event list.", 502, "invalid_response");
  }
  return {
    events: payload.events.map(parseAuditEvent),
    total: payload.page.total,
    limit: payload.page.limit,
    offset: payload.page.offset,
  };
}

export async function getAuditEvent(id: string, signal?: AbortSignal): Promise<AuditEvent> {
  const payload = await request(apiRoutes.audit.details(encodeURIComponent(id)), signal);
  if (!isRecord(payload) || !("event" in payload)) {
    throw new AuditApiError("The audit service returned an invalid event.", 502, "invalid_response");
  }
  return parseAuditEvent(payload.event);
}

export function formatAuditError(error: unknown, fallback: string): string {
  if (!(error instanceof AuditApiError)) return fallback;
  if (error.status === 401) return "Your session has expired. Sign in again.";
  if (error.status === 403) {
    return "Your account isn't authorized to view audit events. Ask an administrator to verify your audit.read permission.";
  }
  if (error.status === 0) return error.message;
  return fallback;
}
