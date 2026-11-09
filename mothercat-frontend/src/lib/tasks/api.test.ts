import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { getTask, listTasks, updateTask } from "@/lib/tasks/api";

const task = {
  id: "task-uuid",
  title: "Inspect site",
  description: "",
  priority: "normal",
  status: "draft",
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
});
