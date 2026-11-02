import Link from "next/link";

import { DataTable, EmptyState, PageHeader, Panel, StatCard } from "@/components/workspace/ui";

export default function DashboardPage() {
  const today = new Date().toLocaleDateString("en", {
    weekday: "long",
    year: "numeric",
    month: "long",
    day: "numeric",
  });

  return (
    <div className="dashboard-page">
      <PageHeader title="Dashboard" description={today} />

      <section aria-label="Key metrics" className="dashboard-kpis">
        <StatCard label="Total staff" value="—" href="/staff" />
        <StatCard label="Departments" value="—" href="/departments" />
        <StatCard label="Active staff" value="—" href="/staff" />
        <StatCard label="Pending requests" value="—" href="/staff/imports" />
      </section>

      <div className="dashboard-overview-grid">
        <Panel
          title="Departments"
          className="dashboard-panel dashboard-departments"
          action={<Link href="/departments" className="text-sm font-medium text-[var(--accent)] hover:underline">View all</Link>}
        >
          <DataTable
            columns={["Name", "Staff count", "Head"]}
            empty={<EmptyState>No departments yet. Add a department to see team distribution here.</EmptyState>}
          />
        </Panel>

        <Panel
          title="Recent activity"
          className="dashboard-panel dashboard-activity"
          action={<Link href="/audit" className="text-sm font-medium text-[var(--accent)] hover:underline">View audit log</Link>}
        >
          <EmptyState>No recent activity yet. Administrative changes will appear here.</EmptyState>
        </Panel>
      </div>

      <Panel title="Pending requests" className="dashboard-panel dashboard-pending">
        <EmptyState>No pending requests to review.</EmptyState>
      </Panel>
    </div>
  );
}
