import { buildApiUrl } from "@/lib/api/config";
import { apiRoutes } from "@/lib/api/routes";
import { fetchWithRetry } from "@/lib/api/retry";

export const taskStatuses = [
  "draft",
  "assigned",
  "in_progress",
  "submitted",
  "changes_requested",
  "approved",
  "rejected",
  "cancelled",
] as const;
export type TaskStatus = (typeof taskStatuses)[number];
export const adminTaskStatuses = ["open", "scheduled", "in_progress", "closed"] as const;
export type AdminTaskStatus = (typeof adminTaskStatuses)[number];
export type TaskPriority = "low" | "normal" | "high" | "urgent";
export type TaskAction =
  | "start"
  | "submit"
  | "approve"
  | "request-changes"
  | "reject"
  | "cancel";

export type TaskSubmission = {
  id: string;
  submission_number: number;
  summary: string;
  links: string[];
  submitter_account_id: string;
  submitted_at: string;
};

export type Task = {
  id: string;
  title: string;
  description: string;
  priority: TaskPriority;
  status: TaskStatus;
  admin_status: AdminTaskStatus;
  admin_status_updated_by: string | null;
  admin_status_updated_at: string | null;
  created_by: string;
  department_id: string;
  assignee_account_id: string | null;
  due_at: string | null;
  version: number;
  created_at: string;
  updated_at: string;
  latest_submission: TaskSubmission | null;
};

export type TaskComment = {
  id: string;
  body: string;
  created_at: string;
  author_account_id: string | null;
};

export type TaskListResult = {
  tasks: Task[];
  page: { limit: number; offset: number; total: number };
};

export type TaskVersionedResult = { task: Task; etag: string | null };

function isTaskPriority(value: unknown): value is TaskPriority {
  return value === "low" || value === "normal" || value === "high" || value === "urgent";
}

function isAdminTaskStatus(value: unknown): value is AdminTaskStatus {
  return adminTaskStatuses.includes(value as AdminTaskStatus);
}

export class TaskApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "TaskApiError";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function requiredString(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  if (typeof value !== "string") throw new TaskApiError("The task service returned an invalid response.", 502, "invalid_response");
  return value;
}

function parseTask(value: unknown): Task {
  if (!isRecord(value)) throw new TaskApiError("The task service returned an invalid task.", 502, "invalid_response");
  const priority = value.priority;
  const status = value.status;
  if (
    typeof value.version !== "number" ||
    !isTaskPriority(priority) ||
    !taskStatuses.includes(status as TaskStatus) ||
    !isAdminTaskStatus(value.admin_status) ||
    (value.admin_status_updated_by !== undefined && value.admin_status_updated_by !== null && typeof value.admin_status_updated_by !== "string") ||
    (value.admin_status_updated_at !== undefined && value.admin_status_updated_at !== null && typeof value.admin_status_updated_at !== "string") ||
    (value.assignee_account_id !== undefined && value.assignee_account_id !== null && typeof value.assignee_account_id !== "string") ||
    (value.due_at !== undefined && value.due_at !== null && typeof value.due_at !== "string")
  ) {
    throw new TaskApiError("The task service returned an invalid task.", 502, "invalid_response");
  }

  let latestSubmission: TaskSubmission | null = null;
  if (value.latest_submission !== undefined && value.latest_submission !== null) {
    const submission = value.latest_submission;
    if (
      !isRecord(submission) ||
      typeof submission.id !== "string" ||
      typeof submission.submission_number !== "number" ||
      typeof submission.summary !== "string" ||
      !Array.isArray(submission.links) ||
      !submission.links.every((link) => typeof link === "string") ||
      typeof submission.submitter_account_id !== "string" ||
      typeof submission.submitted_at !== "string"
    ) {
      throw new TaskApiError("The task service returned an invalid submission.", 502, "invalid_response");
    }
    latestSubmission = {
      id: submission.id,
      submission_number: submission.submission_number,
      summary: submission.summary,
      links: submission.links,
      submitter_account_id: submission.submitter_account_id,
      submitted_at: submission.submitted_at,
    };
  }

  return {
    id: requiredString(value, "id"),
    title: requiredString(value, "title"),
    description: requiredString(value, "description"),
    priority,
    status: status as TaskStatus,
    admin_status: value.admin_status,
    admin_status_updated_by: typeof value.admin_status_updated_by === "string" ? value.admin_status_updated_by : null,
    admin_status_updated_at: typeof value.admin_status_updated_at === "string" ? value.admin_status_updated_at : null,
    created_by: requiredString(value, "created_by"),
    department_id: requiredString(value, "department_id"),
    assignee_account_id: typeof value.assignee_account_id === "string" ? value.assignee_account_id : null,
    due_at: typeof value.due_at === "string" ? value.due_at : null,
    version: value.version,
    created_at: requiredString(value, "created_at"),
    updated_at: requiredString(value, "updated_at"),
    latest_submission: latestSubmission,
  };
}

