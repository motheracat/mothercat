import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createTask, getTask, listTasks, updateTask, updateTaskAdminStatus } from "@/lib/tasks/api";

const task = {
  id: "task-uuid",
  title: "Inspect site",
  description: "",
  priority: "normal",
  status: "draft",
  admin_status: "open",
  admin_status_updated_by: null,
  admin_status_updated_at: null,
  created_by: "account-uuid",
  department_id: "department-uuid",
  assignee_account_id: null,
  due_at: null,
  version: 1,
  created_at: "2026-11-07T12:00:00Z",
  updated_at: "2026-11-07T12:00:00Z",
};

const fetchMock = vi.fn<typeof fetch>();

function jsonResponse(payload: unknown, status = 200, headers?: HeadersInit) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "Content-Type": "application/json", ...Object.fromEntries(new Headers(headers)) },
  });
}

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("task API client", () => {
  it("uses the current user's task list for self-service and parses pagination", async () => {
    fetchMock.mockResolvedValue(jsonResponse({
      tasks: [task],
      page: { limit: 20, offset: 0, total: 1 },
    }));

    await expect(listTasks({ mine: true, limit: 20, offset: 0 })).resolves.toMatchObject({
      tasks: [{ id: "task-uuid", status: "draft" }],
      page: { total: 1 },
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/me/tasks?limit=20&offset=0",
      expect.objectContaining({ credentials: "include", cache: "no-store" }),
    );
  });

  it("includes the required assignee in task creation", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ task: { ...task, status: "assigned", assignee_account_id: "assignee-uuid" } }));

    await createTask({
      title: "Inspect site",
      description: "",
      priority: "normal",
      department_id: "department-uuid",
      assignee_account_id: "assignee-uuid",
      due_at: null,
    });

    expect(fetchMock.mock.calls[0][1]?.body).toBe(JSON.stringify({
      title: "Inspect site",
      description: "",
      priority: "normal",
      department_id: "department-uuid",
      assignee_account_id: "assignee-uuid",
      due_at: null,
    }));
  });

  it("reads a versioned task and forwards its ETag on edits", async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ task }, 200, { ETag: '"1"' }))
      .mockResolvedValueOnce(jsonResponse({ task: { ...task, title: "Inspect roof" } }, 200, { ETag: '"2"' }));

    const current = await getTask("task-uuid");
    expect(current.etag).toBe('"1"');
    const updated = await updateTask(current.task.id, current.etag!, { title: "Inspect roof" });

    expect(updated.task.title).toBe("Inspect roof");
    expect(updated.etag).toBe('"2"');
    const [url, options] = fetchMock.mock.calls[1];
    expect(url).toBe("/api/v1/tasks/task-uuid");
    expect(options?.method).toBe("PATCH");
    expect(new Headers(options?.headers).get("If-Match")).toBe('"1"');
    expect(options?.body).toBe(JSON.stringify({ title: "Inspect roof" }));
  });

  it("uses the task version when an intermediary omits the ETag response header", async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ task }))
      .mockResolvedValueOnce(jsonResponse({ task: { ...task, admin_status: "scheduled", version: 2 } }));

    const current = await getTask("task-uuid");
    expect(current.etag).toBe('"1"');
    const updated = await updateTaskAdminStatus(current.task.id, current.etag!, "scheduled");

    expect(updated.etag).toBe('"2"');
    expect(new Headers(fetchMock.mock.calls[1][1]?.headers).get("If-Match")).toBe('"1"');
  });

  it("filters by administrative status and persists an admin status change with the current ETag", async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse({
        tasks: [{ ...task, admin_status: "scheduled" }],
        page: { limit: 20, offset: 0, total: 1 },
      }))
      .mockResolvedValueOnce(jsonResponse({ task: { ...task, admin_status: "in_progress", admin_status_updated_by: "admin-id" } }, 200, { ETag: '"2"' }));

    await expect(listTasks({ adminStatus: "scheduled", limit: 20, offset: 0 })).resolves.toMatchObject({
      tasks: [{ admin_status: "scheduled" }],
    });
    const updated = await updateTaskAdminStatus("task-uuid", '"1"', "in_progress");

    expect(updated.task.admin_status).toBe("in_progress");
    expect(updated.task.admin_status_updated_by).toBe("admin-id");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/tasks?limit=20&offset=0&admin_status=scheduled");
    const [url, options] = fetchMock.mock.calls[1];
    expect(url).toBe("/api/v1/tasks/task-uuid/admin-status");
    expect(options?.method).toBe("PATCH");
    expect(new Headers(options?.headers).get("If-Match")).toBe('"1"');
    expect(options?.body).toBe(JSON.stringify({ status: "in_progress" }));
  });
});
