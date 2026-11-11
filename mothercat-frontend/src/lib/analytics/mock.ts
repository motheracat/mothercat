import type {
  AnalyticsRange,
  CompanyActivityAnalytics,
  DepartmentActivity,
  MyActivityAnalytics,
  PendingAnalytics,
} from "@/lib/analytics/types";

const departments = [
  "Building & Structural Construction",
  "Civil Engineering",
  "Finance & Accounts",
  "HSE",
  "Human Resources & Administration",
];

const rangeDays: Record<AnalyticsRange, number> = { "7d": 7, "30d": 30, "90d": 90 };
const anchorUtc = Date.UTC(2026, 10, 10);
const activityTypeNames = [
  "Tasks created",
  "Tasks completed",
  "Tasks cancelled",
  "Status updates",
  "Logins",
  "Role changes",
  "Staff changes",
  "Imports",
];
const activityTypeWeights = [0.12, 0.14, 0.02, 0.2, 0.4, 0.02, 0.07, 0.03];
const activityBucketWeights = [0.78, 1.02, 0.94, 1.16, 0.98, 0.61, 1.27];

const companyPeriods: Record<AnalyticsRange, {
  activities: number[];
  tasksCompleted: number[];
  completionRates: number[];
  activeStaff: number[];
  trends: DepartmentActivity["trend"][];
  totalActivities: number;
  changePercent: number;
}> = {
  "7d": {
    activities: [48, 42, 35, 29, 24],
    tasksCompleted: [17, 14, 12, 10, 8],
    completionRates: [82, 76, 91, 73, 88],
    activeStaff: [12, 10, 7, 6, 8],
    trends: ["up", "up", "steady", "down", "up"],
    totalActivities: 178,
    changePercent: 8,
  },
  "30d": {
    activities: [205, 178, 143, 122, 98],
    tasksCompleted: [65, 57, 51, 43, 35],
    completionRates: [84, 79, 87, 75, 86],
    activeStaff: [18, 14, 9, 8, 11],
    trends: ["up", "steady", "down", "up", "up"],
    totalActivities: 746,
    changePercent: 11,
  },
  "90d": {
    activities: [560, 485, 412, 358, 287],
    tasksCompleted: [192, 163, 148, 122, 98],
    completionRates: [81, 78, 89, 77, 85],
    activeStaff: [22, 18, 11, 10, 14],
    trends: ["steady", "up", "down", "up", "steady"],
    totalActivities: 2102,
    changePercent: 7,
  },
};

const personalPeriods: Record<AnalyticsRange, {
  activityScore: number;
  scoreLabel: string;
  changePercent: number;
  completed: number;
  completedOnTime: number;
  completedLate: number;
  activities: number;
  activeDays: number;
  responseScore: number;
  averageResponse: string;
  inProgress: number;
  overdue: number;
  cancelled: number;
  achievementPeriod: string;
}> = {
  "7d": {
    activityScore: 78,
    scoreLabel: "Highly active",
    changePercent: 8,
    completed: 4,
    completedOnTime: 3,
    completedLate: 1,
    activities: 17,
    activeDays: 5,
    responseScore: 4,
    averageResponse: "2.4 working hours",
    inProgress: 2,
    overdue: 1,
    cancelled: 0,
    achievementPeriod: "this week",
  },
  "30d": {
    activityScore: 82,
    scoreLabel: "Highly active",
    changePercent: 6,
    completed: 15,
    completedOnTime: 13,
    completedLate: 2,
    activities: 68,
    activeDays: 18,
    responseScore: 4,
    averageResponse: "2.9 working hours",
    inProgress: 5,
    overdue: 2,
    cancelled: 2,
    achievementPeriod: "this month",
  },
  "90d": {
    activityScore: 80,
    scoreLabel: "Consistently active",
    changePercent: 11,
    completed: 41,
    completedOnTime: 36,
    completedLate: 5,
    activities: 182,
    activeDays: 56,
    responseScore: 4,
    averageResponse: "3.8 working hours",
    inProgress: 8,
    overdue: 3,
    cancelled: 5,
    achievementPeriod: "this quarter",
  },
};

