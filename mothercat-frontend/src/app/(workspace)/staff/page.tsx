import { StaffManagement } from "@/components/workspace/staff-management";
import { getCurrentSession } from "@/lib/auth/session";
import { hasPermission } from "@/lib/auth/permissions";

export default async function StaffPage() {
  const session = await getCurrentSession();
  return (
    <StaffManagement
      permissions={{
        canRead: hasPermission(session, "staff.read"),
        canCreate: hasPermission(session, "staff.create"),
        canUpdate: hasPermission(session, "staff.update"),
        canDeactivate: hasPermission(session, "staff.deactivate"),
        canImport: hasPermission(session, "staff.import"),
        canProvision: hasPermission(session, "user_roles.assign"),
      }}
    />
  );
}
