import { OpportunityManagement } from "@/components/workspace/opportunity-management";
import { getCurrentSession } from "@/lib/auth/session";
import { hasPermission } from "@/lib/auth/permissions";

export default async function OpportunityManagementPage() {
  const session = await getCurrentSession();
  return (
    <OpportunityManagement
      canPublish={hasPermission(session, "opportunities.publish")}
      canReviewApplications={hasPermission(session, "applications.review")}
      canReviewSiwes={hasPermission(session, "it_applications.review")}
    />
  );
}
