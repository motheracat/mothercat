"use client";

import {
  Activity,
  AlertTriangle,
  ArrowDownRight,
  ArrowUpRight,
  CheckCircle2,
  ChevronRight,
  Clock3,
  Command,
  Database,
  LockKeyhole,
  RefreshCw,
  Server,
  ShieldAlert,
  ShieldCheck,
  TerminalSquare,
  Wifi,
  XCircle,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  beginMFAEnrollment,
  confirmMFAEnrollment,
  disableMonitorMFA,
  getEndpointAnalytics,
  getMonitorEndpoints,
  getMonitorEvents,
  getMonitorHistory,
  getMonitorOverview,
  getMonitorSecurityStatus,
  getSecurityConsole,
  revokeMonitorSession,
  setEndpointControl,
  SystemMonitorApiError,
  type AnalyticsPoint,
  type MFAEnrollment,
  type MonitorAlert,
  type MonitorEndpoint,
  type MonitorEvent,
  type MonitorGrant,
  type MonitorOverview,
  type MonitorSecurityEvent,
  type MonitorSession,
  type SecurityConsole,
} from "@/lib/system-monitor/api";

type TerminalEntry = { id: number; kind: "command" | "result" | "error"; text: string };
type PendingControl = {
  action: "pause" | "resume";
  method: string;
  route: string;
  reason: string;
};
type PageTab = "overview" | "endpoints" | "security" | "terminal";

function formatDuration(seconds: number): string {
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3_600);
  const minutes = Math.floor((seconds % 3_600) / 60);
  return [days > 0 ? `${days}d` : null, `${hours}h`, `${minutes}m`].filter(Boolean).join(" ");
}

function formatTime(value?: string): string {
  if (!value) return "No recent request";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "Unknown time" : date.toLocaleString();
}

function numberText(value: number | undefined): string {
  return typeof value === "number" && Number.isFinite(value) ? value.toLocaleString() : "—";
}

function statusTone(value: string): string {
  if (value === "healthy" || value === "enabled" || value === "healthy") return "monitor-tone-good";
  if (value === "degraded" || value === "warning" || value === "paused") return "monitor-tone-warn";
  if (value === "critical" || value === "unavailable" || value === "disabled") return "monitor-tone-bad";
  return "monitor-tone-neutral";
}

function parseCommand(command: string): { action: string; args: string[] } | null {
  const trimmed = command.trim();
  if (trimmed === "help" || trimmed === "status" || trimmed === "endpoints" || trimmed === "events") {
    return { action: trimmed, args: [] };
  }
  const analytics = trimmed.match(/^analytics ([A-Z]+) (\S+)$/);
  if (analytics) return { action: "analytics", args: [analytics[1], analytics[2]] };
  const control = trimmed.match(/^(pause|resume) ([A-Z]+) (\S+) ("(?:[^"\\]|\\.){1,300}")$/);
  if (control) {
    try {
      const reason: unknown = JSON.parse(control[4]);
      if (typeof reason === "string" && reason.trim().length > 0 && reason.length <= 300) {
        return { action: control[1], args: [control[2], control[3], reason] };
      }
    } catch {
      return null;
    }
  }
  return null;
}

function StatCard({
  label,
  value,
  detail,
  icon: Icon,
  tone,
}: {
  label: string;
  value: string;
  detail: string;
  icon: typeof Activity;
  tone: "cyan" | "blue" | "green" | "amber";
}) {
  return (
    <article className={`monitor-stat monitor-stat-${tone}`}>
      <div className="monitor-stat-top">
        <span>{label}</span>
        <Icon size={17} aria-hidden="true" />
      </div>
      <strong>{value}</strong>
      <small>{detail}</small>
    </article>
  );
}

function SeverityTag({ severity }: { severity: string }) {
  return <span className={`monitor-severity ${statusTone(severity)}`}>{severity}</span>;
}

