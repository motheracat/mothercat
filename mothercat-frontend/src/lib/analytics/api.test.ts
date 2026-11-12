import { beforeEach, describe, expect, it, vi } from "vitest";

const sourceMocks = vi.hoisted(() => ({
  listAuditEvents: vi.fn(),
  listDepartments: vi.fn(),
  listStaff: vi.fn(),
  listTasks: vi.fn(),
}));

vi.mock("@/lib/audit/api", () => ({ listAuditEvents: sourceMocks.listAuditEvents }));
vi.mock("@/lib/organization/api", () => ({
  listDepartments: sourceMocks.listDepartments,
  listStaff: sourceMocks.listStaff,
}));
vi.mock("@/lib/tasks/api", () => ({ listTasks: sourceMocks.listTasks }));

import type { AuditEvent } from "@/lib/audit/api";
import type { Department, StaffMember } from "@/lib/organization/api";
import type { Task } from "@/lib/tasks/api";
import { getCompanyActivity, getMyActivity, getPending } from "@/lib/analytics/api";

function dayOffset(offset: number): string {
  const now = new Date();
  return new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() - offset, 12)).toISOString();
}

function auditEvent(id: string, action: string, occurredAt: string): AuditEvent {
  return {
    id,
    actor_type: "user",
    actor_id: "account-1",
    action,
    resource_type: "task",
    resource_id: id,
    occurred_at: occurredAt,
    details: null,
  };
}

function task(
  id: string,
  status: Task["status"],
  updatedAt: string,
  dueAt: string | null = null,
  departmentId = "department-1",
): Task {
  return {
    id,
    title: `Task ${id}`,
    description: "",
    priority: "normal",
    status,
    admin_status: "open",
    admin_status_updated_by: null,
    admin_status_updated_at: null,
    created_by: "account-1",
    department_id: departmentId,
    assignee_account_id: "account-1",
    due_at: dueAt,
    version: 1,
    created_at: dayOffset(10),
    updated_at: updatedAt,
    latest_submission: null,
  };
}

const departments: Department[] = [{ id: "department-1", name: "Construction", description: null }];
const staff: StaffMember[] = [
  {
    id: "staff-1",
    account_id: "account-1",
    staff_id: "M-1",
    department_id: "department-1",
    full_name: "Test User",
    profile_image_url: null,
    work_email: null,
    job_title: null,
    employment_status: "active",
  },
];

function mockTaskList(tasks: Task[]) {
  sourceMocks.listTasks.mockResolvedValue({
    tasks,
    page: { limit: 100, offset: 0, total: tasks.length },
  });
}

beforeEach(() => {
  vi.resetAllMocks();
  sourceMocks.listDepartments.mockResolvedValue(departments);
  sourceMocks.listStaff.mockResolvedValue({
    staff,
    total: staff.length,
    pagination: { limit: 100, offset: 0, total: staff.length },
  });
  sourceMocks.listAuditEvents.mockResolvedValue({
    events: [],
    total: 0,
    limit: 100,
    offset: 0,
  });
  mockTaskList([]);
});

describe("live dashboard analytics", () => {
  it("aggregates current and previous audit events into company activity", async () => {
    const events = [
      auditEvent("current", "task.approved", dayOffset(1)),
      auditEvent("previous", "task.created", dayOffset(8)),
    ];
    sourceMocks.listAuditEvents.mockResolvedValue({
      events,
      total: events.length,
      limit: 100,
      offset: 0,
    });

    const result = await getCompanyActivity("7d");

    expect(result.totalActivities).toBe(1);
    expect(result.changePercent).toBe(0);
    expect(result.departments).toEqual([
      expect.objectContaining({ name: "Construction", activities: 1, tasksCompleted: 1, activeStaff: 1 }),
    ]);
    expect(result.activityTypes).toEqual([{ name: "Approved", count: 1 }]);
    expect(result.timeline.reduce(
      (sum, point) => sum + point.departments.reduce((daySum, item) => daySum + item.value, 0),
      0,
    )).toBe(1);
    expect(sourceMocks.listAuditEvents).toHaveBeenCalledWith(
      expect.objectContaining({ limit: 100, offset: 0 }),
    );
  });

  it("builds personal activity from task records and returns actual task changes", async () => {
    mockTaskList([
      task("completed", "approved", dayOffset(1)),
      task("prior", "in_progress", dayOffset(8)),
    ]);

    const result = await getMyActivity("7d");

    expect(result.dailyTrend.reduce((sum, item) => sum + item.value, 0)).toBe(1);
    expect(result.changePercent).toBe(0);
    expect(result.activityScore).toBeGreaterThan(0);
    expect(result.taskBreakdown.find((item) => item.name === "Completed")?.count).toBe(1);
    expect(result.contributionWeeks).toHaveLength(12);
  });

  it("counts the current pending queue and preserves unassigned items", async () => {
    const currentTasks = [
      task("review", "submitted", dayOffset(2)),
      task("overdue", "in_progress", dayOffset(4), dayOffset(2)),
    ];
    mockTaskList(currentTasks);
    sourceMocks.listStaff.mockResolvedValue({
      staff: [
        ...staff.map((member) => ({ ...member, employment_status: "pending" as const })),
        { ...staff[0], id: "staff-unassigned", account_id: null, department_id: null, employment_status: "pending" },
      ],
      total: 2,
      pagination: { limit: 100, offset: 0, total: 2 },
    });

    const result = await getPending();

    expect(result.totalPending).toBe(4);
    expect(result.totalPending).toBe(result.byType.reduce((sum, item) => sum + item.count, 0));
    expect(result.totalPending).toBe(result.aging.reduce((sum, item) => sum + item.count, 0));
    expect(result.totalPending).toBe(result.departments.reduce((sum, item) => sum + item.count, 0));
    expect(result.topItems.some((item) => item.title === "Test User" && item.ageDays === null)).toBe(true);
    expect(result.departments.find((item) => item.name === "Unassigned / unknown")?.count).toBe(1);
  });
});
