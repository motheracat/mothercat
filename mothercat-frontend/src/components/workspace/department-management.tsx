"use client";

import { useEffect, useState, type FormEvent } from "react";

import {
  createDepartment,
  formatOrganizationError,
  listDepartments,
  updateDepartment,
  type Department,
} from "@/lib/organization/api";
import { DataTable, EmptyState, PageHeader } from "@/components/workspace/ui";

function DepartmentForm({
  department,
  busy,
  error,
  onSave,
  onCancel,
}: {
  department: Department | null;
  busy: boolean;
  error: string | null;
  onSave: (name: string, description: string | null) => Promise<void>;
  onCancel: () => void;
}) {
  const [name, setName] = useState(department?.name ?? "");
  const [description, setDescription] = useState(department?.description ?? "");

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void onSave(name.trim(), description.trim() || null);
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center overflow-y-auto bg-slate-950/40 p-4" role="presentation">
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby="department-form-title"
        className="my-auto w-full max-w-lg rounded-xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-xl"
      >
        <h2 id="department-form-title" className="text-xl font-semibold">{department ? "Edit department" : "Add department"}</h2>
        <p className="mt-1 text-sm text-[var(--muted)]">Use departments to group staff across the organization.</p>
        {error ? <div role="alert" className="error-banner mt-4"><p className="text-sm">{error}</p></div> : null}
        <form className="mt-5 grid gap-4" onSubmit={submit}>
          <label className="block text-sm font-medium">
            Department name
            <input className="form-input mt-1" value={name} onChange={(event) => setName(event.target.value)} maxLength={100} required />
          </label>
          <label className="block text-sm font-medium">
            Description <span className="font-normal text-[var(--muted)]">(optional)</span>
            <textarea
              className="form-input mt-1 !h-24 resize-y py-2"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </label>
          <div className="flex justify-end gap-2">
            <button type="button" className="button-secondary" onClick={onCancel} disabled={busy}>Cancel</button>
            <button type="submit" className="button-primary" disabled={busy}>{busy ? "Saving…" : "Save department"}</button>
          </div>
        </form>
      </section>
    </div>
  );
}

export function DepartmentManagement({ canRead, canManage }: { canRead: boolean; canManage: boolean }) {
  const [departments, setDepartments] = useState<Department[]>([]);
  const [loading, setLoading] = useState(canRead);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [editing, setEditing] = useState<Department | null | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    if (!canRead) return;
    let active = true;
    listDepartments()
      .then((result) => {
        if (active) setDepartments(result);
      })
      .catch((error: unknown) => {
        if (active) setLoadError(formatOrganizationError(error, "Unable to load departments."));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => { active = false; };
  }, [canRead, refreshKey]);

  function refreshDepartments() {
    setLoading(true);
    setLoadError(null);
    setRefreshKey((key) => key + 1);
  }

  async function saveDepartment(name: string, description: string | null) {
    if (!canManage) return;
    setSaving(true);
    setFormError(null);
    setNotice(null);
    try {
      if (editing) {
        const changes: Partial<Pick<Department, "name" | "description">> = {};
        if (name !== editing.name) changes.name = name;
        if (description !== editing.description) changes.description = description;
        if (Object.keys(changes).length > 0) await updateDepartment(editing.id, changes);
        setNotice("Department updated.");
      } else {
        await createDepartment({ name, description });
        setNotice("Department created.");
      }
      setEditing(undefined);
      setLoading(true);
      setRefreshKey((key) => key + 1);
    } catch (error) {
      setFormError(formatOrganizationError(error, "Unable to save this department."));
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <PageHeader
        title="Departments"
        description="Organize teams and assign staff to the right departments."
        actions={canManage ? (
          <button type="button" className="button-primary" onClick={() => { setFormError(null); setEditing(null); }}>
            Add department
          </button>
        ) : null}
      />
      {notice ? <p role="status" className="mb-3 rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800">{notice}</p> : null}
      {!canRead ? (
        <div role="alert" className="error-banner"><p className="text-sm">You do not have permission to view departments.</p></div>
      ) : loadError ? (
        <div role="alert" className="error-banner mb-3">
          <p className="text-sm">{loadError}</p>
          <button type="button" className="button-secondary shrink-0" onClick={refreshDepartments}>Retry</button>
        </div>
      ) : (
        <section className="surface-card">
          <DataTable columns={["Department", "Description", "Actions"]}>
            {loading ? (
              <tr><td colSpan={3} className="!h-auto p-8 text-center text-sm text-[var(--muted)]">Loading departments…</td></tr>
            ) : departments.length === 0 ? (
              <tr>
                <td colSpan={3} className="!h-auto">
                  <EmptyState action={canManage ? <button type="button" className="button-primary" onClick={() => { setFormError(null); setEditing(null); }}>Add department</button> : null}>
                    No departments yet. Create a department to organize staff.
                  </EmptyState>
                </td>
              </tr>
            ) : departments.map((department) => (
              <tr key={department.id}>
                <td className="font-medium">{department.name}</td>
                <td className="max-w-xl whitespace-normal">{department.description || "—"}</td>
                <td>
                  {canManage ? (
                    <button type="button" className="button-ghost !min-h-8 !px-2 text-xs" onClick={() => { setFormError(null); setEditing(department); }}>
                      Edit
                    </button>
                  ) : <span className="text-sm text-[var(--muted)]">—</span>}
                </td>
              </tr>
            ))}
          </DataTable>
        </section>
      )}
      {editing !== undefined ? (
        <DepartmentForm
          key={editing?.id ?? "new-department"}
          department={editing}
          busy={saving}
          error={formError}
          onSave={saveDepartment}
          onCancel={() => { if (!saving) setEditing(undefined); }}
        />
      ) : null}
    </>
  );
}
