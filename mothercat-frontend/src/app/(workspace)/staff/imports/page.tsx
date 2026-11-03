import { StaffImport } from "@/components/workspace/staff-import";
import { getCurrentSession } from "@/lib/auth/session";
import { hasPermission } from "@/lib/auth/permissions";

export default async function StaffImportsPage() {
  const session = await getCurrentSession();
  return <StaffImport canImport={hasPermission(session, "staff.import")} />;
}
