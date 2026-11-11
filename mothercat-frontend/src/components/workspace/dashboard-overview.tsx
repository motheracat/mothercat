"use client";

import Link from "next/link";
import Image from "next/image";
import { useEffect, useState } from "react";

import { EmptyState, ErrorBanner, Panel } from "@/components/workspace/ui";
import { DashboardAnalytics } from "@/components/workspace/dashboard-analytics";
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

function formatRelativeActivityDate(value: string) {
  const timestamp = new Date(value).getTime();
  if (Number.isNaN(timestamp)) return "Unknown time";
  const elapsedSeconds = Math.max(0, Math.floor((Date.now() - timestamp) / 1000));
  if (elapsedSeconds < 60) return "Just now";
  if (elapsedSeconds < 3600) return `${Math.floor(elapsedSeconds / 60)}m ago`;
  if (elapsedSeconds < 86_400) return `${Math.floor(elapsedSeconds / 3600)}h ago`;
  if (elapsedSeconds < 604_800) return `${Math.floor(elapsedSeconds / 86_400)}d ago`;
  return formatActivityDate(value);
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
      <Image
        src="/images/blueprint-construction-skyline.png"
        alt=""
        fill
        priority
        sizes="(max-width: 600px) 100vw, (max-width: 1023px) 50vw, 25vw"
        className="dashboard-people-background"
      />
      <span className="dashboard-people-overlay" aria-hidden="true" />
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

  useEffect(() => {
    let active = true;
    async function loadDashboard() {
      try {
        const [allStaff, staffPreview, allDepartments] = await Promise.all([
          listStaff({ limit: 1, offset: 0 }),
          listStaff({ limit: 5, offset: 0 }),
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
            <DashboardAnalytics />
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
            <ul className="dashboard-activity-list" role="status" aria-label="Loading recent activity" aria-busy="true">
              {Array.from({ length: 5 }, (_, index) => (
                <li className="dashboard-activity-skeleton" key={index}>
                  <span className="skeleton w-3/4" />
                  <span className="skeleton w-1/2" />
                </li>
              ))}
            </ul>
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
            <ul className="dashboard-activity-list" aria-label="Recent activity">
              {recentEvents.slice(0, 5).map((event) => (
                <li key={event.id}>
                  <Link className="dashboard-activity-link" href={`/audit-log/${encodeURIComponent(event.id)}`}>
                    <span className="dashboard-activity-title">
                      <span className="dashboard-activity-actor">
                        {event.actor_name || (event.actor_type === "system" ? "System" : "—")}
                      </span>
                      <span aria-hidden="true"> · </span>
                      <span>{activityLabel(event.action)}</span>
                    </span>
                    <span className="dashboard-activity-subtitle">
                      <span className="dashboard-activity-resource" title={event.resource_name || "—"}>
                        {event.resource_name || "—"}
                      </span>
                      <time dateTime={event.occurred_at} suppressHydrationWarning>{formatRelativeActivityDate(event.occurred_at)}</time>
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
          {canReadAudit && !recentEventsLoading && !recentEventsError && recentEvents.length > 0 ? (
            <div className="dashboard-activity-footer">
              <Link href="/audit">View all</Link>
            </div>
          ) : null}
        </Panel>
      </div>

      {data && data.departmentCount > departmentRows.length ? (
        <p className="dashboard-more-departments">
          Showing {departmentRows.length} of {data.departmentCount} departments.
          {" "}<Link href="/departments">View all departments</Link>
        </p>
      ) : null}
    </>
  );
}
