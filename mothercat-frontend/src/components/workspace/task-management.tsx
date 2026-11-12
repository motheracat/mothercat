"use client";

import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { ClipboardList, MessageSquare, RefreshCw, Send, UserPlus } from "lucide-react";

import { TaskAccordionSelect } from "@/components/workspace/task-accordion-select";
import { DepartmentSelect } from "@/components/workspace/department-select";
import { TaskDetailsModal } from "@/components/workspace/task-details-modal";
import { EmptyState, ErrorBanner, PageHeader, StatusPill } from "@/components/workspace/ui";
import {
  formatOrganizationError,
  listDepartments,
  listStaff,
  type Department,
  type StaffMember,
} from "@/lib/organization/api";
import {
  addTaskComment,
  assignTask,
  formatTaskError,
  getTask,
  listTaskComments,
  listTasks,
  performTaskAction,
  adminTaskStatuses,
  updateTask,
  updateTaskAdminStatus,
  type Task,
  type AdminTaskStatus,
  type TaskAction,
  type TaskComment,
  type TaskPriority,
  type TaskStatus,
  type TaskVersionedResult,
} from "@/lib/tasks/api";

type Props = {
  currentUserId: string;
  canCreate: boolean;
  canAssign: boolean;
  canReview: boolean;
  canCancel: boolean;
  canUpdateStatus: boolean;
};

const pageSize = 20;

const statusLabels: Record<AdminTaskStatus, string> = {
  open: "Open",
  scheduled: "Scheduled",
  in_progress: "In progress",
  closed: "Closed (completed)",
};

const taskStatusLabels: Record<TaskStatus, string> = {
  draft: "Draft",
  assigned: "Assigned",
  in_progress: "In progress",
  submitted: "Submitted",
  changes_requested: "Changes requested",
  approved: "Approved",
  rejected: "Rejected",
  cancelled: "Cancelled",
};

function formatDate(value: string | null): string {
  if (!value) return "No due date";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "Invalid date" : date.toLocaleString();
}

function toLocalDateTime(value: string | null): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function TaskFields({
  task,
  departments,
  onSubmit,
  submitLabel,
  busy,
}: {
  task?: Task;
  departments: Department[];
  onSubmit: (values: { title: string; description: string; priority: TaskPriority; department_id: string; due_at: string | null }) => void;
  submitLabel: string;
  busy: boolean;
}) {
  const [priority, setPriority] = useState<TaskPriority>(task?.priority ?? "normal");
  const [departmentId, setDepartmentId] = useState(task?.department_id ?? "");

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const dateValue = String(data.get("due_at") ?? "");
    onSubmit({
      title: String(data.get("title") ?? "").trim(),
      description: String(data.get("description") ?? "").trim(),
      priority: String(data.get("priority") ?? "normal") as TaskPriority,
      department_id: String(data.get("department_id") ?? ""),
      due_at: dateValue ? new Date(dateValue).toISOString() : null,
    });
  }

  return (
    <form id="task-fields-form" className="task-form" onSubmit={submit}>
      <label className="workspace-field">
        <span className="field-label">Task title</span>
        <input className="form-input" name="title" defaultValue={task?.title ?? ""} maxLength={200} required />
      </label>
      <label className="workspace-field">
        <span className="field-label">Description</span>
        <textarea className="form-input form-textarea" name="description" defaultValue={task?.description ?? ""} maxLength={20_000} rows={4} />
      </label>
      <div className="task-form-grid">
        <label className="workspace-field">
          <span className="field-label">Priority</span>
          <TaskAccordionSelect
            label="Task priority"
            name="priority"
            value={priority}
            placeholder="Select priority"
            options={(["low", "normal", "high", "urgent"] as const).map((option) => ({
              value: option,
              label: option.charAt(0).toUpperCase() + option.slice(1),
            }))}
            onChange={(value) => setPriority(value as TaskPriority)}
            disabled={busy}
          />
        </label>
        {!task ? (
          <label className="workspace-field">
            <span className="field-label">Department</span>
            <DepartmentSelect
              label="Department"
              name="department_id"
              value={departmentId}
              departments={departments}
              placeholder="Select a department"
              onChange={setDepartmentId}
              disabled={busy}
            />
          </label>
        ) : null}
        <label className="workspace-field">
          <span className="field-label">Due date</span>
          <input className="form-input" name="due_at" type="datetime-local" defaultValue={toLocalDateTime(task?.due_at ?? null)} />
        </label>
      </div>
      <button type="submit" className="sr-only" disabled={busy}>{submitLabel}</button>
    </form>
  );
}

