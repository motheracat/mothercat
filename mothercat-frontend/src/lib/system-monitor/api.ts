import { buildApiUrl } from "@/lib/api/config";

export type MonitorOverview = {
  started_at: string;
  uptime_seconds: number;
  database_status: string;
  database: {
    open_connections: number;
    in_use: number;
    idle: number;
    wait_count: number;
    wait_duration_ms: number;
  };
  endpoint_count: number;
  analytics_pending_buckets: number;
  analytics_dropped_requests: number;
};

export type MonitorEndpoint = {
  method: string;
  route: string;
  description: string;
  controller: string;
  pausable: boolean;
  health: string;
  requests: number;
  failed_requests: number;
  error_rate: number;
  last_status?: number;
  last_request_at?: string;
  status_distribution: Record<string, number>;
  latency_buckets: Record<string, number>;
  paused: boolean;
  paused_until?: string;
  pause_status?: number;
  pause_message?: string;
};

export type MonitorEvent = {
  occurred_at: string;
  type: string;
  method: string;
  route: string;
  status: number;
  duration_ms: number;
};

export type AnalyticsPoint = {
  bucket: string;
  requests: number;
  successes: number;
  client_errors: number;
  server_errors: number;
  error_rate: number;
  average_latency_ms: number;
  p95_latency_ms: number;
  maximum_latency_ms: number;
  status_distribution: Record<string, number>;
  latency_distribution: Record<string, number>;
};

export type SecurityStatus = { mfa_enabled: boolean };
export type MonitorSession = {
  id: string;
  created_at: string;
  expires_at: string;
  current: boolean;
};
export type MonitorSecurityEvent = {
  id: number;
  account_id?: string;
  event_type: string;
  severity: string;
  details: unknown;
  occurred_at: string;
};
export type MonitorAlert = {
  source: string;
  event_type: string;
  severity: string;
  account_id?: string;
  failure_count?: number;
  occurred_at: string;
};
export type MonitorAuditEvent = {
  actor_type: string;
  actor_id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  details: unknown;
  occurred_at: string;
};
export type MonitorGrant = {
  account_id: string;
  granted_by: string;
  reason: string;
  granted_at: string;
  expires_at: string;
};
export type SecurityConsole = {
  security: SecurityStatus;
  sessions: MonitorSession[];
  events: MonitorSecurityEvent[];
  alerts: MonitorAlert[];
  audit: MonitorAuditEvent[];
  grants: MonitorGrant[];
  device_metadata_available: false;
};
export type MFAEnrollment = {
  secret: string;
  otpauth_uri: string;
  expires_at: string;
};

export class SystemMonitorApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
  ) {
    super(message);
    this.name = "SystemMonitorApiError";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function parseRecord(value: unknown, description: string): Record<string, unknown> {
  if (!isRecord(value)) throw new Error(`The System Monitor returned an invalid ${description}.`);
  return value;
}

function parseArray<T>(value: unknown, description: string): T[] {
  if (value === null || value === undefined) return [];
  if (!Array.isArray(value)) throw new Error(`The System Monitor returned invalid ${description}.`);
  return value as T[];
}

async function request(path: string, init: RequestInit = {}): Promise<unknown> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body !== undefined) headers.set("Content-Type", "application/json");

  let response: Response;
  try {
    response = await fetch(buildApiUrl(path), {
      ...init,
      headers,
      credentials: "include",
      cache: "no-store",
    });
  } catch {
    throw new SystemMonitorApiError("Could not reach the System Monitor service.", 0, "network_error");
  }

  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    if (response.ok) throw new Error("The System Monitor returned an invalid response.");
  }
  if (!response.ok) {
    const body = isRecord(payload) && isRecord(payload.error) ? payload.error : undefined;
    const code = typeof body?.code === "string" ? body.code : "request_failed";
    const message = response.status === 403
      ? "An active independent System Monitor grant is required."
      : response.status === 401
        ? "Your session has expired. Sign in again."
        : code === "mfa_verification_required"
          ? "Enter a current System Monitor MFA code."
          : code === "mfa_verification_failed"
            ? "The MFA code is invalid, expired, or already used."
            : `System Monitor request failed (${code}).`;
    throw new SystemMonitorApiError(message, response.status, code);
  }
  return payload;
}

