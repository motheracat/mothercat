import Link from "next/link";
import { Search } from "lucide-react";

import { DataTable, EmptyState, PageHeader, Toolbar } from "@/components/workspace/ui";

export default function StaffPage() {
  return (
    <>
      <PageHeader
        title="Staff directory"
        description="View and manage staff records."
        actions={
          <>
            <Link href="/staff/imports" className="button-secondary">Import</Link>
            <button type="button" className="button-primary">Add staff</button>
          </>
        }
      />

      <Toolbar>
        <label className="relative min-w-[220px] flex-1">
          <span className="sr-only">Search staff by name, email or department</span>
          <Search size={16} aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted)]" />
          <input className="form-input pl-9" placeholder="Search name, email, department" />
        </label>
        <label className="w-full sm:w-44">
          <span className="sr-only">Filter by department</span>
          <select className="form-select" defaultValue="">
            <option value="">All departments</option>
          </select>
        </label>
        <label className="w-full sm:w-36">
          <span className="sr-only">Filter by status</span>
          <select className="form-select" defaultValue="">
            <option value="">All statuses</option>
            <option value="active">Active</option>
            <option value="inactive">Inactive</option>
          </select>
        </label>
        <span className="ml-auto self-center text-sm text-[var(--muted)]">Staff records</span>
      </Toolbar>

      <section className="surface-card">
        <DataTable
          columns={["Staff", "Department", "Job title", "Status", "Last active", ""]}
          empty={
            <EmptyState
              action={
                <>
                  <button type="button" className="button-primary">Add staff</button>
                  <Link href="/staff/imports" className="button-secondary">Import CSV</Link>
                </>
              }
            >
              No staff yet. Add your first staff member or import a CSV.
            </EmptyState>
          }
        />
      </section>
    </>
  );
}
