import { OpportunityApplication } from "@/components/public/opportunity-application";

export default async function OpportunityApplicationPage({ params }: { params: Promise<{ opportunityId: string }> }) {
  const { opportunityId } = await params;
  return <OpportunityApplication opportunityId={opportunityId} />;
}