function CommandPanel({
  overview,
  endpoints,
  events,
  onControl,
  mfaCode,
  setMfaCode,
  pauseHours,
  setPauseHours,
  responseStatus,
  setResponseStatus,
  responseMessage,
  setResponseMessage,
  busy,
  onClearPending,
}: {
  overview: MonitorOverview | null;
  endpoints: MonitorEndpoint[];
  events: MonitorEvent[];
  onControl: (pending: PendingControl, code: string, hours: number, responseStatus: number, responseMessage: string) => Promise<void>;
  mfaCode: string;
  setMfaCode: (value: string) => void;
  pauseHours: number;
  setPauseHours: (value: number) => void;
  responseStatus: number;
  setResponseStatus: (value: number) => void;
  responseMessage: string;
  setResponseMessage: (value: string) => void;
  busy: boolean;
  onClearPending: () => void;
}) {
  const [input, setInput] = useState("");
  const [entries, setEntries] = useState<TerminalEntry[]>([
    { id: 1, kind: "result", text: "SYSTEM MONITOR CONTROL INTERFACE · application operations only" },
    { id: 2, kind: "result", text: 'Type "help" to view the fixed command allowlist.' },
  ]);
  const [pending, setPending] = useState<PendingControl | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const nextId = useRef(3);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
  }, [entries, pending]);

  const append = (kind: TerminalEntry["kind"], text: string) => {
    setEntries((current) => [...current.slice(-70), { id: nextId.current++, kind, text }]);
  };

  async function submitCommand(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const command = input.trim();
    if (!command) return;
    setInput("");
    append("command", `monitor> ${command}`);
    const parsed = parseCommand(command);
    if (!parsed) {
      append("error", "Command rejected. Only exact allowlisted operations are accepted; no shell, SQL, or code execution is available.");
      return;
    }
    if (parsed.action === "help") {
      append("result", [
        "help · status · endpoints · events",
        "analytics METHOD ROUTE",
        'pause METHOD ROUTE "reason"',
        'resume METHOD ROUTE "reason"',
        "Pause/resume require a fresh MFA code and explicit confirmation.",
      ].join("\n"));
      return;
    }
    if (parsed.action === "status") {
      append("result", overview
        ? `API uptime ${formatDuration(overview.uptime_seconds)} · database ${overview.database_status} · ${overview.endpoint_count} catalogued endpoints`
        : "System overview is not available yet.");
      return;
    }
    if (parsed.action === "endpoints") {
      const summary = endpoints.slice(0, 20).map((endpoint) =>
        `${endpoint.paused ? "PAUSED" : endpoint.health.toUpperCase()} ${endpoint.method} ${endpoint.route}`,
      );
      append("result", summary.length ? summary.join("\n") : "No endpoint inventory is available.");
      return;
    }
    if (parsed.action === "events") {
      const summary = events.slice(-10).reverse().map((item) =>
        `${formatTime(item.occurred_at)} ${item.type} ${item.method} ${item.route} · ${item.status}`,
      );
      append("result", summary.length ? summary.join("\n") : "No recent process-local events.");
      return;
    }
    if (parsed.action === "analytics") {
      try {
        const [method, route] = parsed.args;
        const points = await getEndpointAnalytics(method, route);
        const requests = points.reduce((total, point) => total + point.requests, 0);
        const errors = points.reduce((total, point) => total + point.client_errors + point.server_errors, 0);
        const meanLatency = points.length
          ? points.reduce((total, point) => total + point.average_latency_ms, 0) / points.length
          : 0;
        append("result", `${method} ${route} · 24h requests ${numberText(requests)} · errors ${numberText(errors)} · mean bucket latency ${meanLatency.toFixed(1)} ms`);
      } catch (error) {
        append("error", error instanceof Error ? error.message : "Analytics request failed.");
      }
      return;
    }
    setPending({
      action: parsed.action as "pause" | "resume",
      method: parsed.args[0],
      route: parsed.args[1],
      reason: parsed.args[2],
    });
    onClearPending();
  }

  async function confirmControl() {
    if (!pending) return;
    try {
      await onControl(pending, mfaCode, pauseHours, responseStatus, responseMessage);
      append("result", `${pending.action.toUpperCase()} confirmed for ${pending.method} ${pending.route}.`);
      setPending(null);
      setMfaCode("");
    } catch (error) {
      append("error", error instanceof Error ? error.message : "The operation could not be completed.");
    }
  }

  return (
    <section className="monitor-terminal">
      <div className="monitor-terminal-top">
        <div className="flex items-center gap-2"><TerminalSquare size={16} /><span>CONTROL TERMINAL</span></div>
        <span className="monitor-terminal-safe"><LockKeyhole size={12} /> ALLOWLISTED</span>
      </div>
      <div ref={scrollRef} className="monitor-terminal-output" aria-live="polite">
        {entries.map((entry) => (
          <pre key={entry.id} className={`monitor-terminal-line monitor-terminal-${entry.kind}`}>{entry.text}</pre>
        ))}
        {pending ? (
          <div className="monitor-control-confirm">
            <div className="monitor-control-title">
              <AlertTriangle size={17} />
              <strong>Confirm {pending.action} operation</strong>
            </div>
            <p>{pending.method} {pending.route}</p>
            <p className="monitor-control-reason">Reason: {pending.reason}</p>
            {pending.action === "pause" ? (
              <div className="monitor-control-fields">
                <label>Duration
                  <select value={pauseHours} onChange={(event) => setPauseHours(Number(event.target.value))}>
                    <option value={1}>1 hour</option>
                    <option value={24}>24 hours</option>
                    <option value={168}>7 days</option>
                    <option value={720}>30 days</option>
                  </select>
                </label>
                <label>Maintenance status
                  <select value={responseStatus} onChange={(event) => setResponseStatus(Number(event.target.value))}>
                    <option value={503}>503 Service Unavailable</option>
                    <option value={429}>429 Too Many Requests</option>
                    <option value={404}>404 Not Found</option>
                  </select>
                </label>
                <label className="monitor-control-message">Custom maintenance response
                  <input value={responseMessage} maxLength={500} onChange={(event) => setResponseMessage(event.target.value)} />
                </label>
              </div>
            ) : null}
            <label className="monitor-code-field">Fresh System Monitor MFA code
              <input
                type="password"
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={6}
                value={mfaCode}
                onChange={(event) => setMfaCode(event.target.value.replace(/\D/g, "").slice(0, 6))}
              />
            </label>
            <div className="monitor-control-actions">
              <button type="button" className="monitor-button monitor-button-quiet" onClick={() => setPending(null)}>Cancel</button>
              <button type="button" className="monitor-button monitor-button-danger" disabled={busy || mfaCode.length !== 6} onClick={() => void confirmControl()}>
                {busy ? "Verifying…" : `Confirm ${pending.action}`}
              </button>
            </div>
          </div>
        ) : null}
      </div>
      <form className="monitor-terminal-input" onSubmit={(event) => void submitCommand(event)}>
        <span aria-hidden="true">›</span>
        <input
          aria-label="System Monitor command"
          autoComplete="off"
          spellCheck={false}
          value={input}
          onChange={(event) => setInput(event.target.value)}
          placeholder='Try "status" or "help"'
        />
        <button type="submit" aria-label="Run allowlisted command"><ChevronRight size={18} /></button>
      </form>
      <p className="monitor-terminal-footnote"><LockKeyhole size={12} /> Commands call fixed application APIs. Operating-system shell, database query, and arbitrary code execution are unavailable.</p>
    </section>
  );
}