const pendingPeriods: Record<AnalyticsRange, {
  byType: number[];
  aging: number[];
  departments: number[];
  itemAges: number[];
}> = {
  "7d": {
    byType: [2, 1, 1, 3, 2],
    aging: [4, 5, 0, 0],
    departments: [3, 2, 1, 2, 1],
    itemAges: [6, 5, 4, 2, 1],
  },
  "30d": {
    byType: [3, 1, 2, 4, 2],
    aging: [4, 5, 2, 1],
    departments: [4, 3, 2, 2, 1],
    itemAges: [26, 18, 12, 6, 3],
  },
  "90d": {
    byType: [4, 2, 3, 6, 3],
    aging: [5, 7, 4, 2],
    departments: [6, 4, 3, 3, 2],
    itemAges: [68, 42, 31, 17, 9],
  },
};

function allocateCounts(total: number, weights: number[]) {
  const weightTotal = weights.reduce((sum, weight) => sum + weight, 0);
  const exactCounts = weights.map((weight) => (total * weight) / weightTotal);
  const counts = exactCounts.map(Math.floor);
  const remainder = total - counts.reduce((sum, count) => sum + count, 0);
  const remainderOrder = exactCounts
    .map((count, index) => ({ index, fraction: count - Math.floor(count) }))
    .sort((a, b) => b.fraction - a.fraction || a.index - b.index);

  for (let index = 0; index < remainder; index += 1) {
    counts[remainderOrder[index].index] += 1;
  }

  return counts;
}

function isoDaysAgo(daysAgo: number) {
  return new Date(anchorUtc - daysAgo * 86_400_000).toISOString().slice(0, 10);
}

function formatDateLabel(date: string) {
  return new Intl.DateTimeFormat("en", { month: "short", day: "numeric", timeZone: "UTC" })
    .format(new Date(`${date}T00:00:00Z`));
}

function makeCompanyActivity(range: AnalyticsRange): CompanyActivityAnalytics {
  const period = companyPeriods[range];
  const departmentsData = departments.map((name, index) => ({
    name,
    activities: period.activities[index],
    tasksCompleted: period.tasksCompleted[index],
    completionRate: period.completionRates[index],
    activeStaff: period.activeStaff[index],
    trend: period.trends[index],
  }));
  const timeline = activityBucketWeights.map((_, index) => {
    const date = isoDaysAgo(Math.round((6 - index) * (rangeDays[range] / 7)));
    return {
      date,
      label: formatDateLabel(date),
      departments: departmentsData.map((department) => ({
        name: department.name,
        value: allocateCounts(department.activities, activityBucketWeights)[index],
      })),
    };
  });
  const dayWeights = [0.55, 1.1, 1.18, 1.14, 1.05, 0.58, 0.4];
  const hourWeights = [0.55, 0.8, 1.1, 1.22, 1.18, 0.9, 0.5, 0.25];
  const heatmapCounts = allocateCounts(
    period.totalActivities,
    dayWeights.flatMap((dayWeight) => hourWeights.map((hourWeight) => dayWeight * hourWeight)),
  );

  return {
    range,
    totalActivities: period.totalActivities,
    changePercent: period.changePercent,
    timeline,
    departments: departmentsData,
    activityTypes: activityTypeNames.map((name, index) => ({
      name,
      count: allocateCounts(period.totalActivities, activityTypeWeights)[index],
    })),
    heatmap: ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"].map((day, dayIndex) => ({
      day,
      hours: heatmapCounts.slice(dayIndex * 8, dayIndex * 8 + 8),
    })),
  };
}

function makePersonalTimeline(days: number, totalActivities: number) {
  const weeklyWeights = [0.56, 1.08, 1.16, 1.12, 1.02, 0.93, 0.43];
  const dailyWeights = Array.from({ length: days }, (_, index) => {
    const date = isoDaysAgo(days - index - 1);
    const weekday = new Date(`${date}T00:00:00Z`).getUTCDay();
    const weekdayWeight = weeklyWeights[weekday];
    const variation = [0.94, 1.02, 1.12, 1.06, 0.98, 1.15, 0.89][index % 7];
    return weekdayWeight * variation;
  });
  const dailyCounts = allocateCounts(totalActivities, dailyWeights);

  return dailyCounts.map((value, index) => {
    const date = isoDaysAgo(days - index - 1);
    return { date, label: formatDateLabel(date), value };
  });
}

