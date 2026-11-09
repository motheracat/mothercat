"use client";

import Link from "next/link";
import Image from "next/image";
import { useEffect, useState } from "react";

import { Clock3, Eye } from "lucide-react";

import { Dialog } from "@/components/workspace/dialog";
import { DataTable, EmptyState, ErrorBanner, LoadingTable, Panel, StatCard } from "@/components/workspace/ui";
import { formatAuditError, listAuditEvents } from "@/lib/audit/api";
import type { AuditEvent } from "@/lib/audit/api";
import {
  formatOrganizationError,
  listDepartments,
  listStaff,
  type Department,
  type StaffMember,
} from "@/lib/organization/api";

type DepartmentPreview = {
  department: Department;
  staff: StaffMember[];
  total: number;
};

type DashboardData = {
  totalStaff: number;
  activeStaff: number;
  pendingStaff: number;
  staffPreview: StaffMember[];
  departments: DepartmentPreview[];
  departmentCount: number;
};

function initials(name: string) {
  return name.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? "").join("") || "S";
}

function formatActivityDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Unknown time";
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
}

function activityLabel(value: string) {
  return value.replace(/[._-]+/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function DashboardAvatarFace({ member }: { member: StaffMember }) {
  const [failedImageUrl, setFailedImageUrl] = useState<string | null>(null);
  const imageUrl = member.profile_image_url;
  const canShowImage = imageUrl && (imageUrl.startsWith("/") || /^https?:\/\//i.test(imageUrl));

  return (
    <>
      {canShowImage && failedImageUrl !== imageUrl ? (
        <Image
          src={imageUrl}
          alt=""
          fill
          sizes="36px"
          unoptimized
          className="dashboard-team-photo"
          onError={() => setFailedImageUrl(imageUrl)}
        />
      ) : (
        <span className="dashboard-team-initials" aria-hidden="true">{initials(member.full_name)}</span>
      )}
      <span
        className={`dashboard-team-status ${member.employment_status === "active" ? "is-active" : "is-inactive"}`}
        aria-hidden="true"
      />
    </>
  );
}

function DashboardTeamAvatar({ member }: { member: StaffMember }) {
  return (
    <Link
      href={`/staff/${encodeURIComponent(member.id)}`}
      className="dashboard-team-avatar"
      aria-label={`${member.full_name}, ${member.employment_status === "active" ? "active" : "not active"}`}
      title={member.full_name}
    >
      <DashboardAvatarFace member={member} />
    </Link>
  );
}

function DashboardPeoplePreview({ staff, hasMore }: { staff: StaffMember[]; hasMore: boolean }) {
  return (
    <Link href="/staff" className="surface-card dashboard-people-card" aria-label="Browse staff profiles">
      <div className="dashboard-people-stack" aria-hidden="true">
        {staff.slice(0, 5).map((member) => (
          <span className="dashboard-people-avatar" key={member.id} title={member.full_name}>
            <DashboardAvatarFace member={member} />
          </span>
        ))}
        {hasMore ? <span className="dashboard-people-more"><span>•••</span></span> : null}
      </div>
    </Link>
  );
}

export function DashboardOverview({ canReadAudit }: { canReadAudit: boolean }) {
  const [data, setData] = useState<DashboardData | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [reloadKey, setReloadKey] = useState(0);
  const [recentEvents, setRecentEvents] = useState<AuditEvent[]>([]);
  const [recentEventsError, setRecentEventsError] = useState<string | null>(null);
  const [recentEventsLoading, setRecentEventsLoading] = useState(canReadAudit);
  const [recentEventsReloadKey, setRecentEventsReloadKey] = useState(0);
  const [selectedRecentEvent, setSelectedRecentEvent] = useState<AuditEvent | null>(null);

  useEffect(() => {
    let active = true;
    async function loadDashboard() {
      try {
        const [allStaff, staffPreview, activeStaff, pendingStaff, allDepartments] = await Promise.all([
          listStaff({ limit: 1, offset: 0 }),
          listStaff({ limit: 5, offset: 0 }),
          listStaff({ limit: 1, offset: 0, employmentStatus: "active" }),
          listStaff({ limit: 1, offset: 0, employmentStatus: "pending" }),
          listDepartments(),
        ]);
        const shownDepartments = allDepartments.slice(0, 5);
        const previews = await Promise.all(
          shownDepartments.map(async (department) => {
            const result = await listStaff({ limit: 5, offset: 0, departmentId: department.id });
            return { department, staff: result.staff, total: result.total };
          }),
        );
        if (!active) return;
        setData({
          totalStaff: allStaff.total,
          activeStaff: activeStaff.total,
          pendingStaff: pendingStaff.total,
          staffPreview: staffPreview.staff.slice(0, 5),
          departments: previews,
          departmentCount: allDepartments.length,
        });
      } catch (loadError) {
        if (active) setError(formatOrganizationError(loadError, "Unable to load dashboard data."));
      } finally {
        if (active) setLoading(false);
      }
    }

    void loadDashboard();
    return () => {
      active = false;
    };
  }, [reloadKey]);

  useEffect(() => {
    if (!canReadAudit) {
      return;
    }

    const controller = new AbortController();
    void listAuditEvents({ limit: 5, offset: 0 }, controller.signal)
      .then(({ events }) => {
        if (!controller.signal.aborted) setRecentEvents(events);
      })
      .catch((loadError: unknown) => {
        if (!controller.signal.aborted) {
          setRecentEvents([]);
          setRecentEventsError(formatAuditError(loadError, "Unable to load recent activity."));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setRecentEventsLoading(false);
      });

    return () => controller.abort();
  }, [canReadAudit, recentEventsReloadKey]);

  const departmentRows = data?.departments ?? [];

  return (
    <>
      {error ? (
        <ErrorBanner
          message={error}
          onRetry={() => {
            setLoading(true);
            setError(null);
            setReloadKey((key) => key + 1);
          }}
        />
      ) : null}
      <section
        aria-label={loading && !data ? "Loading key metrics" : "Key metrics"}
        aria-busy={loading && !data}
        className="dashboard-kpis"
        role={loading && !data ? "status" : undefined}
      >
        {loading && !data ? (
          Array.from({ length: 4 }, (_, index) => (
            <div key={index} className="surface-card dashboard-stat-card space-y-4" aria-hidden="true">
              <div className="skeleton w-2/3" />
              <div className="skeleton h-9 w-1/3" />
            </div>
          ))
        ) : (
          <>
            <DashboardPeoplePreview staff={data?.staffPreview ?? []} hasMore={(data?.totalStaff ?? 0) > (data?.staffPreview.length ?? 0)} />
            <StatCard label="Departments" value={data ? String(data.departmentCount) : "Unavailable"} href="/departments" />
            <StatCard label="Active staff" value={data ? String(data.activeStaff) : "Unavailable"} href="/staff" />
            <StatCard label="Pending staff" value={data ? String(data.pendingStaff) : "Unavailable"} href="/staff" />
          </>
        )}
      </section>

      <div className="dashboard-overview-grid">
        <Panel
          title="Departments"
          className="dashboard-panel dashboard-departments"
          action={<Link href="/departments" className="text-sm font-medium text-[var(--accent)] hover:underline">View all</Link>}
        >
          {loading && !data ? (
            <ul role="status" aria-label="Loading departments" aria-busy="true" className="dashboard-department-list">
              {Array.from({ length: 4 }, (_, index) => (
                <li key={index} className="dashboard-department-row">
                  <div className="dashboard-department-info flex-1">
                    <div className="skeleton w-2/3" />
                    <div className="skeleton w-1/3" />
                  </div>
                  <div className="flex gap-2">
                    <div className="skeleton h-9 w-9 rounded-full" />
                    <div className="skeleton h-9 w-9 rounded-full" />
                    <div className="skeleton h-9 w-9 rounded-full" />
                  </div>
                </li>
              ))}
            </ul>
          ) : error && !data ? (
            <p className="dashboard-loading">Department and team information is unavailable.</p>
          ) : departmentRows.length === 0 ? (
            <EmptyState>No departments yet. Add a department to see team distribution here.</EmptyState>
          ) : (
            <ul className="dashboard-department-list">
              {departmentRows.map(({ department, staff, total }) => (
                <li key={department.id} className="dashboard-department-row">
                  <Link
                    href="/departments"
                    className="dashboard-department-card-link"
                    aria-label={`View ${department.name} department`}
                  />
                  <div className="dashboard-department-card-content">
                    <div className="dashboard-department-info">
                      <span className="dashboard-department-name">{department.name}</span>
                      <span className="dashboard-department-count">
                        {total} {total === 1 ? "staff member" : "staff members"}
                      </span>
                    </div>
                  </div>
                  <div className="dashboard-team-stack" aria-label={`${department.name} team`}>
                    {staff.map((member) => <DashboardTeamAvatar key={member.id} member={member} />)}
                    {total > staff.length ? (
                      <Link
                        href="/staff"
                        className="dashboard-team-more"
                        aria-label={`View ${total - staff.length} more staff members`}
                      >
                        +{total - staff.length}
                      </Link>
                    ) : null}
                    {staff.length === 0 ? <span className="dashboard-team-empty">No team members</span> : null}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Panel>

        <Panel
          title="Recent activity"
          className="dashboard-panel dashboard-activity"
          action={<Link href="/audit" className="text-sm font-medium text-[var(--accent)] hover:underline">View audit log</Link>}
        >
          {!canReadAudit ? (
            <EmptyState>Recent activity is available to accounts with the audit.read permission.</EmptyState>
          ) : recentEventsLoading ? (
            <LoadingTable columns={5} rows={5} />
          ) : recentEventsError ? (
            <div className="p-4">
              <ErrorBanner
                message={recentEventsError}
                onRetry={() => {
                  setRecentEventsLoading(true);
                  setRecentEventsError(null);
                  setRecentEventsReloadKey((key) => key + 1);
                }}
              />
            </div>
          ) : recentEvents.length === 0 ? (
            <EmptyState>No recent activity yet. Administrative changes will appear here.</EmptyState>
          ) : (
            <DataTable columns={["Actor", "Action", "Resource", "Time", ""]}>
              {recentEvents.map((event) => (
                <tr key={event.id}>
                  <td>
                    <span className="block max-w-48 truncate font-medium">
                      {event.actor_name || (event.actor_type === "system" ? "System" : "Name unavailable")}
                    </span>
                    <span className="text-xs text-[var(--muted)]">{activityLabel(event.actor_type)}</span>
                  </td>
                  <td><span className="break-words font-medium">{activityLabel(event.action)}</span></td>
                  <td>
                    <span className="block">{activityLabel(event.resource_type)}</span>
                    <span className="block max-w-48 truncate text-xs text-[var(--muted)]">
                      {event.resource_name || "Name unavailable"}
                    </span>
                  </td>
                  <td className="whitespace-nowrap">
                    <span className="inline-flex items-center gap-1.5">
                      <Clock3 size={14} className="text-[var(--muted)]" aria-hidden="true" />
                      <time dateTime={event.occurred_at}>{formatActivityDate(event.occurred_at)}</time>
                    </span>
                  </td>
                  <td className="text-right">
                    <button
                      type="button"
                      className="button-ghost"
                      aria-label={`View details for ${activityLabel(event.action)} by ${event.actor_name || (event.actor_type === "system" ? "System" : "Name unavailable")}`}
                      onClick={() => setSelectedRecentEvent(event)}
                    >
                      <Eye size={16} aria-hidden="true" />
                      <span className="hidden sm:inline">Details</span>
                    </button>
                  </td>
                </tr>
              ))}
            </DataTable>
          )}
        </Panel>
      </div>

      <Dialog
        open={selectedRecentEvent !== null}
        title="Audit event details"
        onClose={() => setSelectedRecentEvent(null)}
        actions={(
          <button type="button" className="button-secondary" onClick={() => setSelectedRecentEvent(null)}>
            Close
          </button>
        )}
      >
        {selectedRecentEvent ? (
          <div className="space-y-5">
            <dl className="grid gap-4 sm:grid-cols-2">
              <div>
                <dt className="field-label">Action</dt>
                <dd className="mt-1 break-words text-sm font-medium text-[var(--text)]">
                  {activityLabel(selectedRecentEvent.action)}
                </dd>
              </div>
              <div>
                <dt className="field-label">Time</dt>
                <dd className="mt-1 text-sm text-[var(--text)]">{formatActivityDate(selectedRecentEvent.occurred_at)}</dd>
              </div>
              <div>
                <dt className="field-label">Actor</dt>
                <dd className="mt-1 text-sm text-[var(--text)]">
                  {selectedRecentEvent.actor_name || (selectedRecentEvent.actor_type === "system" ? "System" : "Name unavailable")}
                  <span className="text-[var(--muted)]"> · {activityLabel(selectedRecentEvent.actor_type)}</span>
                </dd>
              </div>
              <div>
                <dt className="field-label">Resource</dt>
                <dd className="mt-1 text-sm text-[var(--text)]">
                  {selectedRecentEvent.resource_name || "Name unavailable"}
                  <span className="text-[var(--muted)]"> · {activityLabel(selectedRecentEvent.resource_type)}</span>
                </dd>
              </div>
            </dl>
            <section aria-labelledby="dashboard-audit-details-title">
              <h3 id="dashboard-audit-details-title" className="field-label">Change details</h3>
              <pre className="mt-2 overflow-auto rounded-lg border border-[var(--border)] bg-[var(--page-bg)] p-4 text-sm leading-5 text-[var(--text)]">
                {JSON.stringify(selectedRecentEvent.details, null, 2) ?? "Not provided"}
              </pre>
            </section>
          </div>
        ) : null}
      </Dialog>

      {data && data.departmentCount > departmentRows.length ? (
        <p className="dashboard-more-departments">
          Showing {departmentRows.length} of {data.departmentCount} departments.
          {" "}<Link href="/departments">View all departments</Link>
        </p>
      ) : null}
    </>
  );
}
