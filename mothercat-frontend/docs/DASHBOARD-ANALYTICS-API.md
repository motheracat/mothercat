# Dashboard Analytics API Contract

This document describes the analytics data currently used by the dashboard and the backend API contract that can replace the deterministic frontend mocks. The UI reads analytics through `src/lib/analytics/hooks.ts`; when endpoints are ready, replace the mock implementations in `src/lib/analytics/api.ts` without changing dashboard components.

## Proposed endpoints

All endpoints require the existing authenticated Mothercat session cookie and are scoped by the caller's permissions:

| Endpoint | Scope | Query |
|---|---|---|
| `GET /api/v1/analytics/company-activity` | Company-wide activity and department comparison | `range=7d`, `30d`, or `90d` |
| `GET /api/v1/analytics/my-activity` | Current signed-in user's activity | `range=7d`, `30d`, or `90d` |
| `GET /api/v1/analytics/pending` | Items requiring staff/admin action | `range=7d`, `30d`, or `90d` |

The range is a required enumerated query parameter. Each response should echo the selected `range`. Use a consistent server-side timezone (prefer UTC) when bucketing dates. Return an empty array and zero totals for periods with no data; the frontend displays its empty state from those values. Invalid ranges should receive the application's standard validation response.

## TypeScript response contracts

These are the frontend types in `src/lib/analytics/types.ts`, expressed here as the expected JSON response shapes:

```ts
type AnalyticsRange = "7d" | "30d" | "90d";

interface CompanyActivityAnalytics {
  range: AnalyticsRange;
  totalActivities: number;
  changePercent: number;
  timeline: {
    date: string; // ISO 8601 date, YYYY-MM-DD
    label: string; // Display label for that date/bucket
    departments: { name: string; value: number }[];
  }[];
  departments: {
    name: string;
    activities: number;
    tasksCompleted: number;
    completionRate: number; // 0–100
    activeStaff: number;
    trend: "up" | "down" | "steady";
  }[];
  activityTypes: { name: string; count: number }[];
  heatmap: {
    day: string; // Stable weekday label, e.g. "Mon"
    hours: number[]; // Ordered activity counts for eight working-hour buckets
  }[];
}

interface MyActivityAnalytics {
  range: AnalyticsRange;
  activityScore: number; // 0–100
  scoreLabel: string;
  changePercent: number;
  scoreFactors: {
    name: string;
    value: number;
    maximum: number;
    detail: string;
  }[];
  dailyTrend: {
    date: string; // ISO 8601 date, YYYY-MM-DD
    label: string;
    value: number;
  }[];
  taskBreakdown: { name: string; count: number }[];
  contributionWeeks: {
    week: string;
    days: { date: string; count: number }[]; // Seven days, oldest first
  }[];
  achievements: {
    title: string;
    detail: string;
    date: string; // ISO 8601 date or timestamp
  }[];
  departmentComparison: {
    user: number; // Comparable activity score, 0–100
    departmentAverage: number; // Comparable activity score, 0–100
  };
}

interface PendingAnalytics {
  range: AnalyticsRange;
  totalPending: number;
  byType: {
    name: string;
    count: number;
    href: string; // Same-origin application destination
  }[];
  aging: {
    label: string; // Expected buckets: 0–1 days, 2–7 days, 8–30 days, 30+ days
    count: number;
  }[];
  departments: { name: string; count: number }[];
  topItems: {
    title: string;
    type: string;
    ageDays: number;
    href: string; // Same-origin application destination
  }[];
}
```

## Metric definitions

- **Activity**: count auditable user and system actions in the selected interval. Keep the event inclusion rules stable and document exclusions (for example, health checks or background retries).
- **Change percent**: percent change from the immediately preceding interval of equal duration. For a zero previous-period count, define and document a consistent policy (for example, return `0` when both periods are zero and `100` when activity begins).
- **Department completion rate**: completed tasks divided by tasks due or otherwise eligible in the selected period, multiplied by 100. Return `0` when the denominator is zero.
- **Active staff**: unique staff accounts with at least one qualifying activity in the interval.
- **Personal activity score**: normalized composite of completed tasks, on-time/late work, qualifying activities, active days, and response time. Keep weights and normalization server-side and publish them so the score remains explainable.
- **Department comparison**: compute the user score and department average using the same scoring formula and period.
- **Pending**: current unresolved actionable records, grouped by type and department. Aging is based on time since the record became actionable; aging buckets must sum to the applicable pending total.
- **`topItems.href` and `byType.href`**: return only allowlisted same-origin paths. Do not return executable or arbitrary external URLs.

## Mock data and frontend integration

`src/lib/analytics/mock.ts` contains deterministic samples for all three ranges. It uses the department names supplied for the dashboard: Building & Structural Construction, Civil Engineering, Finance & Accounts, HSE, and Human Resources & Administration. These are illustrative, not production metrics. Keep real metric calculations in the backend once endpoints exist.

The three API functions (`getCompanyActivity`, `getMyActivity`, and `getPending`) are the replacement seam. They currently delay briefly and return mock responses. Preserve their signatures and return the validated response types when connecting `request()` to the proposed endpoints. The corresponding hooks provide `{ data, isLoading, error, retry }`.

## Operational and security notes

- Enforce authorization independently for each analytics scope; company-wide and pending data may require elevated permissions.
- Aggregate server-side and avoid exposing other users' identifying activity in the personal response.
- Avoid returning IP addresses, user agents, raw audit payloads, or sensitive staff data in these aggregate responses.
- Use bounded date ranges, stable pagination/size limits for `topItems`, and appropriate caching that does not leak one account's personal analytics to another.
- Treat pending-item links as navigation hints only; destination endpoints must still enforce authorization.
