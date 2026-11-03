import { StaffProfile } from "@/components/workspace/staff-profile";
import { hasPermission } from "@/lib/auth/permissions";
import { getCurrentSession } from "@/lib/auth/session";

export default async function StaffProfilePage({ params }: { params: Promise<{ staffId: string }> }) {
  const [{ staffId }, session] = await Promise.all([params, getCurrentSession()]);
  return (
    <StaffProfile
      staffId={staffId}
      permissions={{
        canRead: hasPermission(session, "staff.read"),
        canUpdate: hasPermission(session, "staff.update"),
        canDeactivate: hasPermission(session, "staff.deactivate"),
        canProvision: hasPermission(session, "user_roles.assign"),
        canProvisionAdmin: hasPermission(session, "roles.manage"),
      }}
    />
  );
}