async function request(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body !== undefined) headers.set("Content-Type", "application/json");
  let response: Response;
  try {
    response = await fetchWithRetry(buildApiUrl(path), {
      ...init,
      headers,
      credentials: "include",
      cache: "no-store",
    });
  } catch {
    throw new TaskApiError("Could not reach the task service. Check your connection and try again.", 0, "network_error");
  }
  if (!response.ok) {
    let payload: unknown;
    try {
      payload = await response.json();
    } catch {
      payload = null;
    }
    const error = isRecord(payload) && isRecord(payload.error) ? payload.error : undefined;
    throw new TaskApiError(
      typeof error?.message === "string" ? error.message : "The request could not be completed.",
      response.status,
      typeof error?.code === "string" ? error.code : "request_failed",
      typeof error?.request_id === "string" ? error.request_id : response.headers.get("X-Request-ID") ?? undefined,
    );
  }
  return response;
}

async function jsonResponse(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    throw new TaskApiError("The task service returned an invalid response.", 502, "invalid_response");
  }
}

function parseTaskEnvelope(payload: unknown): Task {
  if (!isRecord(payload) || !("task" in payload)) {
    throw new TaskApiError("The task service returned an invalid task response.", 502, "invalid_response");
  }
  return parseTask(payload.task);
}

export async function listTasks(
  params: {
    mine?: boolean;
    status?: TaskStatus | "";
    adminStatus?: AdminTaskStatus | "";
    department?: string;
    assignee?: string;
    dueFrom?: string;
    dueTo?: string;
    limit?: number;
    offset?: number;
  } = {},
  signal?: AbortSignal,
): Promise<TaskListResult> {
  const query = new URLSearchParams({
    limit: String(params.limit ?? 50),
    offset: String(params.offset ?? 0),
  });
  if (params.status) query.set("status", params.status);
  if (params.adminStatus) query.set("admin_status", params.adminStatus);
  if (params.department) query.set("department", params.department);
  if (params.assignee) query.set("assignee", params.assignee);
  if (params.dueFrom) query.set("due_from", params.dueFrom);
  if (params.dueTo) query.set("due_to", params.dueTo);
  const response = await request(`${params.mine ? apiRoutes.tasks.mine : apiRoutes.tasks.list}?${query}`, { signal });
  const payload = await jsonResponse(response);
  if (
    !isRecord(payload) ||
    !Array.isArray(payload.tasks) ||
    !isRecord(payload.page) ||
    typeof payload.page.limit !== "number" ||
    typeof payload.page.offset !== "number" ||
    typeof payload.page.total !== "number"
  ) {
    throw new TaskApiError("The task service returned an invalid task list.", 502, "invalid_response");
  }
  return {
    tasks: payload.tasks.map(parseTask),
    page: { limit: payload.page.limit, offset: payload.page.offset, total: payload.page.total },
  };
}

export async function getTask(taskId: string, signal?: AbortSignal): Promise<TaskVersionedResult> {
  const response = await request(apiRoutes.tasks.details(encodeURIComponent(taskId)), { signal });
  return { task: parseTaskEnvelope(await jsonResponse(response)), etag: response.headers.get("ETag") };
}

export async function createTask(input: {
  title: string;
  description: string;
  priority: TaskPriority;
  department_id: string;
  due_at: string | null;
}): Promise<TaskVersionedResult> {
  const response = await request(apiRoutes.tasks.list, { method: "POST", body: JSON.stringify(input) });
  return { task: parseTaskEnvelope(await jsonResponse(response)), etag: response.headers.get("ETag") };
}

export async function updateTask(
  taskId: string,
  etag: string,
  input: Partial<Pick<Task, "title" | "description" | "priority" | "due_at">>,
): Promise<TaskVersionedResult> {
  if (!etag) throw new TaskApiError("Reload this task before saving; its latest version is unavailable.", 428, "if_match_required");
  const response = await request(apiRoutes.tasks.details(encodeURIComponent(taskId)), {
    method: "PATCH",
    headers: { "If-Match": etag },
    body: JSON.stringify(input),
  });
  return { task: parseTaskEnvelope(await jsonResponse(response)), etag: response.headers.get("ETag") };
}