export function TaskManagement({ currentUserId, canCreate, canAssign, canReview, canCancel, canUpdateStatus }: Props) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const selectedTaskId = searchParams.get("task");
  const managerView = canCreate || canAssign || canReview || canCancel || canUpdateStatus;
  const [tasks, setTasks] = useState<Task[]>([]);
  const [departments, setDepartments] = useState<Department[]>([]);
  const [departmentsError, setDepartmentsError] = useState<string | null>(null);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [filterStatus, setFilterStatus] = useState<AdminTaskStatus | "">("");
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [departmentsReloadKey, setDepartmentsReloadKey] = useState(0);
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const [taskEtag, setTaskEtag] = useState<string | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailReloadKey, setDetailReloadKey] = useState(0);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [editing, setEditing] = useState(false);
  const [submissionSummary, setSubmissionSummary] = useState("");
  const [evidenceLinks, setEvidenceLinks] = useState("");
  const [feedback, setFeedback] = useState("");
  const [assignees, setAssignees] = useState<StaffMember[]>([]);
  const [assigneeId, setAssigneeId] = useState("");
  const [adminStatusDraft, setAdminStatusDraft] = useState<AdminTaskStatus>("open");
  const [assigneeLoading, setAssigneeLoading] = useState(false);
  const [comments, setComments] = useState<TaskComment[]>([]);
  const [commentsLoading, setCommentsLoading] = useState(false);
  const [commentDraft, setCommentDraft] = useState("");

  const departmentsById = useMemo(() => new Map(departments.map((department) => [department.id, department.name])), [departments]);

  useEffect(() => {
    if (!managerView) return;
    const controller = new AbortController();
    void listDepartments(controller.signal)
      .then((result) => {
        setDepartments(result);
        setDepartmentsError(null);
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setDepartmentsError(formatOrganizationError(error, "Unable to load departments for task assignment."));
        }
      });
    return () => controller.abort();
  }, [departmentsReloadKey, managerView]);

  const reload = useCallback(() => {
    setLoading(true);
    setDepartmentsError(null);
    setReloadKey((key) => key + 1);
    setDepartmentsReloadKey((key) => key + 1);
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void listTasks({
      mine: !managerView,
      ...(filterStatus ? { adminStatus: filterStatus } : {}),
      limit: pageSize,
      offset,
    }, controller.signal)
      .then((result) => {
        setTasks(result.tasks);
        setTotal(result.page.total);
        setLoadError(null);
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setTasks([]);
          setTotal(0);
          setLoadError(formatTaskError(error, "Unable to load tasks."));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [managerView, filterStatus, offset, reloadKey]);

  useEffect(() => {
    const taskId = selectedTask?.id;
    const departmentId = selectedTask?.department_id;
    if (!taskId || !departmentId || !canAssign) return;
    const controller = new AbortController();
    void listStaff({
      limit: 100,
      offset: 0,
      departmentId,
      employmentStatus: "active",
    }, controller.signal)
      .then(({ staff }) => {
        if (!controller.signal.aborted) {
          setAssignees(staff.filter((member) => member.account_id && member.employment_status === "active"));
        }
      })
      .catch(() => {
        if (!controller.signal.aborted) setDetailError("Unable to load eligible assignees for this department.");
      })
      .finally(() => {
        if (!controller.signal.aborted) setAssigneeLoading(false);
      });
    return () => controller.abort();
  }, [selectedTask?.id, selectedTask?.department_id, canAssign]);

  useEffect(() => {
    const taskId = selectedTask?.id;
    if (!taskId) return;
    const controller = new AbortController();
    void listTaskComments(taskId, controller.signal)
      .then(setComments)
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setDetailError((current) => current ?? formatTaskError(error, "Unable to load task comments."));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setCommentsLoading(false);
      });
    return () => controller.abort();
  }, [selectedTask?.id, reloadKey]);

  useEffect(() => {
    if (!selectedTaskId) return;

    const controller = new AbortController();
    queueMicrotask(() => {
      if (controller.signal.aborted) return;
      setTaskEtag(null);
      setDetailError(null);
      setEditing(false);
      setDetailLoading(true);
      setFeedback("");
      setSubmissionSummary("");
      setEvidenceLinks("");
      setAssignees([]);
      setAssigneeLoading(canAssign);
      setCommentsLoading(true);
      setAssigneeId("");
      void getTask(selectedTaskId, controller.signal)
        .then((current) => {
          if (controller.signal.aborted) return;
          setSelectedTask(current.task);
          setTaskEtag(current.etag);
          setAssigneeId(current.task.assignee_account_id ?? "");
          setAdminStatusDraft(current.task.admin_status);
        })
        .catch((error: unknown) => {
          if (!controller.signal.aborted) setDetailError(formatTaskError(error, "Unable to load this task."));
        })
        .finally(() => {
          if (!controller.signal.aborted) setDetailLoading(false);
        });
    });
    return () => controller.abort();
  }, [selectedTaskId, detailReloadKey, canAssign]);

  function openTask(task: Task) {
    const params = new URLSearchParams(searchParams.toString());
    params.set("task", task.id);
    router.push(`${pathname}?${params.toString()}`, { scroll: false });
  }

  function closeTaskDetails() {
    const params = new URLSearchParams(searchParams.toString());
    params.delete("task");
    const query = params.toString();
    router.replace(`${pathname}${query ? `?${query}` : ""}`, { scroll: false });
    setEditing(false);
  }

  async function refreshSelectedTask() {
    if (!selectedTask) {
      if (selectedTaskId) setDetailReloadKey((key) => key + 1);
      return;
    }
    setDetailLoading(true);
    setDetailError(null);
    try {
      const current = await getTask(selectedTask.id);
      setSelectedTask(current.task);
      setTaskEtag(current.etag);
      setAssigneeId(current.task.assignee_account_id ?? "");
      setAdminStatusDraft(current.task.admin_status);
      setLoading(true);
      setReloadKey((key) => key + 1);
    } catch (error) {
      setDetailError(formatTaskError(error, "Unable to reload this task."));
    } finally {
      setDetailLoading(false);
    }
  }

  function applyUpdatedTask(result: TaskVersionedResult) {
    setSelectedTask(result.task);
    setTaskEtag(result.etag);
    setAdminStatusDraft(result.task.admin_status);
    setTasks((current) => current.map((task) => task.id === result.task.id ? result.task : task));
  }

  async function saveAdminStatus() {
    if (!selectedTask || !taskEtag || !canUpdateStatus) return;
    setSaving(true);
    setDetailError(null);
    try {
      applyUpdatedTask(await updateTaskAdminStatus(selectedTask.id, taskEtag, adminStatusDraft));
      setLoading(true);
      setReloadKey((key) => key + 1);
    } catch (error) {
      setDetailError(formatTaskError(error, "Unable to update the task status."));
    } finally {
      setSaving(false);
    }
  }

  async function saveTask(values: { title: string; description: string; priority: TaskPriority; department_id: string; due_at: string | null }) {
    if (!selectedTask || !taskEtag) {
      setDetailError("Reload the task to get its latest version before saving.");
      return;
    }
    const changes: Partial<Pick<Task, "title" | "description" | "priority" | "due_at">> = {};
    if (values.title !== selectedTask.title) changes.title = values.title;
    if (values.description !== selectedTask.description) changes.description = values.description;
    if (values.priority !== selectedTask.priority) changes.priority = values.priority;
    if (values.due_at !== selectedTask.due_at) changes.due_at = values.due_at;
    if (Object.keys(changes).length === 0) {
      setEditing(false);
      return;
    }
    setSaving(true);
    setDetailError(null);
    try {
      applyUpdatedTask(await updateTask(selectedTask.id, taskEtag, changes));
      setEditing(false);
    } catch (error) {
      setDetailError(formatTaskError(error, "Unable to update this task."));
    } finally {
      setSaving(false);
    }
  }

  async function saveAssignee() {
    if (!selectedTask || !assigneeId) return;
    setSaving(true);
    setDetailError(null);
    try {
      applyUpdatedTask(await assignTask(selectedTask.id, assigneeId));
    } catch (error) {
      setDetailError(formatTaskError(error, "Unable to assign this task."));
    } finally {
      setSaving(false);
    }
  }

  async function runAction(action: TaskAction) {
    if (!selectedTask) return;
    const body: Record<string, unknown> = {};
    if (action === "submit") {
      const links = evidenceLinks.split(/\r?\n/).map((link) => link.trim()).filter(Boolean);
      if (!submissionSummary.trim()) {
        setDetailError("Add a summary before submitting your work.");
        return;
      }
      if (links.length > 10 || links.some((link) => {
        try {
          return new URL(link).protocol !== "https:";
        } catch {
          return true;
        }
      })) {
        setDetailError("Evidence links must be valid HTTPS URLs (up to 10).");
        return;
      }
      body.summary = submissionSummary.trim();
      body.links = links;
    }
    if (action === "approve" || action === "request-changes" || action === "reject") {
      if (action !== "approve" && !feedback.trim()) {
        setDetailError("Feedback is required for this decision.");
        return;
      }
      if (feedback.length > 2000) {
        setDetailError("Feedback must be no more than 2,000 characters.");
        return;
      }
      if (feedback.trim() || action === "approve") body.feedback = feedback.trim();
    }
    setSaving(true);
    setDetailError(null);
    try {
      applyUpdatedTask(await performTaskAction(selectedTask.id, action, Object.keys(body).length ? body : undefined));
      setSubmissionSummary("");
      setEvidenceLinks("");
      setFeedback("");
      reload();
    } catch (error) {
      setDetailError(formatTaskError(error, "Unable to update the task."));
    } finally {
      setSaving(false);
    }
  }

  async function submitComment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedTask || !commentDraft.trim()) return;
    setSaving(true);
    setDetailError(null);
    try {
      const comment = await addTaskComment(selectedTask.id, commentDraft.trim());
      setComments((current) => [...current, comment]);
      setCommentDraft("");
    } catch (error) {
      setDetailError(formatTaskError(error, "Unable to add your comment."));
    } finally {
      setSaving(false);
    }
  }

  const isAssignee = selectedTask?.assignee_account_id === currentUserId;
  const canEditDraft = selectedTask?.status === "draft" && selectedTask.created_by === currentUserId;
  const canStart = selectedTask?.status === "assigned" && isAssignee;
  const canSubmit = selectedTask?.status === "in_progress" && isAssignee;
  const canDecide = selectedTask?.status === "submitted" && canReview;
  const canCancelSelected = selectedTask && canCancel && !["approved", "rejected", "cancelled"].includes(selectedTask.status);
  const canAssignSelected = canAssign && selectedTask && ["draft", "assigned"].includes(selectedTask.status);

  return (
    <div className="task-management-page">
      <PageHeader
        title="Tasks"
        description="Track task progress, assignments, and administrative scheduling."
        actions={canCreate ? <Link href="/tasks/create" className="button-primary">Create task</Link> : null}
      />
      {loadError ? <ErrorBanner message={loadError} onRetry={reload} /> : null}
      {departmentsError ? <ErrorBanner message={departmentsError} onRetry={reload} /> : null}
      <section className="surface-card task-list-card" aria-label="Task list" aria-busy={loading}>
        <div className="task-list-toolbar">
          <label className="workspace-field">
            <span className="field-label">Administrative status</span>
            <TaskAccordionSelect
              label="Administrative status"
              value={filterStatus}
              placeholder="All statuses"
              options={[
                { value: "", label: "All statuses" },
                ...adminTaskStatuses.map((status) => ({ value: status, label: statusLabels[status] })),
              ]}
              onChange={(status) => {
                setLoading(true);
                setFilterStatus(status as AdminTaskStatus | "");
                setOffset(0);
              }}
            />
          </label>
          <button type="button" className="button-ghost" onClick={reload} disabled={loading} aria-label="Refresh tasks">
            <RefreshCw size={17} aria-hidden="true" />
          </button>
        </div>
        {loading ? (
          <div role="status" aria-busy="true" className="task-list-loading">Loading tasks…</div>
        ) : tasks.length ? (
          <ul className="task-list">
            {tasks.map((task) => (
              <li key={task.id}>
                <button type="button" className="task-list-item" onClick={() => openTask(task)}>
                  <span className="task-list-icon"><ClipboardList size={19} aria-hidden="true" /></span>
                  <span className="task-list-copy">
                    <strong>{task.title}</strong>
                    <span>{departmentsById.get(task.department_id) ?? `Department ID ${task.department_id}`} · Due {formatDate(task.due_at)}</span>
                  </span>
                  <span className="task-list-meta">
                    <StatusPill label={taskStatusLabels[task.status]} tone="info" />
                    <StatusPill label={`Admin: ${statusLabels[task.admin_status]}`} tone="info" />
                    <StatusPill label={`${task.priority} priority`} tone="info" />
                  </span>
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <EmptyState>
            {loadError ? "Tasks could not be loaded." : "No tasks match this view."}
          </EmptyState>
        )}
        <div className="task-pagination">
          <span>{total ? `Showing ${offset + 1}–${Math.min(offset + tasks.length, total)} of ${total}` : "No tasks"}</span>
          <div>
            <button type="button" className="button-secondary" disabled={loading || offset === 0} onClick={() => { setLoading(true); setOffset((value) => Math.max(0, value - pageSize)); }}>Previous</button>
            <button type="button" className="button-secondary" disabled={loading || offset + pageSize >= total} onClick={() => { setLoading(true); setOffset((value) => value + pageSize); }}>Next</button>
          </div>
        </div>
      </section>

      <TaskDetailsModal
        open={Boolean(selectedTaskId)}
        title={selectedTask?.title ?? "Task details"}
        headerMeta={selectedTask ? (
          <>
            <StatusPill label={taskStatusLabels[selectedTask.status]} tone="info" />
            <StatusPill label={`Admin: ${statusLabels[selectedTask.admin_status]}`} tone="info" />
            <StatusPill label={`${selectedTask.priority} priority`} tone="info" />
          </>
        ) : null}
        onClose={closeTaskDetails}
        footer={
          <>
            {detailError ? <button type="button" className="button-secondary" disabled={detailLoading || saving} onClick={() => void refreshSelectedTask()}>Reload task</button> : null}
            {canEditDraft && !editing ? <button type="button" className="button-secondary" onClick={() => setEditing(true)}>Edit draft</button> : null}
            {editing ? (
              <>
                <button type="button" className="button-secondary" disabled={saving} onClick={() => setEditing(false)}>Cancel edit</button>
                <button type="submit" form="task-fields-form" className="button-primary" disabled={saving}>{saving ? "Saving…" : "Save task"}</button>
              </>
            ) : null}
          </>
        }
      >
        {detailLoading || (Boolean(selectedTaskId) && selectedTask?.id !== selectedTaskId && !detailError) ? (
          <div className="task-details-skeleton" role="status" aria-busy="true" aria-label="Loading task details">
            <span /><span /><span /><span />
          </div>
        ) : null}
        {detailError ? <p role="alert" className="field-error">{detailError}</p> : null}
        {selectedTask && selectedTask.id === selectedTaskId && !detailLoading ? (
          <div className="task-detail">
            {editing ? (
              <TaskFields task={selectedTask} departments={departments} onSubmit={(values) => void saveTask(values)} submitLabel="Save task" busy={saving} />
            ) : (
              <>
                {selectedTask.admin_status_updated_by ? (
                  <p className="task-status-updater">
                    Updated by admin{selectedTask.admin_status_updated_at ? ` · ${formatDate(selectedTask.admin_status_updated_at)}` : ""}
                  </p>
                ) : null}
                <p className="task-detail-description">{selectedTask.description || "No description provided."}</p>
                <dl className="task-detail-facts">
                  <div><dt>Work status</dt><dd>{taskStatusLabels[selectedTask.status]}</dd></div>
                  <div><dt>Administrative status</dt><dd>{statusLabels[selectedTask.admin_status]}</dd></div>
                  <div><dt>Department</dt><dd>{departmentsById.get(selectedTask.department_id) ?? `Department ID ${selectedTask.department_id}`}</dd></div>
                  <div><dt>Assignee</dt><dd>{selectedTask.assignee_account_id ?? `Assignment required · task ID ${selectedTask.id}`}</dd></div>
                  <div><dt>Due</dt><dd>{formatDate(selectedTask.due_at)}</dd></div>
                  <div><dt>Created</dt><dd>{formatDate(selectedTask.created_at)}</dd></div>
                  <div><dt>Version</dt><dd>{selectedTask.version}</dd></div>
                </dl>
                {selectedTask.latest_submission ? (
                  <section className="task-submission">
                    <h3>Latest submission · #{selectedTask.latest_submission.submission_number}</h3>
                    <p>{selectedTask.latest_submission.summary}</p>
                    {selectedTask.latest_submission.links.length ? (
                      <ul>{selectedTask.latest_submission.links.map((link) => <li key={link}><a href={link} target="_blank" rel="noreferrer">{link}</a></li>)}</ul>
                    ) : null}
                  </section>
                ) : null}
              </>
            )}

            {canUpdateStatus ? (
              <section className="task-action-section">
                <h3>Update task status</h3>
                <div className="task-inline-action">
                  <TaskAccordionSelect
                    label="Task status"
                    value={adminStatusDraft}
                    placeholder="Select a status"
                    options={adminTaskStatuses.map((status) => ({ value: status, label: statusLabels[status] }))}
                    onChange={(status) => setAdminStatusDraft(status as AdminTaskStatus)}
                    disabled={saving}
                  />
                  <button
                    type="button"
                    className="button-primary"
                    disabled={saving || adminStatusDraft === selectedTask.admin_status}
                    onClick={() => void saveAdminStatus()}
                  >
                    {saving ? "Updating…" : "Update status"}
                  </button>
                </div>
              </section>
            ) : null}

            {canAssignSelected ? (
              <section className="task-action-section">
                <h3><UserPlus size={17} aria-hidden="true" /> Assign task</h3>
                <div className="task-inline-action">
                  <TaskAccordionSelect
                    label="Assign task to"
                    value={assigneeId}
                    placeholder="Select active department staff"
                    options={[
                      { value: "", label: assigneeLoading ? "Loading eligible staff…" : "Select active department staff" },
                      ...assignees.map((member) => ({
                        value: member.account_id ?? "",
                        label: `${member.full_name || `Staff ID ${member.staff_id}`} · ID ${member.staff_id}`,
                      })),
                    ]}
                    onChange={setAssigneeId}
                    disabled={assigneeLoading || saving}
                  />
                  <button type="button" className="button-secondary" disabled={!assigneeId || saving || assigneeId === selectedTask.assignee_account_id} onClick={() => void saveAssignee()}>
                    {saving ? "Saving…" : "Assign"}
                  </button>
                </div>
              </section>
            ) : null}

            {canStart ? <button type="button" className="button-primary" disabled={saving} onClick={() => void runAction("start")}>Start work</button> : null}
            {canSubmit ? (
              <section className="task-action-section">
                <h3>Submit work</h3>
                <label className="workspace-field"><span className="field-label">Summary</span><textarea className="form-input form-textarea" rows={3} maxLength={20_000} value={submissionSummary} onChange={(event) => setSubmissionSummary(event.target.value)} /></label>
                <label className="workspace-field"><span className="field-label">Evidence links (one HTTPS URL per line, up to 10)</span><textarea className="form-input form-textarea" rows={2} value={evidenceLinks} onChange={(event) => setEvidenceLinks(event.target.value)} /></label>
                <button type="button" className="button-primary" disabled={saving} onClick={() => void runAction("submit")}>{saving ? "Submitting…" : "Submit work"}</button>
              </section>
            ) : null}
            {canDecide ? (
              <section className="task-action-section">
                <h3>Review submission</h3>
                <label className="workspace-field"><span className="field-label">Feedback (required for changes or rejection)</span><textarea className="form-input form-textarea" rows={3} maxLength={2000} value={feedback} onChange={(event) => setFeedback(event.target.value)} /></label>
                <div className="task-review-actions">
                  <button type="button" className="button-primary" disabled={saving} onClick={() => void runAction("approve")}>Approve</button>
                  <button type="button" className="button-secondary" disabled={saving} onClick={() => void runAction("request-changes")}>Request changes</button>
                  <button type="button" className="button-danger" disabled={saving} onClick={() => void runAction("reject")}>Reject</button>
                </div>
              </section>
            ) : null}
            {canCancelSelected ? <button type="button" className="button-danger" disabled={saving} onClick={() => void runAction("cancel")}>Cancel task</button> : null}

            <section className="task-action-section">
              <h3><MessageSquare size={17} aria-hidden="true" /> Comments</h3>
              {commentsLoading ? <p role="status">Loading comments…</p> : comments.length ? (
                <ul className="task-comment-list">
                  {comments.map((comment) => (
                    <li key={comment.id}>
                      <p>{comment.body}</p>
                      <time dateTime={comment.created_at}>{formatDate(comment.created_at)}</time>
                    </li>
                  ))}
                </ul>
              ) : <p className="task-muted">No comments yet.</p>}
              <form className="task-comment-form" onSubmit={(event) => void submitComment(event)}>
                <textarea className="form-input form-textarea" aria-label="Write a comment" rows={2} maxLength={5000} value={commentDraft} onChange={(event) => setCommentDraft(event.target.value)} placeholder="Add a comment…" />
                <button type="submit" className="button-secondary" disabled={saving || !commentDraft.trim()}><Send size={15} aria-hidden="true" /> Comment</button>
              </form>
            </section>
          </div>
        ) : null}
      </TaskDetailsModal>
    </div>
  );
}