export async function getMonitorOverview(): Promise<MonitorOverview> {
  const payload = parseRecord(await request("/system-monitor/overview"), "overview");
  return parseRecord(payload.overview, "overview") as unknown as MonitorOverview;
}

export async function getMonitorEndpoints(): Promise<MonitorEndpoint[]> {
  const payload = parseRecord(await request("/system-monitor/endpoints"), "endpoint response");
  return parseArray<MonitorEndpoint>(payload.endpoints, "endpoint list");
}

export async function getMonitorEvents(): Promise<MonitorEvent[]> {
  const payload = parseRecord(await request("/system-monitor/events?limit=100"), "event response");
  return parseArray<MonitorEvent>(payload.events, "event list");
}

export async function getEndpointAnalytics(method: string, route: string): Promise<AnalyticsPoint[]> {
  const query = new URLSearchParams({
    method,
    route,
    interval: "hour",
    from: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
    to: new Date().toISOString(),
  });
  const payload = parseRecord(await request(`/system-monitor/analytics?${query.toString()}`), "analytics response");
  const report = parseRecord(payload.analytics, "analytics report");
  return parseArray<AnalyticsPoint>(report.points, "analytics points");
}

export async function getMonitorHistory() {
  const payload = parseRecord(await request("/system-monitor/endpoints/history?limit=50"), "history response");
  return parseArray<Record<string, unknown>>(payload.history, "control history");
}

export async function getMonitorSecurityStatus(): Promise<SecurityStatus> {
  const payload = parseRecord(await request("/system-monitor/security/status"), "security status response");
  return parseRecord(payload.security, "security status") as unknown as SecurityStatus;
}

export async function getSecurityConsole(code: string): Promise<SecurityConsole> {
  const payload = parseRecord(await request("/system-monitor/security/console", {
    headers: { "X-System-Monitor-MFA": code },
  }), "security console response");
  return {
    security: parseRecord(payload.security, "security status") as unknown as SecurityStatus,
    sessions: parseArray<MonitorSession>(payload.sessions, "session list"),
    events: parseArray<MonitorSecurityEvent>(payload.events, "security events"),
    alerts: parseArray<MonitorAlert>(payload.alerts, "security alerts"),
    audit: parseArray<MonitorAuditEvent>(payload.audit, "audit events"),
    grants: parseArray<MonitorGrant>(payload.grants, "access grants"),
    device_metadata_available: false,
  };
}

export async function beginMFAEnrollment(password: string): Promise<MFAEnrollment> {
  const payload = parseRecord(await request("/system-monitor/security/mfa/enroll", {
    method: "POST",
    headers: { "X-System-Monitor-Action": "confirm" },
    body: JSON.stringify({ password }),
  }), "MFA enrollment response");
  return parseRecord(payload.enrollment, "MFA enrollment") as unknown as MFAEnrollment;
}

export async function confirmMFAEnrollment(code: string): Promise<void> {
  await request("/system-monitor/security/mfa/confirm", {
    method: "POST",
    headers: { "X-System-Monitor-Action": "confirm" },
    body: JSON.stringify({ code }),
  });
}

export async function setEndpointControl(
  action: "pause" | "resume",
  payload: Record<string, string | number>,
  mfaCode: string,
): Promise<void> {
  await request(`/system-monitor/endpoints/${action}`, {
    method: "POST",
    headers: {
      "X-System-Monitor-Action": "confirm",
      "X-System-Monitor-MFA": mfaCode,
    },
    body: JSON.stringify(payload),
  });
}

export async function revokeMonitorSession(sessionId: string, reason: string, mfaCode: string): Promise<void> {
  await request("/system-monitor/security/sessions/revoke", {
    method: "POST",
    headers: {
      "X-System-Monitor-Action": "confirm",
      "X-System-Monitor-MFA": mfaCode,
    },
    body: JSON.stringify({ session_id: sessionId, reason }),
  });
}

export async function disableMonitorMFA(reason: string, mfaCode: string): Promise<void> {
  await request("/system-monitor/security/mfa/disable", {
    method: "POST",
    headers: {
      "X-System-Monitor-Action": "confirm",
    },
    body: JSON.stringify({ reason, code: mfaCode }),
  });
}