export async function updateTaskAdminStatus(
  taskId: string,
  etag: string,
  status: AdminTaskStatus,
): Promise<TaskVersionedResult> {
  if (!etag) throw new TaskApiError("Reload this task before changing its status.", 428, "if_match_required");
  const response = await request(apiRoutes.tasks.adminStatus(encodeURIComponent(taskId)), {
    method: "PATCH",
    headers: { "If-Match": etag },
    body: JSON.stringify({ status }),
  });
  return { task: parseTaskEnvelope(await jsonResponse(response)), etag: response.headers.get("ETag") };
}

export async function assignTask(taskId: string, accountId: string): Promise<TaskVersionedResult> {
  const response = await request(apiRoutes.tasks.assign(encodeURIComponent(taskId)), {
    method: "POST",
    body: JSON.stringify({ assignee_account_id: accountId }),
  });
  return { task: parseTaskEnvelope(await jsonResponse(response)), etag: response.headers.get("ETag") };
}

export async function performTaskAction(
  taskId: string,
  action: TaskAction,
  body?: Record<string, unknown>,
): Promise<TaskVersionedResult> {
  const response = await request(apiRoutes.tasks.action(encodeURIComponent(taskId), action), {
    method: "POST",
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  return { task: parseTaskEnvelope(await jsonResponse(response)), etag: response.headers.get("ETag") };
}

export async function listTaskComments(taskId: string, signal?: AbortSignal): Promise<TaskComment[]> {
  const query = new URLSearchParams({ limit: "50", offset: "0" });
  const response = await request(`${apiRoutes.tasks.comments(encodeURIComponent(taskId))}?${query}`, { signal });
  const payload = await jsonResponse(response);
  if (!isRecord(payload) || !Array.isArray(payload.comments)) {
    throw new TaskApiError("The task service returned an invalid comments list.", 502, "invalid_response");
  }
  return payload.comments.map((item): TaskComment => {
    if (
      !isRecord(item) ||
      typeof item.id !== "string" ||
      typeof item.body !== "string" ||
      typeof item.created_at !== "string" ||
      (item.author_account_id !== undefined && item.author_account_id !== null && typeof item.author_account_id !== "string")
    ) {
      throw new TaskApiError("The task service returned an invalid comment.", 502, "invalid_response");
    }
    return {
      id: item.id,
      body: item.body,
      created_at: item.created_at,
      author_account_id: typeof item.author_account_id === "string" ? item.author_account_id : null,
    };
  });
}

export async function addTaskComment(taskId: string, body: string): Promise<TaskComment> {
  const response = await request(apiRoutes.tasks.comments(encodeURIComponent(taskId)), {
    method: "POST",
    body: JSON.stringify({ body }),
  });
  const payload = await jsonResponse(response);
  if (!isRecord(payload) || !isRecord(payload.comment)) {
    throw new TaskApiError("The task service returned an invalid comment.", 502, "invalid_response");
  }
  const comment = payload.comment;
  if (
    typeof comment.id !== "string" ||
    typeof comment.body !== "string" ||
    typeof comment.created_at !== "string" ||
    (comment.author_account_id !== undefined && comment.author_account_id !== null && typeof comment.author_account_id !== "string")
  ) {
    throw new TaskApiError("The task service returned an invalid comment.", 502, "invalid_response");
  }
  return {
    id: comment.id,
    body: comment.body,
    created_at: comment.created_at,
    author_account_id: typeof comment.author_account_id === "string" ? comment.author_account_id : null,
  };
}

export function formatTaskError(error: unknown, fallback: string): string {
  if (!(error instanceof TaskApiError)) return fallback;
  if (error.code === "origin_not_allowed" || error.code === "invalid_origin") {
    return "This action was blocked by the site security check. Refresh the page and try again.";
  }
  if (error.code === "unauthorized" || error.status === 401) return "Your session expired. Sign in again.";
  if (error.code === "permission_denied" || error.status === 403) return "You don’t have permission to perform this task action.";
  if (error.code === "task_conflict" || error.status === 409) return "This task has changed state. Refresh it before trying again.";
  if (error.code === "version_conflict" || error.status === 412) return "This task was updated by someone else. Reload it to get the latest version.";
  if (error.status === 503) return "The task service is temporarily unavailable. Try again later.";
  return error.requestId ? `${fallback} (Request ID: ${error.requestId})` : fallback;
}
