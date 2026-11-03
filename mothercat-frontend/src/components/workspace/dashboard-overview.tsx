"use client";

import Link from "next/link";
import Image from "next/image";
import { useEffect, useState } from "react";

import { EmptyState, ErrorBanner, Panel, StatCard } from "@/components/workspace/ui";
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
  departments: DepartmentPreview[];
  departmentCount: number;
};

function initials(name: string) {
  return name.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? "").join("") || "S";
}

function DashboardTeamAvatar({ member }: { member: StaffMember }) {
  const [imageFailed, setImageFailed] = useState(false);
  const imageUrl = member.profile_image_url;
  const canShowImage = imageUrl && (imageUrl.startsWith("/") || /^https?:\/\//i.test(imageUrl));

  return (
    <Link
      href={`/staff/${encodeURIComponent(member.id)}`}
      className="dashboard-team-avatar"
      aria-label={`${member.full_name}, ${member.employment_status === "active" ? "active" : "not active"}`}
      title={member.full_name}
    >
      {canShowImage && !imageFailed ? (
        <Image
          src={imageUrl}
          alt=""
          fill
          sizes="36px"
          unoptimized
          className="dashboard-team-photo"
          onError={() => setImageFailed(true)}
        />
      ) : (
        <span className="dashboard-team-initials" aria-hidden="true">{initials(member.full_name)}</span>
      )}
      <span
        className={`dashboard-team-status ${member.employment_status === "active" ? "is-active" : "is-inactive"}`}
        aria-hidden="true"
      />
    </Link>
  );
}

export function DashboardOverview() {
  const [data, setData] = useState<DashboardData | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let active = true;
    async function loadDashboard() {
      try {
        const [allStaff, activeStaff, pendingStaff, allDepartments] = await Promise.all([
          listStaff({ limit: 1, offset: 0 }),
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
            <StatCard label="Total staff" value={data ? String(data.totalStaff) : "Unavailable"} href="/staff" />
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
                  <div className="dashboard-department-info">
                    <Link href="/departments" className="dashboard-department-name">{department.name}</Link>
                    <span className="dashboard-department-count">{total} {total === 1 ? "staff member" : "staff members"}</span>
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
          <EmptyState>No recent activity yet. Administrative changes will appear here.</EmptyState>
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
