"use client";

import Link from "next/link";
import {
  ChevronLeft,
  ChevronRight,
  Clock3,
  FilterX,
  RefreshCw,
  Search,
} from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import type { FormEvent } from "react";

import {
  DataTable,
  EmptyState,
  ErrorBanner,
  LoadingTable,
  PageHeader,
  Panel,
} from "@/components/workspace/ui";
import {
  formatAuditError,
  listAuditEvents,
} from "@/lib/audit/api";
import type { AuditEvent, AuditEventFilter } from "@/lib/audit/api";

const pageSize = 25;

type FilterDraft = {
  actorId: string;
  action: string;
  resourceType: string;
  resourceId: string;
  from: string;
  to: string;
};

const emptyFilter: FilterDraft = {
  actorId: "",
  action: "",
  resourceType: "",
  resourceId: "",
  from: "",
  to: "",
};

function toApiDateTime(value: string) {
  if (!value) return undefined;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString();
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Unknown time";
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function displayLabel(value: string) {
  return value
    .replace(/[._-]+/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function AuditLog({
  canRead,
  developmentOpen = false,
}: {
  canRead: boolean;
  developmentOpen?: boolean;
}) {
  const [draft, setDraft] = useState<FilterDraft>(emptyFilter);
  const [filters, setFilters] = useState<FilterDraft>(emptyFilter);
  const [offset, setOffset] = useState(0);
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(canRead);
  const [listError, setListError] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [filterError, setFilterError] = useState<string | null>(null);
  const [isMobile, setIsMobile] = useState(false);

  useEffect(() => {
    const update = () => setIsMobile(window.innerWidth < 768);
    update();
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, []);

  const loadEvents = useCallback(async (signal: AbortSignal) => {
    try {
      const request: AuditEventFilter = {
        limit: pageSize,
        offset,
        ...(filters.actorId ? { actorId: filters.actorId } : {}),
        ...(filters.action ? { action: filters.action } : {}),
        ...(filters.resourceType ? { resourceType: filters.resourceType } : {}),
        ...(filters.resourceId ? { resourceId: filters.resourceId } : {}),
        ...(toApiDateTime(filters.from) ? { from: toApiDateTime(filters.from) } : {}),
        ...(toApiDateTime(filters.to) ? { to: toApiDateTime(filters.to) } : {}),
      };
      const result = await listAuditEvents(request, signal);
      if (!signal.aborted) {
        setEvents(result.events);
        setTotal(result.total);
      }
    } catch (error) {
      if (!signal.aborted) {
        setEvents([]);
        setListError(formatAuditError(error, "Unable to load audit events.", developmentOpen));
      }
    } finally {
      if (!signal.aborted) setLoading(false);
    }
  }, [developmentOpen, filters, offset]);

  useEffect(() => {
    if (!canRead) return;
    const controller = new AbortController();
    void Promise.resolve().then(() => loadEvents(controller.signal));
    return () => controller.abort();
  }, [canRead, loadEvents, refreshKey]);

  function applyFilters(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const from = toApiDateTime(draft.from);
    const to = toApiDateTime(draft.to);
    if (draft.from && !from || draft.to && !to) {
      setFilterError("Enter a valid start and end date.");
      return;
    }
    if (from && to && new Date(from).getTime() > new Date(to).getTime()) {
      setFilterError("The start date must be before the end date.");
      return;
    }
    setFilterError(null);
    setLoading(true);
    setListError(null);
    setOffset(0);
    setFilters({
      actorId: draft.actorId.trim(),
      action: draft.action.trim(),
      resourceType: draft.resourceType.trim(),
      resourceId: draft.resourceId.trim(),
      from: draft.from,
      to: draft.to,
    });
  }

  function clearFilters() {
    setDraft(emptyFilter);
    setFilters(emptyFilter);
    setFilterError(null);
    setLoading(true);
    setListError(null);
    setOffset(0);
    setRefreshKey((key) => key + 1);
  }

  function refreshEvents() {
    setLoading(true);
    setListError(null);
    setRefreshKey((key) => key + 1);
  }

  function changePage(nextOffset: number) {
    setLoading(true);
    setListError(null);
    setOffset(nextOffset);
  }

  if (!canRead) {
    return (
      <>
        <PageHeader title="Audit log" description="Review administrative and account activity." />
        <Panel title="Access restricted">
          <EmptyState>Audit events are available to accounts with the audit.read permission.</EmptyState>
        </Panel>
      </>
    );
  }

  const firstItem = total === 0 ? 0 : offset + 1;
  const lastItem = Math.min(offset + pageSize, total);

  return (
    <div className="grid gap-4 pb-6">
      <PageHeader
        title="Audit log"
        description="."
        actions={(
          <button
            type="button"
            className="button-secondary"
            onClick={refreshEvents}
            disabled={loading}
          >
            <RefreshCw size={16} aria-hidden="true" />
            Refresh
          </button>
        )}
      />

      {developmentOpen ? (
        <p className="rounded-lg border border-blue-200 bg-blue-50 px-4 py-3 text-sm leading-5 text-blue-900">
          Development-open mode lets you preview this page. The API still checks your real session and audit.read permission before returning events.
        </p>
      ) : null}

      <Panel title="Find events">
        <form className="grid gap-4 p-4" onSubmit={applyFilters}>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <label className="workspace-field">
              <span className="field-label">Actor account ID</span>
              <input
                className="form-input"
                value={draft.actorId}
                maxLength={200}
                onChange={(event) => setDraft({ ...draft, actorId: event.target.value })}
                placeholder="Account or system actor"
              />
            </label>
            <label className="workspace-field">
              <span className="field-label">Action</span>
              <input
                className="form-input"
                value={draft.action}
                maxLength={160}
                onChange={(event) => setDraft({ ...draft, action: event.target.value })}
                placeholder="e.g. staff.updated"
              />
            </label>
            <label className="workspace-field">
              <span className="field-label">Resource type</span>
              <input
                className="form-input"
                value={draft.resourceType}
                maxLength={100}
                onChange={(event) => setDraft({ ...draft, resourceType: event.target.value })}
                placeholder="e.g. staff_member"
              />
            </label>
            <label className="workspace-field">
              <span className="field-label">Resource ID</span>
              <input
                className="form-input"
                value={draft.resourceId}
                maxLength={200}
                onChange={(event) => setDraft({ ...draft, resourceId: event.target.value })}
                placeholder="Resource identifier"
              />
            </label>
            <label className="workspace-field">
              <span className="field-label">From</span>
              <input
                className="form-input"
                type="datetime-local"
                value={draft.from}
                onChange={(event) => setDraft({ ...draft, from: event.target.value })}
              />
            </label>
            <label className="workspace-field">
              <span className="field-label">To</span>
              <input
                className="form-input"
                type="datetime-local"
                value={draft.to}
                onChange={(event) => setDraft({ ...draft, to: event.target.value })}
              />
            </label>
          </div>
          {filterError ? <p role="alert" className="field-error">{filterError}</p> : null}
          <div className="flex flex-wrap items-center gap-2">
            <button type="submit" className="button-primary">
              <Search size={16} aria-hidden="true" />
              Apply filters
            </button>
            <button type="button" className="button-secondary" onClick={clearFilters}>
              <FilterX size={16} aria-hidden="true" />
              Clear
            </button>
            <p className="ml-auto text-xs text-[var(--muted)]">
              Filters match exact actor account, action, and resource identifiers.
            </p>
          </div>
        </form>
      </Panel>

      <Panel
        title="Activity"
        action={(
          <span className="text-xs text-[var(--muted)]" aria-live="polite">
            {loading ? "Updating…" : total === 1 ? "1 event" : `${total.toLocaleString()} events`}
          </span>
        )}
      >
        {loading ? (
          <LoadingTable columns={5} rows={6} />
        ) : listError ? (
          <div className="p-4">
            <ErrorBanner message={listError} onRetry={refreshEvents} />
          </div>
        ) : events.length === 0 ? (
          <EmptyState>
            {Object.values(filters).some(Boolean)
              ? "No audit events match these filters. Try broadening your search."
              : "No audit events are available yet."}
          </EmptyState>
        ) : isMobile ? (
          <ul className="audit-summary-list" role="list" aria-label="Recent audit events">
            {events.map((event) => (
              <li key={event.id}>
                <Link
                  className="audit-summary-link"
                  href={`/audit-log/${encodeURIComponent(event.id)}`}
                  aria-label={`View details for ${displayLabel(event.action)} by ${event.actor_name || (event.actor_type === "system" ? "System" : "Name unavailable")}`}
                >
                <div className="audit-summary-meta flex items-center justify-between gap-2">
                  <div>
                    <span className="block min-w-0 break-words font-medium">
                      {event.actor_name || (event.actor_type === "system" ? "System" : "Name unavailable")}
                    </span>
                    <span className="text-xs text-[var(--muted)]">{displayLabel(event.actor_type)}</span>
                  </div>
                  <time dateTime={event.occurred_at} className="shrink-0 text-xs text-[var(--muted)]">
                    {formatDate(event.occurred_at)}
                  </time>
                </div>
                <span className="block break-words font-medium text-[var(--text)]">{displayLabel(event.action)}</span>
                <span className="audit-summary-resource">
                  {displayLabel(event.resource_type)} · {event.resource_name || "Name unavailable"}
                </span>
                </Link>
              </li>
            ))}
          </ul>
        ) : (
          <>
            <DataTable columns={["Actor", "Action", "Resource", "Time", ""]} className="audit-event-table">
              {events.map((event) => (
                <tr key={event.id}>
                  <td>
                    <span className="block max-w-48 truncate font-medium">
                      {event.actor_name || (event.actor_type === "system" ? "System" : "Name unavailable")}
                    </span>
                    <span className="text-xs text-[var(--muted)]">{displayLabel(event.actor_type)}</span>
                  </td>
                  <td><span className="break-words font-medium">{displayLabel(event.action)}</span></td>
                  <td>
                    <span className="block">{displayLabel(event.resource_type)}</span>
                    <span className="block max-w-48 truncate text-xs text-[var(--muted)]">
                      {event.resource_name || "Name unavailable"}
                    </span>
                  </td>
                  <td className="whitespace-nowrap">
                    <span className="inline-flex items-center gap-1.5">
                      <Clock3 size={14} className="text-[var(--muted)]" aria-hidden="true" />
                      <time dateTime={event.occurred_at}>{formatDate(event.occurred_at)}</time>
                    </span>
                  </td>
                  <td className="text-right">
                    <Link
                      className="button-ghost"
                      href={`/audit-log/${encodeURIComponent(event.id)}`}
                      aria-label={`View details for ${displayLabel(event.action)} by ${event.actor_name || (event.actor_type === "system" ? "System" : "Name unavailable")}`}
                    >
                      <span>Details</span>
                    </Link>
                  </td>
                </tr>
              ))}
            </DataTable>
            <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border)] px-4 py-3">
              <p className="text-sm text-[var(--muted)]" aria-live="polite">
                Showing {firstItem.toLocaleString()}–{lastItem.toLocaleString()} of {total.toLocaleString()}
              </p>
              <nav aria-label="Audit event pages" className="flex items-center gap-2">
                <button
                  type="button"
                  className="button-secondary"
                  onClick={() => changePage(Math.max(0, offset - pageSize))}
                  disabled={offset === 0}
                >
                  <ChevronLeft size={16} aria-hidden="true" />
                  Previous
                </button>
                <button
                  type="button"
                  className="button-secondary"
                  onClick={() => changePage(offset + pageSize)}
                  disabled={offset + pageSize >= total}
                >
                  Next
                  <ChevronRight size={16} aria-hidden="true" />
                </button>
              </nav>
            </div>
          </>
        )}
      </Panel>
    </div>
  );
}
