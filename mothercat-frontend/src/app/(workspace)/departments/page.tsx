import { DataTable, EmptyState, PageHeader } from "@/components/workspace/ui";

export default function DepartmentsPage() {
  return (
    <>
      <PageHeader
        title="Departments"
        description="Organize teams and department ownership."
        actions={<button type="button" className="button-primary">Add department</button>}
      />

      <section className="surface-card">
        <DataTable
          columns={["Name", "Head", "Staff count", "Created", ""]}
          empty={<EmptyState>No departments yet. Add your first department to organize staff.</EmptyState>}
        />
      </section>
    </>
  );
}
