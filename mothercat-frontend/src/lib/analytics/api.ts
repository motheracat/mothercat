import { listAuditEvents, type AuditEvent } from "@/lib/audit/api";
import { listDepartments, listStaff, type Department, type StaffMember } from "@/lib/organization/api";
import { listTasks, type Task } from "@/lib/tasks/api";
import type {
  AnalyticsRange,
  CompanyActivityAnalytics,
  MyActivityAnalytics,
  PendingAnalytics,
} from "@/lib/analytics/types";

const pageSize = 100;
const dayMs = 86_400_000;
const terminalTaskStatuses = new Set(["approved", "rejected", "cancelled"]);
const ongoingTaskStatuses = new Set(["draft", "assigned", "in_progress", "changes_requested"]);

type DateWindow = {
  currentStart: Date;
  previousStart: Date;
  endExclusive: Date;
};

function getDateWindow(range: AnalyticsRange, now = new Date()): DateWindow {
  const days = range === "7d" ? 7 : range === "30d" ? 30 : 90;
  const endExclusive = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() + 1));
  const currentStart = new Date(endExclusive.getTime() - days * dayMs);
  return {
    currentStart,
    previousStart: new Date(currentStart.getTime() - days * dayMs),
    endExclusive,
  };
}

function toDate(value: Date): string {
  return value.toISOString().slice(0, 10);
}

function dateLabel(date: string): string {
  return new Intl.DateTimeFormat("en", { month: "short", day: "numeric", timeZone: "UTC" })
    .format(new Date(`${date}T00:00:00Z`));
}

async function loadAllPages<T>(
  load: (limit: number, offset: number) => Promise<{ items: T[]; total: number }>,
): Promise<T[]> {
  const firstPage = await load(pageSize, 0);
  const items = [...firstPage.items];
  for (let offset = firstPage.items.length; offset < firstPage.total; offset += pageSize) {
    if (offset > 1_000_000) {
      throw new Error("The analytics source returned more records than can be paged safely.");
    }
    const page = await load(pageSize, offset);
    if (page.items.length === 0) {
      throw new Error("The analytics source returned an incomplete record list.");
    }
    items.push(...page.items);
  }
  return items;
}

async function loadAuditEvents(from: Date, to: Date): Promise<AuditEvent[]> {
  const toInclusive = new Date(to.getTime() - 1);
  return loadAllPages((limit, offset) =>
    listAuditEvents({ from: from.toISOString(), to: toInclusive.toISOString(), limit, offset })
      .then((page) => ({ items: page.events, total: page.total })),
  );
}

async function loadTasks(mine = false): Promise<Task[]> {
  return loadAllPages((limit, offset) =>
    listTasks({ mine, limit, offset })
      .then((page) => ({ items: page.tasks, total: page.page.total })),
  );
}

async function loadStaff(filter: { employmentStatus?: "pending" } = {}): Promise<StaffMember[]> {
  return loadAllPages((limit, offset) =>
    listStaff({ limit, offset, ...filter })
      .then((page) => ({ items: page.staff, total: page.total })),
  );
}

function countChange(current: number, previous: number): number | null {
  if (previous === 0) return current === 0 ? 0 : null;
  return Math.round(((current - previous) / previous) * 100);
}

function eventsInWindow(events: AuditEvent[], start: Date, endExclusive: Date): AuditEvent[] {
  return events.filter((event) => {
    const time = Date.parse(event.occurred_at);
    return Number.isFinite(time) && time >= start.getTime() && time < endExclusive.getTime();
  });
}

