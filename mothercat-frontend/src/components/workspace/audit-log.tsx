"use client";

import {
  ChevronLeft,
  ChevronRight,
  Clock3,
  Eye,
  FilterX,
  RefreshCw,
  Search,
} from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import type { FormEvent } from "react";

import { Dialog } from "@/components/workspace/dialog";
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
  getAuditEvent,
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
  return value.replaceAll("_", " ");
}

function AuditEventDetails({
  event,
  loading,
  error,
  onRetry,
}: {
  event: AuditEvent | null;
  loading: boolean;
  error: string | null;
  onRetry: () => void;
}) {
  if (loading) {
    return <p role="status" aria-live="polite" className="text-sm text-[var(--muted)]">Loading event details…</p>;
  }
  if (error) return <ErrorBanner message={error} onRetry={onRetry} />;
  if (!event) return null;

  return (
    <div className="space-y-5">
      <dl className="grid gap-4 sm:grid-cols-2">
        <div>
          <dt className="field-label">Action</dt>
          <dd className="mt-1 break-words text-sm font-medium text-[var(--text)]">{event.action}</dd>
        </div>
        <div>
          <dt className="field-label">Time</dt>
          <dd className="mt-1 text-sm text-[var(--text)]">{formatDate(event.occurred_at)}</dd>
        </div>
        <div>
          <dt className="field-label">Actor</dt>
          <dd className="mt-1 break-all text-sm text-[var(--text)]">
            {event.actor_id} <span className="text-[var(--muted)]">({displayLabel(event.actor_type)})</span>
          </dd>
        </div>
        <div>
          <dt className="field-label">Resource</dt>
          <dd className="mt-1 break-all text-sm text-[var(--text)]">
            {displayLabel(event.resource_type)} · {event.resource_id}
          </dd>
        </div>
        <div className="sm:col-span-2">
          <dt className="field-label">Event ID</dt>
          <dd className="mt-1 break-all font-mono text-xs text-[var(--muted)]">{event.id}</dd>
        </div>
      </dl>
      <section aria-labelledby="audit-event-data-title">
        <h3 id="audit-event-data-title" className="field-label">Event data</h3>
        <pre className="mt-2 max-h-72 overflow-auto rounded-lg border border-[var(--border)] bg-[var(--page-bg)] p-4 text-xs leading-5 text-[var(--text)]">
          {JSON.stringify(event.details, null, 2)}
        </pre>
      </section>
    </div>
  );
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
  const [selectedEventId, setSelectedEventId] = useState<string | null>(null);
  const [selectedEvent, setSelectedEvent] = useState<AuditEvent | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [detailRefreshKey, setDetailRefreshKey] = useState(0);
  const [filterError, setFilterError] = useState<string | null>(null);

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

  useEffect(() => {
    if (!selectedEventId) return;
    const controller = new AbortController();
    void getAuditEvent(selectedEventId, controller.signal)
      .then((event) => {
        if (!controller.signal.aborted) setSelectedEvent(event);
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setDetailError(formatAuditError(error, "Unable to load this audit event.", developmentOpen));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setDetailLoading(false);
      });
    return () => controller.abort();
  }, [developmentOpen, selectedEventId, detailRefreshKey]);

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

  function openDetails(eventId: string) {
    setSelectedEvent(null);
    setDetailLoading(true);
    setDetailError(null);
    setSelectedEventId(eventId);
  }

  function retryDetails() {
    setSelectedEvent(null);
    setDetailLoading(true);
    setDetailError(null);
    setDetailRefreshKey((key) => key + 1);
  }

  function closeDetails() {
    setSelectedEventId(null);
    setSelectedEvent(null);
    setDetailError(null);
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
        description="Review administrative and account activity recorded by the system."
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
              <span className="field-label">Actor ID</span>
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
              Filters match exact actor, action, and resource values.
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
        ) : (
          <>
            <DataTable columns={["Actor", "Action", "Resource", "Time", ""]}>
              {events.map((event) => (
                <tr key={event.id}>
                  <td>
                    <span className="block max-w-48 truncate font-medium" title={event.actor_id}>{event.actor_id}</span>
                    <span className="text-xs text-[var(--muted)]">{displayLabel(event.actor_type)}</span>
                  </td>
                  <td><span className="break-all font-medium">{event.action}</span></td>
                  <td>
                    <span className="block">{displayLabel(event.resource_type)}</span>
                    <span className="block max-w-48 truncate text-xs text-[var(--muted)]" title={event.resource_id}>
                      {event.resource_id}
                    </span>
                  </td>
                  <td className="whitespace-nowrap">
                    <span className="inline-flex items-center gap-1.5">
                      <Clock3 size={14} className="text-[var(--muted)]" aria-hidden="true" />
                      <time dateTime={event.occurred_at}>{formatDate(event.occurred_at)}</time>
                    </span>
                  </td>
                  <td className="text-right">
                    <button
                      type="button"
                      className="button-ghost"
                      aria-label={`View details for ${event.action} by ${event.actor_id}`}
                      onClick={() => openDetails(event.id)}
                    >
                      <Eye size={16} aria-hidden="true" />
                      <span className="hidden sm:inline">Details</span>
                    </button>
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

      <Dialog
        open={selectedEventId !== null}
        title="Audit event details"
        onClose={closeDetails}
        actions={(
          <button type="button" className="button-secondary" onClick={closeDetails}>
            Close
          </button>
        )}
      >
        <AuditEventDetails
          event={selectedEvent}
          loading={detailLoading}
          error={detailError}
          onRetry={retryDetails}
        />
      </Dialog>
    </div>
  );
}
