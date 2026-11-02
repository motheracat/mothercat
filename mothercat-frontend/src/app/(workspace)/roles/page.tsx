import { DataTable, EmptyState, PageHeader } from "@/components/workspace/ui";

export default function RolesPage() {
  return (
    <>
      <PageHeader title="Roles & permissions" description="Review roles and their assigned access." />
      <section className="surface-card">
        <DataTable
          columns={["Role", "Description", "Members", "Permissions", ""]}
          empty={<EmptyState>No role data is available yet.</EmptyState>}
        />
      </section>
    </>
  );
}
