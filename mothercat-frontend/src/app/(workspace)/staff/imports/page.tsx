import Link from "next/link";

import { EmptyState, PageHeader, Panel } from "@/components/workspace/ui";

export default function StaffImportsPage() {
  return (
    <>
      <PageHeader title="Staff imports" description="Import staff records from a CSV file." />
      <Panel title="CSV import">
        <EmptyState action={<Link href="/staff" className="button-secondary">Back to staff directory</Link>}>
          CSV import is not available yet.
        </EmptyState>
      </Panel>
    </>
  );
}
