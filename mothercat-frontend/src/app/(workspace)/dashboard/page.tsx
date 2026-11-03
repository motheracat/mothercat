import { DashboardOverview } from "@/components/workspace/dashboard-overview";

export default function DashboardPage() {
  const today = new Date().toLocaleDateString("en", {
    weekday: "long",
    year: "numeric",
    month: "long",
    day: "numeric",
  });

  return <DashboardOverview date={today} />;
}
