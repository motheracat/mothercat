import { AuditEventDetail } from "@/components/workspace/audit-event-detail";
import { isDevOpen } from "@/lib/auth/mode";
import { getCurrentSession } from "@/lib/auth/session";
import { hasPermission } from "@/lib/auth/permissions";

export default async function AuditEventPage({ params }: { params: Promise<{ id: string }> }) {
  const [{ id }, session] = await Promise.all([params, getCurrentSession()]);
  return (
    <AuditEventDetail
      eventId={id}
      canRead={hasPermission(session, "audit.read")}
      developmentOpen={isDevOpen()}
    />
  );
}
