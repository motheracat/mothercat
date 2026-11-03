import { DepartmentManagement } from "@/components/workspace/department-management";
import { getCurrentSession } from "@/lib/auth/session";
import { hasPermission } from "@/lib/auth/permissions";

export default async function DepartmentsPage() {
  const session = await getCurrentSession();
  return (
    <DepartmentManagement
      canRead={hasPermission(session, "staff.read")}
      canManage={hasPermission(session, "departments.manage")}
    />
  );
}