function activityName(action: string): string {
  const lastPart = action.split(".").at(-1) ?? action;
  return lastPart.replace(/[_-]+/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function isTaskCompletion(event: AuditEvent): boolean {
  return event.resource_type.toLowerCase().includes("task") &&
    /(^|[._-])(approved|completed)([._-]|$)/i.test(event.action);
}

export async function getCompanyActivity(range: AnalyticsRange): Promise<CompanyActivityAnalytics> {
  const window = getDateWindow(range);
  const [events, departments, staff] = await Promise.all([
    loadAuditEvents(window.previousStart, window.endExclusive),
    listDepartments(),
    loadStaff(),
  ]);
  const currentEvents = eventsInWindow(events, window.currentStart, window.endExclusive);
  const previousEvents = eventsInWindow(events, window.previousStart, window.currentStart);
  const departmentById = new Map(departments.map((department) => [department.id, department.name]));
  const staffByAccount = new Map<string, StaffMember>();
  for (const member of staff) {
    if (member.account_id) staffByAccount.set(member.account_id, member);
  }
  const departmentNames = new Set(departments.map((department) => department.name));
  const eventDepartment = (event: AuditEvent): string => {
    const staffMember = staffByAccount.get(event.actor_id);
    const name = staffMember?.department_id ? departmentById.get(staffMember.department_id) : undefined;
    return name ?? "Unassigned / unknown";
  };
  const currentByDepartment = new Map<string, AuditEvent[]>();
  const previousByDepartment = new Map<string, number>();
  for (const department of departments) currentByDepartment.set(department.name, []);
  for (const event of previousEvents) {
    const name = eventDepartment(event);
    previousByDepartment.set(name, (previousByDepartment.get(name) ?? 0) + 1);
  }
  const dayDepartmentCounts = new Map<string, Map<string, number>>();
  for (const event of currentEvents) {
    const name = eventDepartment(event);
    departmentNames.add(name);
    const departmentEvents = currentByDepartment.get(name) ?? [];
    departmentEvents.push(event);
    currentByDepartment.set(name, departmentEvents);
    const timestamp = Date.parse(event.occurred_at);
    const date = toDate(new Date(timestamp));
    const dayCounts = dayDepartmentCounts.get(date) ?? new Map<string, number>();
    dayCounts.set(name, (dayCounts.get(name) ?? 0) + 1);
    dayDepartmentCounts.set(date, dayCounts);
  }

  const dayCount = Math.round((window.endExclusive.getTime() - window.currentStart.getTime()) / dayMs);
  const timeline = Array.from({ length: dayCount }, (_, index) => {
    const date = toDate(new Date(window.currentStart.getTime() + index * dayMs));
    const dayCounts = dayDepartmentCounts.get(date);
    return {
      date,
      label: dateLabel(date),
      departments: [...departmentNames].map((name) => ({
        name,
        value: dayCounts?.get(name) ?? 0,
      })),
    };
  });

  const departmentActivity = [...departmentNames].map((name) => {
    const departmentEvents = currentByDepartment.get(name) ?? [];
    const completed = departmentEvents.filter(isTaskCompletion).length;
    const previousCount = previousByDepartment.get(name) ?? 0;
    const activities = departmentEvents.length;
    return {
      name,
      activities,
      tasksCompleted: completed,
      activeStaff: new Set(departmentEvents
        .filter((event) => event.actor_type !== "system" && event.actor_id !== "")
        .map((event) => event.actor_id)).size,
      trend: activities > previousCount ? "up" as const : activities < previousCount ? "down" as const : "steady" as const,
    };
  }).sort((a, b) => b.activities - a.activities || a.name.localeCompare(b.name));

  const activityCounts = new Map<string, number>();
  for (const event of currentEvents) {
    const name = activityName(event.action);
    activityCounts.set(name, (activityCounts.get(name) ?? 0) + 1);
  }

  const heatmapCounts = Array.from({ length: 7 }, () => Array<number>(8).fill(0));
  for (const event of currentEvents) {
    const timestamp = Date.parse(event.occurred_at);
    if (!Number.isFinite(timestamp)) continue;
    const date = new Date(timestamp);
    const hourBucket = Math.floor(date.getUTCHours() / 3);
    heatmapCounts[date.getUTCDay()][hourBucket] += 1;
  }

  return {
    range,
    totalActivities: currentEvents.length,
    changePercent: countChange(currentEvents.length, previousEvents.length),
    timeline,
    departments: departmentActivity,
    activityTypes: [...activityCounts].map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count),
    heatmap: ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].map((day, index) => ({
      day,
      hours: heatmapCounts[index],
    })),
  };
}

function isCompleted(task: Task): boolean {
  return task.status === "approved";
}

