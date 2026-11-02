import { DataTable, EmptyState, PageHeader, Toolbar } from "@/components/workspace/ui";

export default function AuditPage() {
  return (
    <>
      <PageHeader title="Audit log" description="Review recent account and workspace activity." />
      <Toolbar>
        <label className="w-full sm:w-48">
          <span className="sr-only">Filter audit events by actor</span>
          <select className="form-select" defaultValue="">
            <option value="">All actors</option>
          </select>
        </label>
        <label className="w-full sm:w-48">
          <span className="sr-only">Filter audit events by action</span>
          <select className="form-select" defaultValue="">
            <option value="">All actions</option>
          </select>
        </label>
      </Toolbar>
      <section className="surface-card">
        <DataTable
          columns={["Actor", "Action", "Target", "Time"]}
          empty={<EmptyState>No audit events are available yet.</EmptyState>}
        />
      </section>
    </>
  );
}
