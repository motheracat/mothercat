import { DashboardOverview } from "@/components/workspace/dashboard-overview";
import { EmptyState, Panel } from "@/components/workspace/ui";

export default function DashboardPage() {
  const today = new Date().toLocaleDateString("en", {
    weekday: "long",
    year: "numeric",
    month: "long",
    day: "numeric",
  });

  return (
    <div className="dashboard-page">
      <header className="page-header">
        <div>
          <h1 className="sr-only">Dashboard</h1>
          <p className="page-description !mt-0">{today}</p>
        </div>
      </header>
      <DashboardOverview />
      <Panel title="Pending requests" className="dashboard-panel dashboard-pending">
        <EmptyState>No pending requests to review.</EmptyState>
      </Panel>
    </div>
  );
}