function isOpen(task: Task): boolean {
  return !terminalTaskStatuses.has(task.status);
}

function taskUpdateTime(task: Task): number {
  const timestamp = Date.parse(task.updated_at);
  return Number.isFinite(timestamp) ? timestamp : Number.NaN;
}

export async function getMyActivity(range: AnalyticsRange): Promise<MyActivityAnalytics> {
  const window = getDateWindow(range);
  const tasks = await loadTasks(true);
  const currentTasks = tasks.filter((task) => {
    const time = taskUpdateTime(task);
    return time >= window.currentStart.getTime() && time < window.endExclusive.getTime();
  });
  const previousTasks = tasks.filter((task) => {
    const time = taskUpdateTime(task);
    return time >= window.previousStart.getTime() && time < window.currentStart.getTime();
  });
  const completed = currentTasks.filter(isCompleted).length;
  const activeDays = new Set(currentTasks.map((task) => task.updated_at.slice(0, 10))).size;
  const activityScore = currentTasks.length === 0
    ? 0
    : Math.round((completed / currentTasks.length) * 60 + (activeDays / (range === "7d" ? 7 : range === "30d" ? 30 : 90)) * 40);
  const scoreLabel = activityScore >= 80 ? "Highly active" : activityScore >= 50 ? "Active" : activityScore > 0 ? "Getting started" : "No activity yet";
  const rangeDays = range === "7d" ? 7 : range === "30d" ? 30 : 90;
  const today = new Date();
  const todayStart = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate()));
  const contributionStart = new Date(todayStart.getTime() - 83 * dayMs);
  const activityStart = new Date(todayStart.getTime() - (Math.max(rangeDays, 84) - 1) * dayMs);
  const activityByDate = new Map<string, number>();
  for (const task of tasks) {
    const timestamp = taskUpdateTime(task);
    if (!Number.isFinite(timestamp) || timestamp < activityStart.getTime() || timestamp >= todayStart.getTime() + dayMs) continue;
    const date = toDate(new Date(timestamp));
    activityByDate.set(date, (activityByDate.get(date) ?? 0) + 1);
  }

  const dailyTrend = Array.from({ length: rangeDays }, (_, index) => {
    const date = toDate(new Date(window.currentStart.getTime() + index * dayMs));
    return { date, label: dateLabel(date), value: activityByDate.get(date) ?? 0 };
  });
  const contributionWeeks = Array.from({ length: 12 }, (_, weekIndex) => {
    const days = Array.from({ length: 7 }, (_, dayIndex) => {
      const date = toDate(new Date(contributionStart.getTime() + (weekIndex * 7 + dayIndex) * dayMs));
      return { date, count: activityByDate.get(date) ?? 0 };
    });
    return { week: `Week of ${dateLabel(days[0].date)}`, days };
  });
  const taskBreakdown = [
    { name: "Completed", count: currentTasks.filter(isCompleted).length },
    { name: "Awaiting review", count: currentTasks.filter((task) => task.status === "submitted").length },
    { name: "Overdue", count: currentTasks.filter((task) => isOpen(task) && task.status !== "submitted" && task.due_at !== null && Date.parse(task.due_at) < Date.now()).length },
    { name: "In progress", count: currentTasks.filter((task) => ongoingTaskStatuses.has(task.status) && !(task.due_at && Date.parse(task.due_at) < Date.now())).length },
    { name: "Other", count: currentTasks.filter((task) =>
      !isCompleted(task) &&
      task.status !== "submitted" &&
      !(isOpen(task) && task.due_at !== null && Date.parse(task.due_at) < Date.now()) &&
      !ongoingTaskStatuses.has(task.status),
    ).length },
  ];
  const maximum = Math.max(1, currentTasks.length);

  return {
    range,
    activityScore,
    scoreLabel,
    changePercent: countChange(currentTasks.length, previousTasks.length),
    scoreFactors: [
      { name: "Tasks completed", value: completed, maximum, detail: `${completed} approved tasks last updated in this period` },
      { name: "Active days", value: activeDays, maximum: rangeDays, detail: `${activeDays} days with task updates` },
    ],
    dailyTrend,
    taskBreakdown,
    contributionWeeks,
    achievements: [
      ...(completed > 0 ? [{
        title: `${completed} approved tasks updated`,
        detail: "These task records currently have an approved status and were updated in this period.",
        date: toDate(new Date(Math.max(...currentTasks.filter(isCompleted).map((task) => taskUpdateTime(task))))),
      }] : []),
      ...(activeDays > 0 ? [{
        title: `${activeDays} active days`,
        detail: "You updated tasks on these days during the selected period.",
        date: toDate(new Date(Math.max(...currentTasks.map((task) => taskUpdateTime(task))))),
      }] : []),
    ],
  };
}

