"use client";

import Link from "next/link";
import { ArrowLeft, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";

import { ErrorBanner, PageHeader, Panel } from "@/components/workspace/ui";
import { formatAuditError, getAuditEvent } from "@/lib/audit/api";
import type { AuditEvent } from "@/lib/audit/api";

function displayLabel(value: string) {
  return value.replace(/[._-]+/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function stringDetail(details: unknown, key: string): string | undefined {
  if (typeof details !== "object" || details === null || Array.isArray(details)) return undefined;
  const value = (details as Record<string, unknown>)[key];
  return typeof value === "string" ? value : undefined;
}

function AuditValue({ value }: { value: unknown }) {
  if (value === null || value === undefined) return <span className="text-[var(--muted)]">Not provided</span>;
  if (typeof value !== "object") return <span className="break-words">{String(value)}</span>;
  if (Array.isArray(value)) {
    if (!value.length) return <span className="text-[var(--muted)]">None</span>;
    return <ul className="list-inside list-disc space-y-1">{value.map((item, index) => <li key={index}><AuditValue value={item} /></li>)}</ul>;
  }
  const entries = Object.entries(value);
  if (!entries.length) return <span className="text-[var(--muted)]">None</span>;
  return (
    <dl className="grid min-w-0 gap-3 sm:grid-cols-2">
      {entries.map(([key, item]) => (
        <div className="min-w-0" key={key}>
          <dt className="text-xs font-medium text-[var(--muted)]">{displayLabel(key)}</dt>
          <dd className="mt-1 break-words text-sm text-[var(--text)]"><AuditValue value={item} /></dd>
        </div>
      ))}
    </dl>
  );
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return `Invalid timestamp · ${value || "empty value"}`;
  return new Intl.DateTimeFormat(undefined, { dateStyle: "full", timeStyle: "long" }).format(date);
}

export function AuditEventDetail({
  eventId,
  canRead,
  developmentOpen,
}: {
  eventId: string;
  canRead: boolean;
  developmentOpen: boolean;
}) {
  const [event, setEvent] = useState<AuditEvent | null>(null);
  const [loading, setLoading] = useState(canRead);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    if (!canRead) return;
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    void getAuditEvent(eventId, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setEvent(result);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) {
          setEvent(null);
          setError(formatAuditError(cause, "Unable to load this audit event.", developmentOpen));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [canRead, developmentOpen, eventId, reloadKey]);

  if (!canRead) {
    return (
      <>
        <PageHeader title="Audit event" description="Review a full audit record." />
        <div role="alert" className="error-banner">Audit events are available to accounts with the audit.read permission.</div>
      </>
    );
  }

  return (
    <div className="grid min-w-0 gap-4 pb-6">
      <PageHeader
        title="Audit event details"
        description="."
        actions={<Link href="/audit" className="button-secondary"><ArrowLeft size={16} aria-hidden="true" /> Back to audit log</Link>}
      />
      {error ? <ErrorBanner message={error} onRetry={() => setReloadKey((key) => key + 1)} /> : null}
      {loading ? <p role="status" className="p-5 text-sm text-[var(--muted)]">Loading audit event…</p> : null}
      {!loading && event ? (
        <Panel title="Event details" className="audit-event-detail-panel">
          <dl className="audit-event-detail-grid">
            <div><dt>Actor</dt><dd>{event.actor_name || (event.actor_type === "system" ? "System" : event.actor_id || "Actor ID unavailable")}</dd></div>
            <div><dt>Role</dt><dd>{event.actor_role || stringDetail(event.details, "actor_role") || displayLabel(event.actor_type) || "Actor type unavailable"}</dd></div>
            <div><dt>Actor ID</dt><dd>{event.actor_id || "Actor ID unavailable"}</dd></div>
            <div><dt>Action</dt><dd>{displayLabel(event.action)}</dd></div>
            <div><dt>Resource type</dt><dd>{displayLabel(event.resource_type)}</dd></div>
            <div><dt>Resource name</dt><dd>{event.resource_name || event.resource_id || "Resource ID unavailable"}</dd></div>
            <div><dt>Resource ID</dt><dd>{event.resource_id || "Resource ID unavailable"}</dd></div>
            <div><dt>Timestamp</dt><dd>{formatDate(event.occurred_at)}</dd></div>
            {event.ip_address || stringDetail(event.details, "ip_address") ? (
              <div><dt>IP address</dt><dd>{event.ip_address || stringDetail(event.details, "ip_address")}</dd></div>
            ) : null}
            {event.user_agent || stringDetail(event.details, "user_agent") ? (
              <div><dt>User agent</dt><dd>{event.user_agent || stringDetail(event.details, "user_agent")}</dd></div>
            ) : null}
          </dl>
          {typeof event.details === "object" && event.details !== null && !Array.isArray(event.details) &&
          ("before" in event.details || "after" in event.details) ? (
            <div className="audit-event-change-grid">
              <section><h3>Before</h3><AuditValue value={(event.details as Record<string, unknown>).before} /></section>
              <section><h3>After</h3><AuditValue value={(event.details as Record<string, unknown>).after} /></section>
            </div>
          ) : null}
          <section className="audit-event-raw-details">
            <h3>Additional details</h3>
            <div className="rounded-lg border border-[var(--border)] bg-[var(--page-bg)] p-4 text-sm leading-5 text-[var(--text)]">
              <AuditValue value={event.details} />
            </div>
          </section>
        </Panel>
      ) : null}
    </div>
  );
}
