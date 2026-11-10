import { OpportunityManagement } from "@/components/workspace/opportunity-management";
import { getCurrentSession } from "@/lib/auth/session";
import { hasAnyPermission, hasPermission } from "@/lib/auth/permissions";

export default async function OpportunityManagementPage() {
  const session = await getCurrentSession();
  return (
    <OpportunityManagement
      canPublish={hasAnyPermission(session, ["opportunities.publish", "roles.manage"])}
      canReviewApplications={hasPermission(session, "applications.review")}
      canReviewSiwes={hasPermission(session, "it_applications.review")}
    />
  );
}