function ageInDays(createdAt: string, now: number): number | null {
  const created = Date.parse(createdAt);
  return Number.isFinite(created) ? Math.max(0, Math.floor((now - created) / dayMs)) : null;
}

export async function getPending(): Promise<PendingAnalytics> {
  const now = Date.now();
  const [tasks, pendingStaff, departments] = await Promise.all([
    loadTasks(),
    loadStaff({ employmentStatus: "pending" }),
    listDepartments(),
  ]);
  const awaitingReview = tasks.filter((task) => task.status === "submitted");
  const overdue = tasks.filter((task) =>
    isOpen(task) &&
    task.status !== "submitted" &&
    task.due_at !== null &&
    Date.parse(task.due_at) < now,
  );
  const departmentCounts = new Map(departments.map((department) => [department.id, { name: department.name, count: 0 }]));
  const unassignedDepartment = { name: "Unassigned / unknown", count: 0 };
  for (const staffMember of pendingStaff) {
    const item = staffMember.department_id ? departmentCounts.get(staffMember.department_id) : undefined;
    if (item) item.count += 1;
    else unassignedDepartment.count += 1;
  }
  for (const task of [...awaitingReview, ...overdue]) {
    const item = departmentCounts.get(task.department_id);
    if (item) item.count += 1;
    else unassignedDepartment.count += 1;
  }
  const taskItems = [
    ...awaitingReview.map((task) => ({
      id: task.id,
      title: task.title,
      type: "Task awaiting review",
      ageDays: ageInDays(task.latest_submission?.submitted_at ?? task.updated_at, now),
      href: "/tasks",
    })),
    ...overdue.map((task) => ({
      id: task.id,
      title: task.title,
      type: "Overdue task",
      ageDays: task.due_at ? ageInDays(task.due_at, now) : null,
      href: "/tasks",
    })),
  ];
  const staffItems = pendingStaff.map((member) => ({
    id: member.id,
    title: member.full_name,
    type: "Staff awaiting approval",
    ageDays: null,
    href: "/staff",
  }));
  const pendingItems = [...taskItems, ...staffItems];
  const agingCounts = new Map([["0–1 days", 0], ["2–7 days", 0], ["8–30 days", 0], ["30+ days", 0], ["Age unavailable", 0]]);
  for (const item of pendingItems) {
    if (item.ageDays === null) {
      agingCounts.set("Age unavailable", (agingCounts.get("Age unavailable") ?? 0) + 1);
    } else {
      const label = item.ageDays <= 1 ? "0–1 days" : item.ageDays <= 7 ? "2–7 days" : item.ageDays <= 30 ? "8–30 days" : "30+ days";
      agingCounts.set(label, (agingCounts.get(label) ?? 0) + 1);
    }
  }
  const byType = [
    { name: "Staff awaiting approval", count: pendingStaff.length, href: "/staff" },
    { name: "Tasks awaiting review", count: awaitingReview.length, href: "/tasks" },
    { name: "Overdue tasks", count: overdue.length, href: "/tasks" },
  ];
  if (unassignedDepartment.count > 0) departmentCounts.set("unassigned", unassignedDepartment);

  return {
    totalPending: pendingItems.length,
    byType,
    aging: [...agingCounts].map(([label, count]) => ({ label, count })),
    departments: [...departmentCounts.values()].sort((a, b) => b.count - a.count || a.name.localeCompare(b.name)),
    topItems: pendingItems
      .sort((a, b) => (b.ageDays ?? -1) - (a.ageDays ?? -1))
      .slice(0, 5),
  };
}
