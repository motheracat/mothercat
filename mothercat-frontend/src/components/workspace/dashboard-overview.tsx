"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import { DataTable, EmptyState, ErrorBanner, PageHeader, Panel, StatCard, StatusPill } from "@/components/workspace/ui";
import { useWorkspace } from "@/components/workspace/workspace-context";
import { hasPermission } from "@/lib/auth/permissions";
import { formatOrganizationError, listDepartments, listStaff } from "@/lib/organization/api";
import type { DepartmentRecord, StaffRecord } from "@/lib/organization/api";

type DashboardData = {
  totalStaff: number;
  activeStaff: number;
  pendingStaff: number;
  latestStaff: StaffRecord[];
  pendingRecords: StaffRecord[];
  departments: DepartmentRecord[];
};

export function DashboardOverview({ date }: { date: string }) {
  const { session, devOpen } = useWorkspace();
  const canRead = !devOpen && hasPermission(session, "staff.read");
  const [data, setData] = useState<DashboardData | null>(null);
  const [loadState, setLoadState] = useState<{ key: number; error: string | null } | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const loading = canRead && loadState?.key !== refreshKey;
  const error = loadState?.key === refreshKey ? loadState.error : null;

  const loadDashboard = useCallback(async (signal: AbortSignal): Promise<DashboardData> => {
    const [allStaff, activeStaff, pendingStaff, departments] = await Promise.all([
      listStaff({ limit: 5, offset: 0 }, signal),
      listStaff({ limit: 1, offset: 0, employment_status: "active" }, signal),
      listStaff({ limit: 5, offset: 0, employment_status: "pending" }, signal),
      listDepartments(signal),
    ]);
    return {
      totalStaff: allStaff.pagination.total,
      activeStaff: activeStaff.pagination.total,
      pendingStaff: pendingStaff.pagination.total,
      latestStaff: allStaff.staff,
      pendingRecords: pendingStaff.staff,
      departments: departments.departments,
    };
  }, []);

  useEffect(() => {
    if (!canRead) return;
    const controller = new AbortController();
    void loadDashboard(controller.signal)
      .then((result) => {
        setData(result);
        setLoadState({ key: refreshKey, error: null });
      })
      .catch((requestError: unknown) => {
        if (!controller.signal.aborted) {
          setLoadState({ key: refreshKey, error: formatOrganizationError(requestError, "Unable to load dashboard data.") });
        }
      });
    return () => controller.abort();
  }, [canRead, loadDashboard, refreshKey]);

  const refresh = useCallback(() => setRefreshKey((value) => value + 1), []);
  const count = (value: number | undefined) => loading ? "…" : value === undefined ? "—" : value.toLocaleString();

  return (
    <div className="dashboard-page">
      <PageHeader title="Dashboard" description={date} />
      {devOpen ? <p className="field-error mb-4">Development navigation only. Organization data is not connected in this mode.</p> : null}
      {!canRead && !devOpen ? <p role="alert" className="field-error mb-4">Your account does not have permission to read organization data.</p> : null}
      {error ? <div className="mb-4"><ErrorBanner message={error} onRetry={refresh} /></div> : null}

      <section aria-label="Key metrics" className="dashboard-kpis">
        <StatCard label="Total staff" value={count(data?.totalStaff)} href="/staff" />
        <StatCard label="Departments" value={count(data?.departments.length)} href="/departments" />
        <StatCard label="Active staff" value={count(data?.activeStaff)} href="/staff" />
        <StatCard label="Pending staff" value={count(data?.pendingStaff)} href="/staff" />
      </section>

      {loading && !data ? <p role="status" aria-live="polite" className="mb-4 text-sm text-[var(--muted)]">Loading organization overview…</p> : null}

      <div className="dashboard-overview-grid">
        <Panel
          title="Departments"
          className="dashboard-panel dashboard-departments"
          action={<Link href="/departments" className="text-sm font-medium text-[var(--accent)] hover:underline">View all</Link>}
        >
          <DataTable
            columns={["Name", "Description"]}
            empty={error
              ? <span className="text-sm text-[var(--muted)]">Department data is unavailable.</span>
              : loading
                ? <span role="status" className="text-sm text-[var(--muted)]">Loading departments…</span>
                : canRead
                  ? <EmptyState>No departments yet.</EmptyState>
                  : <span className="text-sm text-[var(--muted)]">Department data is unavailable.</span>}
          >
            {data?.departments.length ? data.departments.slice(0, 5).map((department) => (
              <tr key={department.id}>
                <td className="font-medium">{department.name}</td>
                <td className="max-w-[360px] whitespace-normal">{department.description || "—"}</td>
              </tr>
            )) : undefined}
          </DataTable>
        </Panel>

        <Panel
          title="Recently added staff"
          className="dashboard-panel dashboard-activity"
          action={<Link href="/staff" className="text-sm font-medium text-[var(--accent)] hover:underline">View directory</Link>}
        >
          <DataTable
            columns={["Staff member", "Status"]}
            empty={error
              ? <span className="text-sm text-[var(--muted)]">Staff data is unavailable.</span>
              : loading
                ? <span role="status" className="text-sm text-[var(--muted)]">Loading staff…</span>
                : canRead
                  ? <EmptyState>No staff records yet.</EmptyState>
                  : <span className="text-sm text-[var(--muted)]">Staff data is unavailable.</span>}
          >
            {data?.latestStaff.length ? data.latestStaff.map((record) => (
              <tr key={record.id}>
                <td>
                  <span className="block font-medium">{record.full_name}</span>
                  <span className="text-xs text-[var(--muted)]">ID:{record.staff_id}</span>
                </td>
                <td><StatusPill label={record.employment_status} tone={record.employment_status === "active" ? "success" : record.employment_status === "pending" ? "warning" : "neutral"} /></td>
              </tr>
            )) : undefined}
          </DataTable>
        </Panel>
      </div>

      <Panel
        title="Pending staff profiles"
        className="dashboard-panel dashboard-pending"
        action={<Link href="/staff" className="text-sm font-medium text-[var(--accent)] hover:underline">Review directory</Link>}
      >
        {data?.pendingRecords.length ? (
          <ul className="dashboard-pending-list">
            {data.pendingRecords.map((record) => (
              <li key={record.id}>
                <span className="font-medium">{record.full_name}</span>
                <span className="text-sm text-[var(--muted)]">ID:{record.staff_id}</span>
              </li>
            ))}
            {data.pendingStaff > data.pendingRecords.length ? (
              <li className="text-sm text-[var(--muted)]">+{data.pendingStaff - data.pendingRecords.length} more pending profiles</li>
            ) : null}
          </ul>
        ) : (
          <EmptyState>{data ? "No pending staff profiles." : "Pending staff totals will appear when dashboard data loads."}</EmptyState>
        )}
      </Panel>
    </div>
  );
}