export function SystemMonitor() {
  const [activeTab, setActiveTab] = useState<PageTab>("overview");
  const [overview, setOverview] = useState<MonitorOverview | null>(null);
  const [endpoints, setEndpoints] = useState<MonitorEndpoint[]>([]);
  const [events, setEvents] = useState<MonitorEvent[]>([]);
  const [history, setHistory] = useState<Record<string, unknown>[]>([]);
  const [analytics, setAnalytics] = useState<AnalyticsPoint[]>([]);
  const [selectedEndpoint, setSelectedEndpoint] = useState("");
  const [securityStatus, setSecurityStatus] = useState<{ mfa_enabled: boolean } | null>(null);
  const [securityConsole, setSecurityConsole] = useState<SecurityConsole | null>(null);
  const [pageError, setPageError] = useState("");
  const [securityError, setSecurityError] = useState("");
  const [securityNotice, setSecurityNotice] = useState("");
  const [refreshing, setRefreshing] = useState(false);
  const [search, setSearch] = useState("");
  const [healthFilter, setHealthFilter] = useState("all");
  const [analyticsError, setAnalyticsError] = useState("");
  const [analyticsBusy, setAnalyticsBusy] = useState(false);
  const [mfaCode, setMfaCode] = useState("");
  const [securityCode, setSecurityCode] = useState("");
  const [securityBusy, setSecurityBusy] = useState(false);
  const [controlBusy, setControlBusy] = useState(false);
  const [enrollmentBusy, setEnrollmentBusy] = useState(false);
  const [enrollment, setEnrollment] = useState<MFAEnrollment | null>(null);
  const [enrollmentPassword, setEnrollmentPassword] = useState("");
  const [enrollmentCode, setEnrollmentCode] = useState("");
  const [securityAction, setSecurityAction] = useState<{ kind: "revoke"; session: MonitorSession } | { kind: "disable-mfa" } | null>(null);
  const [pauseHours, setPauseHours] = useState(24);
  const [responseStatus, setResponseStatus] = useState(503);
  const [responseMessage, setResponseMessage] = useState("This endpoint is temporarily unavailable.");

  const refreshMonitor = useCallback(async (showSpinner = false) => {
    if (showSpinner) setRefreshing(true);
    try {
      const [nextOverview, nextEndpoints, nextEvents, nextHistory] = await Promise.all([
        getMonitorOverview(),
        getMonitorEndpoints(),
        getMonitorEvents(),
        getMonitorHistory(),
      ]);
      setOverview(nextOverview);
      setEndpoints(nextEndpoints);
      setEvents(nextEvents);
      setHistory(nextHistory);
      setPageError("");
      setSelectedEndpoint((current) => current || `${nextEndpoints[0]?.method ?? ""} ${nextEndpoints[0]?.route ?? ""}`);
    } catch (error) {
      setPageError(error instanceof Error ? error.message : "System Monitor data is unavailable.");
    } finally {
      if (showSpinner) setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    let active = true;
    void refreshMonitor();
    void getMonitorSecurityStatus().then((status) => {
      if (active) setSecurityStatus(status);
    }).catch((error: unknown) => {
      if (active) setSecurityError(error instanceof Error ? error.message : "Security status is unavailable.");
    });
    const timer = window.setInterval(() => {
      if (active) void refreshMonitor();
    }, 10_000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [refreshMonitor]);

  const selected = useMemo(() => {
    const [method, ...route] = selectedEndpoint.split(" ");
    return endpoints.find((endpoint) => endpoint.method === method && endpoint.route === route.join(" ")) ?? null;
  }, [endpoints, selectedEndpoint]);

  const filteredEndpoints = useMemo(() => endpoints.filter((endpoint) => {
    const matchesText = `${endpoint.method} ${endpoint.route} ${endpoint.description} ${endpoint.controller}`
      .toLowerCase().includes(search.toLowerCase());
    const matchesHealth = healthFilter === "all" ||
      (healthFilter === "paused" ? endpoint.paused : endpoint.health === healthFilter);
    return matchesText && matchesHealth;
  }), [endpoints, healthFilter, search]);

  async function loadAnalytics(endpoint: MonitorEndpoint) {
    setAnalyticsBusy(true);
    setAnalyticsError("");
    setSelectedEndpoint(`${endpoint.method} ${endpoint.route}`);
    try {
      setAnalytics(await getEndpointAnalytics(endpoint.method, endpoint.route));
    } catch (error) {
      setAnalytics([]);
      setAnalyticsError(error instanceof Error ? error.message : "Endpoint analytics could not be loaded.");
    } finally {
      setAnalyticsBusy(false);
    }
  }

  async function loadSecurityConsole() {
    setSecurityBusy(true);
    setSecurityError("");
    setSecurityNotice("");
    try {
      setSecurityConsole(await getSecurityConsole(securityCode));
      setSecurityCode("");
    } catch (error) {
      setSecurityError(error instanceof Error ? error.message : "Security console data could not be loaded.");
    } finally {
      setSecurityBusy(false);
    }
  }

  async function enrollMFA() {
    setEnrollmentBusy(true);
    setSecurityError("");
    try {
      setEnrollment(await beginMFAEnrollment(enrollmentPassword));
      setEnrollmentPassword("");
    } catch (error) {
      setSecurityError(error instanceof Error ? error.message : "MFA enrollment could not be started.");
    } finally {
      setEnrollmentBusy(false);
    }
  }

  async function confirmEnrollment() {
    setEnrollmentBusy(true);
    setSecurityError("");
    try {
      await confirmMFAEnrollment(enrollmentCode);
      setEnrollment(null);
      setEnrollmentCode("");
      setSecurityStatus({ mfa_enabled: true });
      setSecurityError("");
    } catch (error) {
      setSecurityError(error instanceof Error ? error.message : "MFA enrollment could not be confirmed.");
    } finally {
      setEnrollmentBusy(false);
    }
  }

  async function executeControl(pending: PendingControl, code: string, durationHours: number, status: number, message: string) {
    setControlBusy(true);
    try {
      const payload = pending.action === "pause"
        ? {
            method: pending.method,
            route: pending.route,
            reason: pending.reason,
            response_status: status,
            response_message: message,
            expires_at: new Date(Date.now() + durationHours * 60 * 60 * 1000).toISOString(),
          }
        : { method: pending.method, route: pending.route, reason: pending.reason };
      await setEndpointControl(pending.action, payload as Record<string, string | number>, code);
      await refreshMonitor();
    } finally {
      setControlBusy(false);
    }
  }

  async function revokeSession(session: MonitorSession) {
    setSecurityBusy(true);
    setSecurityError("");
    setSecurityNotice("");
    try {
      await revokeMonitorSession(session.id, "Revoked from System Monitor Security Centre", securityCode);
      setSecurityCode("");
      setSecurityAction(null);
      setSecurityConsole(null);
      setSecurityNotice("Session revoked. Enter a fresh MFA code to load an updated security snapshot.");
    } catch (error) {
      setSecurityError(error instanceof Error ? error.message : "The session could not be revoked.");
    } finally {
      setSecurityBusy(false);
    }
  }

  async function disableMFA() {
    setSecurityBusy(true);
    setSecurityError("");
    setSecurityNotice("");
    try {
      await disableMonitorMFA("Disabled from System Monitor Security Centre", securityCode);
      setSecurityCode("");
      setSecurityAction(null);
      setSecurityStatus({ mfa_enabled: false });
      setSecurityConsole(null);
    } catch (error) {
      setSecurityError(error instanceof Error ? error.message : "System Monitor MFA could not be disabled.");
    } finally {
      setSecurityBusy(false);
    }
  }

  const totalRequests = endpoints.reduce((total, endpoint) => total + endpoint.requests, 0);
  const unhealthyEndpoints = endpoints.filter((endpoint) => endpoint.health !== "healthy" || endpoint.paused).length;
  const database = overview?.database;

  return (
    <div className="system-monitor-page">
      <header className="monitor-hero">
        <div className="monitor-hero-grid" aria-hidden="true" />
        <div className="monitor-hero-content">
          <div className="monitor-eyebrow"><span className="monitor-live-dot" /> SYSTEM CONTROL PLANE <span className="monitor-separator">/</span> MONITOR 01</div>
          <div className="monitor-title-row">
            <div>
              <h1>System Monitor</h1>
              <p>Application health, endpoint telemetry and privileged controls in one place.</p>
            </div>
            <button type="button" className="monitor-refresh" onClick={() => void refreshMonitor(true)} disabled={refreshing}>
              <RefreshCw size={15} className={refreshing ? "monitor-spin" : ""} /> {refreshing ? "Refreshing" : "Refresh"}
            </button>
          </div>
          <nav className="monitor-tabs" aria-label="System Monitor sections">
            {([
              ["overview", "Overview", Activity],
              ["endpoints", "Endpoints", Server],
              ["security", "Security Centre", ShieldCheck],
              ["terminal", "Command Terminal", Command],
            ] as const).map(([key, label, Icon]) => (
              <button
                key={key}
                type="button"
                aria-current={activeTab === key ? "page" : undefined}
                className={`monitor-tab${activeTab === key ? " monitor-tab-active" : ""}`}
                onClick={() => setActiveTab(key)}
              >
                <Icon size={15} /> {label}
              </button>
            ))}
          </nav>
        </div>
      </header>

      <main className="monitor-main">
        {pageError ? (
          <div className="monitor-access-error" role="alert">
            <ShieldAlert size={20} />
            <div><strong>Monitor access unavailable</strong><p>{pageError}</p></div>
            <button type="button" onClick={() => void refreshMonitor(true)}>Retry</button>
          </div>
        ) : null}

        {activeTab === "overview" ? (
          <div className="monitor-content-grid">
            <div className="monitor-stats-grid">
              <StatCard label="API UPTIME" value={overview ? formatDuration(overview.uptime_seconds) : "—"} detail={overview ? `Since ${formatTime(overview.started_at)}` : "Waiting for telemetry"} icon={Wifi} tone="cyan" />
              <StatCard label="REQUESTS · PROCESS" value={numberText(totalRequests)} detail="Since API process start" icon={Activity} tone="blue" />
              <StatCard label="DATABASE" value={overview?.database_status ?? "—"} detail={`${numberText(database?.in_use)} in use · ${numberText(database?.idle)} idle`} icon={Database} tone={overview?.database_status === "healthy" ? "green" : "amber"} />
              <StatCard label="ENDPOINT HEALTH" value={`${endpoints.length - unhealthyEndpoints}/${endpoints.length || "—"}`} detail={`${unhealthyEndpoints} paused or degraded`} icon={Server} tone={unhealthyEndpoints ? "amber" : "green"} />
            </div>

            <div className="monitor-overview-grid">
              <section className="monitor-panel monitor-health-panel">
                <div className="monitor-panel-heading">
                  <div><span className="monitor-kicker">RUNTIME</span><h2>Service health</h2></div>
                  <span className="monitor-panel-live"><span className="monitor-live-dot" /> LIVE · 10S</span>
                </div>
                <div className="monitor-health-grid">
                  <div className="monitor-health-cell"><span>Database pool</span><strong>{numberText(database?.open_connections)} <small>connections</small></strong><div className="monitor-progress"><i style={{ width: `${database && database.open_connections ? Math.min(100, database.in_use / database.open_connections * 100) : 0}%` }} /></div><small>{numberText(database?.in_use)} active · {numberText(database?.idle)} idle</small></div>
                  <div className="monitor-health-cell"><span>Analytics queue</span><strong>{numberText(overview?.analytics_pending_buckets)} <small>buckets</small></strong><div className="monitor-progress monitor-progress-blue"><i style={{ width: `${overview ? Math.min(100, overview.analytics_pending_buckets / 100) : 0}%` }} /></div><small>{numberText(overview?.analytics_dropped_requests)} dropped requests</small></div>
                  <div className="monitor-health-cell"><span>API processes</span><strong>{overview?.database_status === "healthy" ? "Operational" : "Check status"}</strong><small>Application process health is reported by the API.</small></div>
                </div>
                <div className="monitor-footnote"><Clock3 size={13} /> Server CPU and memory metrics are not exposed by the current backend.</div>
              </section>

              <section className="monitor-panel">
                <div className="monitor-panel-heading"><div><span className="monitor-kicker">ACTIVITY STREAM</span><h2>Recent system events</h2></div><button className="monitor-text-button" onClick={() => setActiveTab("endpoints")}>All endpoints <ChevronRight size={14} /></button></div>
                <EventList events={events.slice(-7).reverse()} />
                <div className="monitor-footnote"><Activity size={13} /> Process-local events · recent events reset when the API restarts.</div>
              </section>
            </div>

            <section className="monitor-panel monitor-traffic-panel">
              <div className="monitor-panel-heading"><div><span className="monitor-kicker">ROUTE TELEMETRY</span><h2>Endpoint activity</h2></div><button className="monitor-text-button" onClick={() => setActiveTab("endpoints")}>Open inventory <ChevronRight size={14} /></button></div>
              <EndpointTable endpoints={endpoints.slice(0, 6)} onSelect={(endpoint) => { setActiveTab("endpoints"); void loadAnalytics(endpoint); }} compact />
            </section>
          </div>
        ) : null}

        {activeTab === "endpoints" ? (
          <div className="monitor-content-grid">
            <section className="monitor-panel">
              <div className="monitor-panel-heading">
                <div><span className="monitor-kicker">DISCOVERED ROUTES</span><h2>Endpoint inventory</h2></div>
                <span className="monitor-count">{endpoints.length} routes</span>
              </div>
              <div className="monitor-filters">
                <label className="monitor-search"><span className="sr-only">Search endpoints</span><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search method, route or controller" /></label>
                <label><span className="sr-only">Filter endpoint health</span><select value={healthFilter} onChange={(event) => setHealthFilter(event.target.value)}><option value="all">All health states</option><option value="healthy">Healthy</option><option value="degraded">Degraded</option><option value="paused">Paused</option></select></label>
              </div>
              <EndpointTable endpoints={filteredEndpoints} onSelect={(endpoint) => void loadAnalytics(endpoint)} />
            </section>

            <div className="monitor-detail-grid">
              <section className="monitor-panel monitor-analytics-panel">
                <div className="monitor-panel-heading">
                  <div><span className="monitor-kicker">HISTORICAL · 24 HOURS</span><h2>{selected ? `${selected.method} ${selected.route}` : "Select an endpoint"}</h2></div>
                  {selected ? <button className="monitor-icon-button" title="Refresh analytics" onClick={() => void loadAnalytics(selected)} disabled={analyticsBusy}><RefreshCw size={15} /></button> : null}
                </div>
                {analyticsBusy ? <p className="monitor-empty">Loading endpoint analytics…</p> : analyticsError ? <p className="monitor-inline-error">{analyticsError}</p> : (
                  <AnalyticsChart points={analytics} />
                )}
              </section>
              <section className="monitor-panel">
                <div className="monitor-panel-heading"><div><span className="monitor-kicker">CONTROL HISTORY</span><h2>Recent state changes</h2></div></div>
                {history.length ? <div className="monitor-history-list">{history.slice(0, 6).map((item, index) => (
                  <div className="monitor-history-item" key={`${String(item.changed_at)}-${index}`}>
                    <span className={`monitor-history-icon ${item.action === "paused" ? "is-paused" : ""}`}>{item.action === "paused" ? <LockKeyhole size={14} /> : <CheckCircle2 size={14} />}</span>
                    <div><strong>{String(item.action ?? "changed")} · {String(item.method ?? "")}</strong><small>{String(item.route ?? "")}</small><small>{formatTime(typeof item.changed_at === "string" ? item.changed_at : undefined)}</small></div>
                  </div>
                ))}</div> : <p className="monitor-empty">No endpoint state changes recorded.</p>}
              </section>
            </div>
            <section className="monitor-panel">
              <div className="monitor-panel-heading"><div><span className="monitor-kicker">LIVE TRAFFIC</span><h2>Recent API requests</h2></div><span className="monitor-panel-live"><span className="monitor-live-dot" /> LIVE</span></div>
              <EventList events={[...events].reverse().slice(0, 12)} />
            </section>
          </div>
        ) : null}

        {activeTab === "security" ? (
          <SecurityCentre
            status={securityStatus}
            consoleData={securityConsole}
            error={securityError}
            notice={securityNotice}
            code={securityCode}
            setCode={setSecurityCode}
            busy={securityBusy}
            enrollment={enrollment}
            enrollmentPassword={enrollmentPassword}
            setEnrollmentPassword={setEnrollmentPassword}
            enrollmentCode={enrollmentCode}
            setEnrollmentCode={setEnrollmentCode}
            enrollmentBusy={enrollmentBusy}
            onStartEnrollment={() => void enrollMFA()}
            onConfirmEnrollment={() => void confirmEnrollment()}
            onLoadConsole={() => void loadSecurityConsole()}
            pendingAction={securityAction}
            setPendingAction={setSecurityAction}
            onRevoke={(session) => void revokeSession(session)}
            onDisableMFA={() => void disableMFA()}
          />
        ) : null}

        {activeTab === "terminal" ? (
          <div className="monitor-terminal-layout">
            <div className="monitor-terminal-intro"><span className="monitor-kicker">OPERATOR WORKSPACE</span><h2>Controlled operations</h2><p>Use the explicit command allowlist to inspect health, query endpoint analytics, or begin a confirmed pause/resume operation.</p></div>
            <CommandPanel
              overview={overview}
              endpoints={endpoints}
              events={events}
              onControl={executeControl}
              mfaCode={mfaCode}
              setMfaCode={setMfaCode}
              pauseHours={pauseHours}
              setPauseHours={setPauseHours}
              responseStatus={responseStatus}
              setResponseStatus={setResponseStatus}
              responseMessage={responseMessage}
              setResponseMessage={setResponseMessage}
              busy={controlBusy}
              onClearPending={() => setMfaCode("")}
            />
            <div className="monitor-command-caveat"><ShieldCheck size={17} /><span>Every privileged command is re-authorized server-side. Endpoint controls are limited to catalogued, pausable routes. Authentication, health and Monitor recovery routes cannot be paused.</span></div>
          </div>
        ) : null}
      </main>
    </div>
  );
}

function EndpointTable({
  endpoints,
  onSelect,
  compact = false,
}: {
  endpoints: MonitorEndpoint[];
  onSelect: (endpoint: MonitorEndpoint) => void;
  compact?: boolean;
}) {
  if (!endpoints.length) return <p className="monitor-empty">No matching endpoint telemetry.</p>;
  return (
    <div className="monitor-table-scroll">
      <table className="monitor-table">
        <thead><tr><th>Endpoint</th><th>Health</th><th>Requests</th><th>Error rate</th><th>Latency</th><th /></tr></thead>
        <tbody>
          {endpoints.map((endpoint) => {
            const latencyTotal = Object.values(endpoint.latency_buckets).reduce((total, count) => total + count, 0);
            return (
              <tr key={`${endpoint.method} ${endpoint.route}`} className={endpoint.paused ? "monitor-row-paused" : ""}>
                <td><div className="monitor-endpoint-route"><span className={`monitor-method method-${endpoint.method.toLowerCase()}`}>{endpoint.method}</span><code>{endpoint.route}</code></div><small className="monitor-controller">{endpoint.controller} · {endpoint.description}</small></td>
                <td><span className={`monitor-health-tag ${statusTone(endpoint.paused ? "paused" : endpoint.health)}`}><i />{endpoint.paused ? "Paused" : endpoint.health}</span></td>
                <td>{numberText(endpoint.requests)}</td>
                <td>{`${(endpoint.error_rate * 100).toFixed(1)}%`}</td>
                <td>{numberText(latencyTotal)} samples</td>
                <td><button className="monitor-row-action" type="button" onClick={() => onSelect(endpoint)}>{compact ? "Inspect" : "Analytics"} <ChevronRight size={13} /></button></td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function EventList({ events }: { events: MonitorEvent[] }) {
  if (!events.length) return <p className="monitor-empty">No recent request events.</p>;
  return (
    <div className="monitor-event-list">
      {events.map((event, index) => {
        const failed = event.status >= 400;
        const Icon = failed ? XCircle : CheckCircle2;
        return (
          <div className="monitor-event-item" key={`${event.occurred_at}-${event.method}-${index}`}>
            <span className={`monitor-event-icon ${failed ? "is-error" : ""}`}><Icon size={14} /></span>
            <div className="monitor-event-main"><strong>{event.type.replaceAll("_", " ")}</strong><span><b>{event.method}</b> {event.route}</span></div>
            <span className={`monitor-event-status ${failed ? "is-error" : ""}`}>{event.status}</span>
            <small>{event.duration_ms}ms · {formatTime(event.occurred_at)}</small>
          </div>
        );
      })}
    </div>
  );
}

function AnalyticsChart({ points }: { points: AnalyticsPoint[] }) {
  const maxRequests = Math.max(1, ...points.map((point) => point.requests));
  if (!points.length) return <p className="monitor-empty">Select an endpoint to load historical request analytics.</p>;
  const totals = points.reduce((sum, point) => ({
    requests: sum.requests + point.requests,
    errors: sum.errors + point.client_errors + point.server_errors,
    latency: sum.latency + point.average_latency_ms,
  }), { requests: 0, errors: 0, latency: 0 });
  return (
    <div>
      <div className="monitor-analytics-metrics">
        <div><span>Requests</span><strong>{numberText(totals.requests)}</strong></div>
        <div><span>Error requests</span><strong>{numberText(totals.errors)}</strong></div>
        <div><span>Avg. bucket latency</span><strong>{(totals.latency / points.length).toFixed(1)}<small> ms</small></strong></div>
        <div><span>Peak p95</span><strong>{Math.max(...points.map((point) => point.p95_latency_ms))}<small> ms</small></strong></div>
      </div>
      <div className="monitor-chart" aria-label="Hourly request counts">
        {points.map((point) => (
          <div className="monitor-chart-column" key={point.bucket} title={`${formatTime(point.bucket)} · ${point.requests} requests · ${(point.error_rate * 100).toFixed(1)}% errors`}>
            <div className="monitor-chart-bar" style={{ height: `${Math.max(3, point.requests / maxRequests * 100)}%` }}>
              {point.client_errors + point.server_errors > 0 ? <i style={{ height: `${Math.min(100, (point.client_errors + point.server_errors) / Math.max(1, point.requests) * 100)}%` }} /> : null}
            </div>
          </div>
        ))}
      </div>
      <div className="monitor-chart-axis"><span>{formatTime(points[0]?.bucket)}</span><span>{formatTime(points.at(-1)?.bucket)}</span></div>
      <div className="monitor-chart-legend"><span><i /> Requests</span><span><i /> Errors</span><small>P95 is an estimate based on fixed latency buckets.</small></div>
    </div>
  );
}

function SecurityCentre({
  status,
  consoleData,
  error,
  notice,
  code,
  setCode,
  busy,
  enrollment,
  enrollmentPassword,
  setEnrollmentPassword,
  enrollmentCode,
  setEnrollmentCode,
  enrollmentBusy,
  onStartEnrollment,
  onConfirmEnrollment,
  onLoadConsole,
  pendingAction,
  setPendingAction,
  onRevoke,
  onDisableMFA,
}: {
  status: { mfa_enabled: boolean } | null;
  consoleData: SecurityConsole | null;
  error: string;
  notice: string;
  code: string;
  setCode: (value: string) => void;
  busy: boolean;
  enrollment: MFAEnrollment | null;
  enrollmentPassword: string;
  setEnrollmentPassword: (value: string) => void;
  enrollmentCode: string;
  setEnrollmentCode: (value: string) => void;
  enrollmentBusy: boolean;
  onStartEnrollment: () => void;
  onConfirmEnrollment: () => void;
  onLoadConsole: () => void;
  pendingAction: { kind: "revoke"; session: MonitorSession } | { kind: "disable-mfa" } | null;
  setPendingAction: (action: { kind: "revoke"; session: MonitorSession } | { kind: "disable-mfa" } | null) => void;
  onRevoke: (session: MonitorSession) => void;
  onDisableMFA: () => void;
}) {
  return (
    <div className="monitor-content-grid">
      <div className="monitor-security-banner">
        <div className="monitor-security-icon"><ShieldCheck size={22} /></div>
        <div><span className="monitor-kicker">INDEPENDENT PRIVILEGED ACCESS</span><h2>Security Centre</h2><p>Access depends on a separate, expiring Monitor grant. Admin role permissions do not grant access.</p></div>
        <span className={`monitor-security-state ${status?.mfa_enabled ? "is-enabled" : ""}`}><i /> MFA {status?.mfa_enabled ? "ENABLED" : status ? "NOT ENROLLED" : "CHECKING"}</span>
      </div>
      {error ? <div className="monitor-inline-error" role="alert">{error}</div> : null}
      {notice ? <div className="monitor-inline-notice" role="status">{notice}</div> : null}
      <div className="monitor-security-grid">
        <section className="monitor-panel">
          <div className="monitor-panel-heading"><div><span className="monitor-kicker">PRIVILEGED PROOF</span><h2>Multi-factor authentication</h2></div><LockKeyhole size={18} /></div>
          {!status?.mfa_enabled ? (
            <div className="monitor-enrollment">
              <p>Enroll an authenticator before using privileged controls. Password reauthentication is required to begin setup.</p>
              {!enrollment ? (
                <form onSubmit={(event) => { event.preventDefault(); onStartEnrollment(); }}>
                  <label>Current account password<input type="password" autoComplete="current-password" value={enrollmentPassword} onChange={(event) => setEnrollmentPassword(event.target.value)} required /></label>
                  <button type="submit" className="monitor-button monitor-button-primary" disabled={enrollmentBusy || !enrollmentPassword}>{enrollmentBusy ? "Starting…" : "Begin MFA enrollment"}</button>
                </form>
              ) : (
                <div className="monitor-mfa-setup">
                  <strong>Scan this secret in your authenticator app</strong>
                  <code>{enrollment.secret}</code>
                  <a href={enrollment.otpauth_uri}>Open authenticator setup link</a>
                  <small>Expires {formatTime(enrollment.expires_at)}. This secret is shown once; store it only in your authenticator.</small>
                  <label>Verify six-digit code<input type="password" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={enrollmentCode} onChange={(event) => setEnrollmentCode(event.target.value.replace(/\D/g, "").slice(0, 6))} /></label>
                  <button type="button" className="monitor-button monitor-button-primary" disabled={enrollmentBusy || enrollmentCode.length !== 6} onClick={onConfirmEnrollment}>Confirm and enable MFA</button>
                </div>
              )}
            </div>
          ) : (
            <div className="monitor-mfa-enabled"><CheckCircle2 size={19} /><p>MFA is enabled and is required for every privileged control and Security Centre snapshot.</p><button type="button" className="monitor-button monitor-button-quiet" onClick={() => setPendingAction({ kind: "disable-mfa" })}>Disable MFA</button></div>
          )}
        </section>
        <section className="monitor-panel">
          <div className="monitor-panel-heading"><div><span className="monitor-kicker">ONE-TIME VERIFICATION</span><h2>Load security snapshot</h2></div><ShieldAlert size={18} /></div>
          <p className="monitor-panel-copy">A snapshot fetches sessions, alerts, security events, grants, and audit history in one request. MFA codes are single-use; this avoids code replay across separate reads.</p>
          <label className="monitor-code-field">Current MFA code<input type="password" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, "").slice(0, 6))} /></label>
          <button type="button" className="monitor-button monitor-button-primary" disabled={busy || code.length !== 6 || !status?.mfa_enabled} onClick={onLoadConsole}>{busy ? "Verifying…" : "Verify and load snapshot"}</button>
          {consoleData ? <span className="monitor-snapshot-time"><CheckCircle2 size={13} /> Snapshot loaded · device metadata unavailable</span> : null}
        </section>
      </div>

      {consoleData ? (
        <>
          {pendingAction ? (
            <section className="monitor-action-confirm" role="alert">
              <AlertTriangle size={18} />
              <div><strong>Confirm privileged action</strong><p>{pendingAction.kind === "revoke" ? `Revoke session ${pendingAction.session.id}? The current session cannot be revoked.` : "Disable System Monitor MFA? This removes the second-factor requirement and is audited."}</p></div>
              <label>Fresh MFA code<input type="password" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, "").slice(0, 6))} /></label>
              <button type="button" className="monitor-button monitor-button-danger" disabled={busy || code.length !== 6} onClick={() => pendingAction.kind === "revoke" ? onRevoke(pendingAction.session) : onDisableMFA()}>Confirm</button>
              <button type="button" className="monitor-button monitor-button-quiet" onClick={() => setPendingAction(null)}>Cancel</button>
            </section>
          ) : null}
          <section className="monitor-panel">
            <div className="monitor-panel-heading"><div><span className="monitor-kicker">ACTIVE ACCESS</span><h2>Monitor grants <span className="monitor-count">{consoleData.grants.length}</span></h2></div><LockKeyhole size={17} /></div>
            <div className="monitor-grant-list">
              {consoleData.grants.map((grant: MonitorGrant) => <div key={`${grant.account_id}-${grant.granted_at}`}><strong>{grant.account_id}</strong><span>{grant.reason}</span><small>Expires {formatTime(grant.expires_at)} · granted by {grant.granted_by}</small></div>)}
              {!consoleData.grants.length ? <p className="monitor-empty">No active independent Monitor grants.</p> : null}
            </div>
          </section>
          <div className="monitor-security-grid">
            <section className="monitor-panel">
              <div className="monitor-panel-heading"><div><span className="monitor-kicker">AUTHENTICATION</span><h2>Alerts</h2></div><span className="monitor-count">{consoleData.alerts.length}</span></div>
              <AlertList alerts={consoleData.alerts} />
            </section>
            <section className="monitor-panel">
              <div className="monitor-panel-heading"><div><span className="monitor-kicker">SESSION VISIBILITY</span><h2>Active sessions</h2></div><span className="monitor-count">{consoleData.sessions.length}</span></div>
              <div className="monitor-session-list">
                {consoleData.sessions.map((session) => (
                  <div className="monitor-session" key={session.id}>
                    <div><strong>{session.current ? "Current browser session" : "Staff session"}</strong><small>{session.id} · created {formatTime(session.created_at)}</small><small>Expires {formatTime(session.expires_at)}</small></div>
                    {session.current ? <span className="monitor-current-tag">CURRENT</span> : <button type="button" className="monitor-row-action monitor-revoke" onClick={() => setPendingAction({ kind: "revoke", session })}>Revoke</button>}
                  </div>
                ))}
                {!consoleData.sessions.length ? <p className="monitor-empty">No active sessions were returned.</p> : null}
              </div>
              <p className="monitor-footnote">Device and IP metadata are not recorded by the current identity session schema.</p>
            </section>
          </div>
          <div className="monitor-security-grid">
            <section className="monitor-panel">
              <div className="monitor-panel-heading"><div><span className="monitor-kicker">DETECTION</span><h2>Security events</h2></div><span className="monitor-count">{consoleData.events.length}</span></div>
              <SecurityEventList events={consoleData.events} />
            </section>
            <section className="monitor-panel">
              <div className="monitor-panel-heading"><div><span className="monitor-kicker">ACCOUNTABILITY</span><h2>Privileged audit trail</h2></div><span className="monitor-count">{consoleData.audit.length}</span></div>
              <div className="monitor-audit-list">
                {consoleData.audit.slice(0, 12).map((event, index) => <div key={`${event.occurred_at}-${index}`}><strong>{event.action}</strong><small>{event.resource_type} · {event.resource_id}</small><small>{formatTime(event.occurred_at)}</small></div>)}
                {!consoleData.audit.length ? <p className="monitor-empty">No privileged actions are recorded.</p> : null}
              </div>
            </section>
          </div>
        </>
      ) : <div className="monitor-security-placeholder"><ShieldAlert size={18} /><span>Security data remains hidden until MFA is verified for a snapshot.</span></div>}
    </div>
  );
}

function AlertList({ alerts }: { alerts: MonitorAlert[] }) {
  if (!alerts.length) return <p className="monitor-empty">No active alerts in the current window.</p>;
  return <div className="monitor-alert-list">{alerts.slice(0, 12).map((alert, index) => <div key={`${alert.occurred_at}-${index}`}><SeverityTag severity={alert.severity} /><div><strong>{alert.event_type.replaceAll("_", " ")}</strong><small>{alert.source} · {alert.failure_count ? `${alert.failure_count} failures · ` : ""}{formatTime(alert.occurred_at)}</small></div></div>)}</div>;
}

function SecurityEventList({ events }: { events: MonitorSecurityEvent[] }) {
  if (!events.length) return <p className="monitor-empty">No security events have been recorded.</p>;
  return <div className="monitor-alert-list">{events.slice(0, 12).map((event) => <div key={event.id}><SeverityTag severity={event.severity} /><div><strong>{event.event_type.replaceAll("_", " ")}</strong><small>{event.account_id ? `Account ${event.account_id} · ` : ""}{formatTime(event.occurred_at)}</small></div></div>)}</div>;
}
