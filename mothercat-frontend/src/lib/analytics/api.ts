import { companyActivityMocks, myActivityMocks, pendingMocks } from "@/lib/analytics/mock";
import type {
  AnalyticsRange,
  CompanyActivityAnalytics,
  MyActivityAnalytics,
  PendingAnalytics,
} from "@/lib/analytics/types";

const mockLatencyMs = 180;

export async function getCompanyActivity(range: AnalyticsRange): Promise<CompanyActivityAnalytics> {
  // TODO: replace with the company activity analytics endpoint.
  await new Promise((resolve) => setTimeout(resolve, mockLatencyMs));
  return companyActivityMocks[range];
}

export async function getMyActivity(range: AnalyticsRange): Promise<MyActivityAnalytics> {
  // TODO: replace with the signed-in user's activity analytics endpoint.
  await new Promise((resolve) => setTimeout(resolve, mockLatencyMs));
  return myActivityMocks[range];
}

export async function getPending(range: AnalyticsRange): Promise<PendingAnalytics> {
  // TODO: replace with the pending-work analytics endpoint.
  await new Promise((resolve) => setTimeout(resolve, mockLatencyMs));
  return pendingMocks[range];
}