function makeMyActivity(range: AnalyticsRange): MyActivityAnalytics {
  const period = personalPeriods[range];
  const longTrend = makePersonalTimeline(90, personalPeriods["90d"].activities);
  const contributionDays = longTrend.slice(-84);
  const taskBreakdown = [
    { name: "Completed", count: period.completed },
    { name: "In progress", count: period.inProgress },
    { name: "Overdue", count: period.overdue },
    { name: "Cancelled", count: period.cancelled },
  ];

  return {
    range,
    activityScore: period.activityScore,
    scoreLabel: period.scoreLabel,
    changePercent: period.changePercent,
    scoreFactors: [
      { name: "Tasks completed", value: period.completed, maximum: Math.max(period.completed, range === "7d" ? 6 : range === "30d" ? 20 : 54), detail: `${period.completed} completed ${period.achievementPeriod}` },
      { name: "Completed on time", value: period.completedOnTime, maximum: period.completed, detail: `${period.completedOnTime} of ${period.completed} met their due date` },
      { name: "Completed late", value: period.completedLate, maximum: period.completed, detail: `${period.completedLate} missed the due date` },
      { name: "Activities performed", value: period.activities, maximum: range === "7d" ? 22 : range === "30d" ? 90 : 240, detail: "Comments, updates, and other account actions" },
      { name: "Active days", value: period.activeDays, maximum: rangeDays[range], detail: `${period.activeDays} days with activity` },
      { name: "Average response time", value: period.responseScore, maximum: 5, detail: `First response averaged ${period.averageResponse}` },
    ],
    dailyTrend: makePersonalTimeline(rangeDays[range], period.activities),
    taskBreakdown,
    contributionWeeks: Array.from({ length: 12 }, (_, weekIndex) => {
      const days = contributionDays.slice(weekIndex * 7, weekIndex * 7 + 7);
      return {
        week: `Week of ${days[0].label}`,
        days: days.map(({ date, value }) => ({ date, count: value })),
      };
    }),
    achievements: [
      {
        title: `${period.completed} tasks completed ${period.achievementPeriod}`,
        detail: `${period.completedOnTime} were delivered by their due dates.`,
        date: isoDaysAgo(1),
      },
      {
        title: `${period.activeDays} active days`,
        detail: "You contributed regularly during the selected period.",
        date: isoDaysAgo(3),
      },
      {
        title: "Timely team responses",
        detail: `Your average first response was ${period.averageResponse}.`,
        date: isoDaysAgo(6),
      },
    ],
    departmentComparison: {
      user: period.activityScore,
      departmentAverage: range === "7d" ? 71 : range === "30d" ? 74 : 72,
    },
  };
}

function makePending(range: AnalyticsRange): PendingAnalytics {
  const period = pendingPeriods[range];
  const byTypeNames = [
    { name: "Staff awaiting approval", href: "/staff" },
    { name: "Pending staff imports", href: "/staff/imports" },
    { name: "Role changes", href: "/roles" },
    { name: "Tasks awaiting review", href: "/tasks" },
    { name: "Overdue tasks", href: "/tasks" },
  ];
  const titles = [
    "Review submitted project milestones",
    "Approve new staff records",
    "Resolve overdue site safety task",
    "Review quarterly staff import",
    "Confirm department role update",
  ];
  const types = ["Task review", "Staff approval", "Overdue task", "Staff import", "Role change"];
  const hrefs = ["/tasks", "/staff", "/tasks", "/staff/imports", "/roles"];
  const totalPending = period.byType.reduce((sum, count) => sum + count, 0);

  return {
    range,
    totalPending,
    byType: byTypeNames.map((item, index) => ({ ...item, count: period.byType[index] })),
    aging: ["0–1 days", "2–7 days", "8–30 days", "30+ days"].map((label, index) => ({
      label,
      count: period.aging[index],
    })),
    departments: departments.map((name, index) => ({ name, count: period.departments[index] })),
    topItems: titles.map((title, index) => ({
      title,
      type: types[index],
      ageDays: period.itemAges[index],
      href: hrefs[index],
    })),
  };
}

export const companyActivityMocks: Record<AnalyticsRange, CompanyActivityAnalytics> = {
  "7d": makeCompanyActivity("7d"),
  "30d": makeCompanyActivity("30d"),
  "90d": makeCompanyActivity("90d"),
};

export const myActivityMocks: Record<AnalyticsRange, MyActivityAnalytics> = {
  "7d": makeMyActivity("7d"),
  "30d": makeMyActivity("30d"),
  "90d": makeMyActivity("90d"),
};

export const pendingMocks: Record<AnalyticsRange, PendingAnalytics> = {
  "7d": makePending("7d"),
  "30d": makePending("30d"),
  "90d": makePending("90d"),
};
