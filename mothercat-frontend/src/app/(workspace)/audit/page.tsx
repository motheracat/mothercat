import { AuditLog } from "@/components/workspace/audit-log";
import { getCurrentSession } from "@/lib/auth/session";
import { isDevOpen } from "@/lib/auth/mode";
import { hasPermission } from "@/lib/auth/permissions";

export default async function AuditPage() {
  const session = await getCurrentSession();
  const canRead = hasPermission(session, "audit.read");
  return <AuditLog canRead={canRead} developmentOpen={isDevOpen()} />;
}
