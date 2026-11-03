"use client";

import { Pencil } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import type { FormEvent } from "react";

import { Dialog } from "@/components/workspace/dialog";
import { EmptyState, ErrorBanner, PageHeader } from "@/components/workspace/ui";
import { useWorkspace } from "@/components/workspace/workspace-context";
import { hasPermission } from "@/lib/auth/permissions";
import {
  createDepartment,
  formatOrganizationError,
  listDepartments,
  updateDepartment,
} from "@/lib/organization/api";
import type { DepartmentPayload, DepartmentRecord } from "@/lib/organization/api";

function DepartmentEditor({
  department,
  saving,
  error,
  onSubmit,
}: {
  department?: DepartmentRecord;
  saving: boolean;
  error: string | null;
  onSubmit: (payload: DepartmentPayload) => void;
}) {
  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const nameValue = data.get("name");
    const descriptionValue = data.get("description");
    onSubmit({
      name: typeof nameValue === "string" ? nameValue.trim() : "",
      description: typeof descriptionValue === "string" && descriptionValue.trim()
        ? descriptionValue.trim()
        : null,
    });
  }

  return (
    <form className="workspace-form" onSubmit={handleSubmit}>
      <label className="workspace-field">
        <span className="field-label">Department name</span>
        <input
          className="form-input"
          name="name"
          defaultValue={department?.name ?? ""}
          maxLength={160}
          required
          autoFocus
        />
      </label>
      <label className="workspace-field">
        <span className="field-label">Description <span className="text-[var(--muted)]">(optional)</span></span>
        <textarea
          className="form-input form-textarea"
          name="description"
          defaultValue={department?.description ?? ""}
          maxLength={2000}
          rows={4}
        />
      </label>
      {error ? <p role="alert" className="field-error">{error}</p> : null}
      <button type="submit" className="button-primary" disabled={saving}>
        {saving ? "Saving…" : department ? "Save changes" : "Create department"}
      </button>
    </form>
  );
}

export function DepartmentDirectory() {
  const { session, devOpen, notify } = useWorkspace();
  const canRead = !devOpen && hasPermission(session, "staff.read");
  const canManage = !devOpen && hasPermission(session, "departments.manage");
  const [departments, setDepartments] = useState<DepartmentRecord[]>([]);
  const [loadState, setLoadState] = useState<{ key: number; error: string | null } | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<DepartmentRecord | null>(null);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const loading = canRead && loadState?.key !== refreshKey;
  const listError = loadState?.key === refreshKey ? loadState.error : null;

  useEffect(() => {
    if (!canRead) return;
    const controller = new AbortController();
    void listDepartments(controller.signal)
      .then((results) => {
        setDepartments(results);
        setLoadState({ key: refreshKey, error: null });
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setLoadState({ key: refreshKey, error: formatOrganizationError(error, "Unable to load departments.") });
        }
      });
    return () => controller.abort();
  }, [canRead, refreshKey]);

  const refresh = useCallback(() => setRefreshKey((value) => value + 1), []);

  function openCreate() {
    setEditing(null);
    setFormError(null);
    setDialogOpen(true);
  }

  function openEdit(department: DepartmentRecord) {
    setEditing(department);
    setFormError(null);
    setDialogOpen(true);
  }

  async function saveDepartment(payload: DepartmentPayload) {
    setSaving(true);
    setFormError(null);
    try {
      if (editing) {
        await updateDepartment(editing.id, payload);
        notify("Department updated.");
      } else {
        await createDepartment(payload);
        notify("Department created.");
      }
      setDialogOpen(false);
      setEditing(null);
      refresh();
    } catch (error) {
      const message = formatOrganizationError(error, "Unable to save this department.");
      setFormError(message);
      notify(message, "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <PageHeader
        title="Departments"
        description="Organize teams and department ownership."
        actions={canManage ? <button type="button" className="button-primary" onClick={openCreate}>Add department</button> : null}
      />
      {devOpen ? <p className="field-error mb-4">Development navigation only. Organization data is not connected in this mode.</p> : null}
      {!canRead && !devOpen ? <p role="alert" className="field-error mb-4">Your account does not have permission to read departments.</p> : null}
      {canRead ? (
        <>
          {listError ? <div className="mb-4"><ErrorBanner message={listError} onRetry={refresh} /></div> : null}
          <section className="surface-card" aria-label="Departments">
            {loading ? (
              <div className="p-5 text-sm text-[var(--muted)]" role="status" aria-live="polite">Loading departments…</div>
            ) : departments.length > 0 ? (
              <div className="max-h-[min(60vh,640px)] overflow-auto">
                <table className="data-table">
                  <caption className="sr-only">Departments and descriptions</caption>
                  <thead><tr>
                    <th scope="col" className="h-10">Name</th>
                    <th scope="col" className="h-10">Description</th>
                    {canManage ? <th scope="col" className="h-10"><span className="sr-only">Actions</span></th> : null}
                  </tr></thead>
                  <tbody>
                    {departments.map((department) => (
                      <tr key={department.id}>
                        <td className="font-medium">{department.name}</td>
                        <td className="max-w-[640px] whitespace-normal">{department.description || "—"}</td>
                        {canManage ? (
                          <td>
                            <button type="button" className="button-secondary" onClick={() => openEdit(department)}>
                              <Pencil size={15} aria-hidden="true" />
                              Edit
                            </button>
                          </td>
                        ) : null}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <EmptyState action={canManage ? <button type="button" className="button-primary" onClick={openCreate}>Add department</button> : null}>
                No departments yet.
              </EmptyState>
            )}
          </section>
          <p className="mt-3 text-sm text-[var(--muted)]">{loading ? " " : `${departments.length} ${departments.length === 1 ? "department" : "departments"}`}</p>
        </>
      ) : null}

      <Dialog
        open={dialogOpen}
        title={editing ? "Edit department" : "Add department"}
        onClose={() => {
          setDialogOpen(false);
          setEditing(null);
          setFormError(null);
        }}
      >
        <DepartmentEditor
          key={editing?.id ?? "new-department"}
          department={editing ?? undefined}
          saving={saving}
          error={formError}
          onSubmit={(payload) => void saveDepartment(payload)}
        />
      </Dialog>
    </>
  );
}
