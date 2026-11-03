import { AccessManagement } from "@/components/workspace/access-management";
import { getCurrentSession } from "@/lib/auth/session";
import { isDevOpen } from "@/lib/auth/mode";
import { hasPermission } from "@/lib/auth/permissions";

export default async function RolesPage() {
  const session = await getCurrentSession();
  const devOpen = isDevOpen();
  return (
    <AccessManagement
      canRead={!devOpen && hasPermission(session, "roles.read")}
      canManage={!devOpen && hasPermission(session, "roles.manage")}
      canAssign={!devOpen && hasPermission(session, "user_roles.assign")}
      permissions={devOpen ? [] : session?.permissions ?? []}
      currentUserId={devOpen ? "" : session?.userId ?? ""}
    />
  );
}
