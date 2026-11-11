export type AnalyticsRange = "7d" | "30d" | "90d";

export type CompanyActivityPoint = {
  date: string;
  label: string;
  departments: { name: string; value: number }[];
};

export interface DepartmentActivity {
  name: string;
  activities: number;
  tasksCompleted: number;
  completionRate: number;
  activeStaff: number;
  trend: "up" | "down" | "steady";
}

export interface CompanyActivityAnalytics {
  range: AnalyticsRange;
  totalActivities: number;
  changePercent: number;
  timeline: CompanyActivityPoint[];
  departments: DepartmentActivity[];
  activityTypes: { name: string; count: number }[];
  heatmap: { day: string; hours: number[] }[];
}

export interface MyActivityAnalytics {
  range: AnalyticsRange;
  activityScore: number;
  scoreLabel: string;
  changePercent: number;
  scoreFactors: { name: string; value: number; maximum: number; detail: string }[];
  dailyTrend: { date: string; label: string; value: number }[];
  taskBreakdown: { name: string; count: number }[];
  contributionWeeks: { week: string; days: { date: string; count: number }[] }[];
  achievements: { title: string; detail: string; date: string }[];
  departmentComparison: { user: number; departmentAverage: number };
}

export interface PendingAnalytics {
  range: AnalyticsRange;
  totalPending: number;
  byType: { name: string; count: number; href: string }[];
  aging: { label: string; count: number }[];
  departments: { name: string; count: number }[];
  topItems: { title: string; type: string; ageDays: number; href: string }[];
}
