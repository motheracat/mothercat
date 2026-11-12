"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";

import { TaskAccordionSelect } from "@/components/workspace/task-accordion-select";
import { DepartmentSelect } from "@/components/workspace/department-select";
import { ErrorBanner } from "@/components/workspace/ui";
import {
  formatOrganizationError,
  listDepartments,
  listStaff,
  type Department,
  type StaffMember,
} from "@/lib/organization/api";
import {
  createTask,
  formatTaskError,
  type Task,
  type TaskPriority,
} from "@/lib/tasks/api";

type Props = {
  canCreate: boolean;
  canAssign: boolean;
  previewMode: boolean;
};

const priorities: TaskPriority[] = ["low", "normal", "high", "urgent"];

export function CreateTaskForm({ canCreate, canAssign, previewMode }: Props) {
  const router = useRouter();
  const [departments, setDepartments] = useState<Department[]>([]);
  const [departmentError, setDepartmentError] = useState<string | null>(null);
  const [departmentsLoading, setDepartmentsLoading] = useState(true);
  const [selectedDepartment, setSelectedDepartment] = useState("");
  const [priority, setPriority] = useState<TaskPriority>("normal");
  const [eligibleStaff, setEligibleStaff] = useState<StaffMember[]>([]);
  const [staffLoading, setStaffLoading] = useState(false);
  const [staffError, setStaffError] = useState<string | null>(null);
  const [staffReloadKey, setStaffReloadKey] = useState(0);
  const [assigneeAccountId, setAssigneeAccountId] = useState("");
  const [creating, setCreating] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [createdTask, setCreatedTask] = useState<Task | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    void listDepartments(controller.signal)
      .then((result) => {
        setDepartments(result);
        setDepartmentError(null);
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setDepartmentError(formatOrganizationError(error, "Unable to load departments."));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setDepartmentsLoading(false);
      });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    if (!selectedDepartment || !canAssign) return;
    const controller = new AbortController();
    void (async () => {
      try {
        const allStaff: StaffMember[] = [];
        let offset = 0;
        let total = 0;
        do {
          const result = await listStaff({
            limit: 100,
            offset,
            departmentId: selectedDepartment,
            employmentStatus: "active",
          }, controller.signal);
          allStaff.push(...result.staff);
          total = result.total;
          offset += result.pagination.limit;
        } while (offset < total && !controller.signal.aborted);
        if (!controller.signal.aborted) {
          setEligibleStaff(allStaff.filter((member) => member.account_id && member.employment_status === "active"));
          setStaffError(null);
        }
      } catch (error: unknown) {
        if (!controller.signal.aborted) {
          setEligibleStaff([]);
          setStaffError(formatOrganizationError(error, "Unable to load eligible staff for this department."));
        }
      } finally {
        if (!controller.signal.aborted) setStaffLoading(false);
      }
    })();
    return () => controller.abort();
  }, [selectedDepartment, canAssign, staffReloadKey]);

  function navigateToTasks() {
    router.push("/tasks");
    router.refresh();
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canCreate || !canAssign || !assigneeAccountId || createdTask) return;
    const data = new FormData(event.currentTarget);
    const dueValue = String(data.get("due_at") ?? "");
    setCreating(true);
    setFormError(null);
    try {
      const result = await createTask({
        title: String(data.get("title") ?? "").trim(),
        description: String(data.get("description") ?? "").trim(),
        priority: String(data.get("priority") ?? "normal") as TaskPriority,
        department_id: String(data.get("department_id") ?? ""),
        assignee_account_id: assigneeAccountId,
        due_at: dueValue ? new Date(dueValue).toISOString() : null,
      });
      setCreatedTask(result.task);
      navigateToTasks();
    } catch (error) {
      setFormError(formatTaskError(error, "Unable to create the task."));
    } finally {
      setCreating(false);
    }
  }

  return (
    <div className="task-management-page">
      {!canCreate ? <p role="alert" className="field-error">You don’t have permission to create tasks.</p> : null}
      {canCreate && !canAssign ? (
        <p role="alert" className="field-error">Task creation requires permission to assign it to an eligible staff member.</p>
      ) : null}

      {departmentError ? <ErrorBanner message={departmentError} onRetry={() => {
        setDepartmentsLoading(true);
        setDepartmentError(null);
        void listDepartments().then(setDepartments).catch((error: unknown) => {
          setDepartmentError(formatOrganizationError(error, "Unable to load departments."));
        }).finally(() => setDepartmentsLoading(false));
      }} /> : null}
      {formError ? <p role="alert" className="field-error">{formError}</p> : null}
      {createdTask ? (
        <p role="status" className="access-notice">Task created successfully. Opening the task list…</p>
      ) : null}

      <section className="surface-card create-task-card">
        <form className="task-form" onSubmit={(event) => void submit(event)}>
          <label className="workspace-field">
            <span className="field-label">Task title</span>
            <input className="form-input" name="title" maxLength={200} required disabled={!canCreate || creating || Boolean(createdTask)} />
          </label>
          <label className="workspace-field">
            <span className="field-label">Description</span>
            <textarea className="form-input form-textarea" name="description" maxLength={20_000} rows={5} disabled={!canCreate || creating || Boolean(createdTask)} />
          </label>
          <div className="task-form-grid">
            <label className="workspace-field">
              <span className="field-label">Department</span>
              <DepartmentSelect
                label="Department"
                name="department_id"
                value={selectedDepartment}
                departments={departments}
                placeholder={departmentsLoading ? "Loading departments…" : "Select a department"}
                emptyMessage={departmentError ? "Departments could not be loaded. Use the retry action above." : "No departments are available."}
                onChange={(departmentId) => {
                  setSelectedDepartment(departmentId);
                  setEligibleStaff([]);
                  setAssigneeAccountId("");
                  setStaffError(null);
                  setStaffLoading(Boolean(departmentId) && canAssign);
                }}
                disabled={(!canCreate && !previewMode) || departmentsLoading || creating || Boolean(createdTask)}
              />
            </label>
          <label className="workspace-field">
            <span className="field-label">Priority</span>
            <TaskAccordionSelect
              label="Task priority"
              name="priority"
              value={priority}
              placeholder="Select priority"
              options={priorities.map((option) => ({
                value: option,
                label: option.charAt(0).toUpperCase() + option.slice(1),
              }))}
              onChange={(value) => setPriority(value as TaskPriority)}
              disabled={!canCreate || creating || Boolean(createdTask)}
            />
          </label>
            <label className="workspace-field">
              <span className="field-label">Due date</span>
              <input className="form-input" name="due_at" type="datetime-local" disabled={!canCreate || creating || Boolean(createdTask)} />
            </label>
            {canAssign ? (
              <label className="workspace-field">
                <span className="field-label">Assign to staff or Admin</span>
                <TaskAccordionSelect
                  label="Assign to staff or Admin"
                  value={assigneeAccountId}
                  placeholder="Select an assignee"
                  options={[
                    {
                      value: "",
                      label: !selectedDepartment ? "Select a department first" : staffLoading ? "Loading eligible staff…" : "Select an assignee",
                    },
                    ...eligibleStaff.map((member) => ({
                      value: member.account_id ?? "",
                      label: `${member.full_name} · ${member.job_title || `ID ${member.staff_id}`}`,
                    })),
                  ]}
                  onChange={setAssigneeAccountId}
                  disabled={!selectedDepartment || staffLoading || Boolean(staffError) || creating || Boolean(createdTask)}
                />
              </label>
            ) : null}
            {canAssign ? <p className="task-muted create-task-assignee-hint">An assignee is required. Only active, account-linked members of the selected department are eligible.</p> : null}
          </div>
          {staffError ? (
            <div className="field-error" role="alert">
              <p>{staffError}</p>
              <button type="button" className="button-secondary" disabled={staffLoading} onClick={() => {
                setStaffLoading(true);
                setStaffError(null);
                setStaffReloadKey((key) => key + 1);
              }}>Retry loading assignees</button>
            </div>
          ) : null}
          {selectedDepartment && canAssign && !staffLoading && !staffError && eligibleStaff.length === 0 ? (
            <p className="field-error" role="alert">No active account-linked staff are available in this department. Add an eligible staff member before creating a task.</p>
          ) : null}
          <div className="create-task-actions">
            <Link href="/tasks" className="button-secondary">Cancel</Link>
            <button type="submit" className="button-primary" disabled={!canCreate || !canAssign || creating || Boolean(createdTask) || departmentsLoading || Boolean(departmentError) || !departments.length || !selectedDepartment || staffLoading || Boolean(staffError) || !assigneeAccountId}>
              {creating ? "Creating task…" : "Create task"}
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}
