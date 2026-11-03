import { DashboardOverview } from "@/components/workspace/dashboard-overview";
import { EmptyState, PageHeader, Panel } from "@/components/workspace/ui";

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
      <DashboardOverview />
      <Panel title="Pending requests" className="dashboard-panel dashboard-pending">
        <EmptyState>No pending requests to review.</EmptyState>
      </Panel>
    </div>
  );
}
