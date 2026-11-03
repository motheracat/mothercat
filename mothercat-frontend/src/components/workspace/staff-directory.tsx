"use client";

import Link from "next/link";
import { Search } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { FormEvent } from "react";

import { Dialog } from "@/components/workspace/dialog";
import { EmptyState, ErrorBanner, LoadingTable, PageHeader, StatusPill } from "@/components/workspace/ui";
import { useWorkspace } from "@/components/workspace/workspace-context";
import { hasPermission } from "@/lib/auth/permissions";
import {
  createStaff,
  deactivateStaff,
  formatOrganizationError,
  getStaff,
  listDepartments,
  listStaff,
  provisionStaff,
  updateStaff,
} from "@/lib/organization/api";
import type {
  DepartmentRecord,
  StaffCreatePayload,
  StaffRecord,
  StaffStatus,
  StaffUpdatePayload,
} from "@/lib/organization/api";

const pageSize = 25;
const staffStatuses: StaffStatus[] = ["pending", "active", "inactive", "deactivated"];

function isStaffStatus(value: string): value is StaffStatus {
  return staffStatuses.includes(value as StaffStatus);
}

function getStatusTone(status: StaffStatus) {
  if (status === "active") return "success";
  if (status === "pending") return "warning";
  if (status === "deactivated") return "danger";
  return "neutral";
}

function fieldValue(formData: FormData, name: string) {
  const value = formData.get(name);
  return typeof value === "string" ? value.trim() : "";
}

function StaffEditor({
  staff,
  departments,
  saving,
  error,
  onSubmit,
}: {
  staff?: StaffRecord;
  departments: DepartmentRecord[];
  saving: boolean;
  error: string | null;
  onSubmit: (payload: StaffCreatePayload | StaffUpdatePayload) => void;
}) {
  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const fullName = fieldValue(data, "full_name");
    const email = fieldValue(data, "work_email");
    const title = fieldValue(data, "job_title");
    const departmentId = fieldValue(data, "department_id");

    if (staff) {
      onSubmit({
        full_name: fullName,
        work_email: email || null,
        job_title: title || null,
        department_id: departmentId || null,
      });
      return;
    }

    onSubmit({
      staff_id: fieldValue(data, "staff_id"),
      full_name: fullName,
      work_email: email || null,
      job_title: title || null,
      department_id: departmentId || null,
    });
  }

  return (
    <form className="workspace-form" onSubmit={handleSubmit}>
      {staff ? (
        <p className="text-sm text-[var(--muted)]">Official staff ID: <strong className="font-semibold text-[var(--text)]">ID:{staff.staff_id}</strong></p>
      ) : (
        <label className="workspace-field">
          <span className="field-label">Official staff ID</span>
          <input
            className="form-input"
            name="staff_id"
            inputMode="numeric"
            autoComplete="off"
            pattern="[0-9]{4,5}"
            minLength={4}
            maxLength={5}
            required
            autoFocus
            placeholder="e.g. 23847"
          />
        </label>
      )}
      <label className="workspace-field">
        <span className="field-label">Full name</span>
        <input
          className="form-input"
          name="full_name"
          defaultValue={staff?.full_name}
          maxLength={200}
          required
          autoFocus={Boolean(staff)}
          autoComplete="name"
        />
      </label>
      <label className="workspace-field">
        <span className="field-label">Work email <span className="text-[var(--muted)]">(optional)</span></span>
        <input className="form-input" name="work_email" type="email" maxLength={254} defaultValue={staff?.work_email ?? ""} autoComplete="email" />
      </label>
      <label className="workspace-field">
        <span className="field-label">Job title <span className="text-[var(--muted)]">(optional)</span></span>
        <input className="form-input" name="job_title" maxLength={150} defaultValue={staff?.job_title ?? ""} />
      </label>
      <label className="workspace-field">
        <span className="field-label">Department <span className="text-[var(--muted)]">(optional)</span></span>
        <select className="form-select" name="department_id" defaultValue={staff?.department_id ?? ""}>
          <option value="">Unassigned</option>
          {staff?.department_id && !departments.some((department) => department.id === staff.department_id) ? (
            <option value={staff.department_id}>Current department</option>
          ) : null}
          {departments.map((department) => <option key={department.id} value={department.id}>{department.name}</option>)}
        </select>
      </label>
      {error ? <p role="alert" className="field-error">{error}</p> : null}
      {!staff ? <p className="text-xs leading-5 text-[var(--muted)]">This creates a pending staff profile only. Account setup and role assignment are separate.</p> : null}
      <button type="submit" className="button-primary" disabled={saving}>
        {saving ? "Saving…" : staff ? "Save changes" : "Create staff profile"}
      </button>
    </form>
  );
}

export function StaffDirectory() {
  const { session, devOpen, notify } = useWorkspace();
  const canRead = !devOpen && hasPermission(session, "staff.read");
  const canCreate = !devOpen && hasPermission(session, "staff.create");
  const canUpdate = !devOpen && hasPermission(session, "staff.update");
  const canDeactivate = !devOpen && hasPermission(session, "staff.deactivate");
  const canProvision = !devOpen && hasPermission(session, "user_roles.assign");
  const [staff, setStaff] = useState<StaffRecord[]>([]);
  const [departments, setDepartments] = useState<DepartmentRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [departmentId, setDepartmentId] = useState("");
  const [status, setStatus] = useState<StaffStatus | "">("");
  const [page, setPage] = useState(0);
  const [loadedKey, setLoadedKey] = useState<string | null>(null);
  const [listErrorState, setListErrorState] = useState<{ key: string; message: string } | null>(null);
  const [departmentsError, setDepartmentsError] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [dialogMode, setDialogMode] = useState<"create" | "detail" | "edit" | "deactivate" | "provision" | null>(null);
  const [selectedStaff, setSelectedStaff] = useState<StaffRecord | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const detailRequest = useRef<AbortController | null>(null);

  const departmentsById = useMemo(
    () => new Map(departments.map((department) => [department.id, department.name])),
    [departments],
  );
  const queryKey = `${page}|${debouncedSearch}|${departmentId}|${status}|${refreshKey}`;
  const listError = listErrorState?.key === queryKey ? listErrorState.message : null;
  const loading = canRead && loadedKey !== queryKey;

  useEffect(() => {
    const timeout = window.setTimeout(() => setDebouncedSearch(search.trim()), 250);
    return () => window.clearTimeout(timeout);
  }, [search]);

  useEffect(() => {
    if (!canRead) return;
    const controller = new AbortController();
    void listStaff({
      limit: pageSize,
      offset: page * pageSize,
      ...(debouncedSearch ? { search: debouncedSearch } : {}),
      ...(departmentId ? { department_id: departmentId } : {}),
      ...(status ? { employment_status: status } : {}),
    }, controller.signal)
      .then((result) => {
        setStaff(result.staff);
        setTotal(result.total);
        setListErrorState(null);
        setLoadedKey(queryKey);
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setStaff([]);
        setTotal(0);
        setListErrorState({ key: queryKey, message: formatOrganizationError(error, "Unable to load staff records.") });
        setLoadedKey(queryKey);
      })
    return () => controller.abort();
  }, [canRead, debouncedSearch, departmentId, page, queryKey, refreshKey, status]);

  useEffect(() => {
    if (!canRead) return;
    const controller = new AbortController();
    void listDepartments(controller.signal)
      .then((results) => {
        setDepartments(results);
        setDepartmentsError(null);
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setDepartmentsError(formatOrganizationError(error, "Unable to load departments."));
        }
      });
    return () => controller.abort();
  }, [canRead]);

  const refresh = useCallback(() => setRefreshKey((value) => value + 1), []);

  async function openStaff(staffId: string) {
    detailRequest.current?.abort();
    const controller = new AbortController();
    detailRequest.current = controller;
    setSelectedStaff(null);
    setDetailError(null);
    setDetailLoading(true);
    setFormError(null);
    setDialogMode("detail");
    try {
      const result = await getStaff(staffId, controller.signal);
      if (!controller.signal.aborted) setSelectedStaff(result);
    } catch (error) {
      if (!controller.signal.aborted) {
        setDetailError(formatOrganizationError(error, "Unable to load this staff profile."));
      }
    } finally {
      if (!controller.signal.aborted) setDetailLoading(false);
    }
  }

  function closeDialog() {
    detailRequest.current?.abort();
    setDialogMode(null);
    setSelectedStaff(null);
    setDetailError(null);
    setFormError(null);
  }

  async function saveStaff(payload: StaffCreatePayload | StaffUpdatePayload) {
    setSaving(true);
    setFormError(null);
    try {
      if (dialogMode === "edit" && selectedStaff) {
        if ("staff_id" in payload) throw new Error("A staff update cannot include a staff ID.");
        const result = await updateStaff(selectedStaff.id, payload);
        setSelectedStaff(result);
        setDialogMode("detail");
        notify("Staff profile updated.");
      } else if (dialogMode === "create") {
        if (!("staff_id" in payload)) throw new Error("A new staff profile requires a staff ID.");
        await createStaff(payload);
        closeDialog();
        notify("Staff profile created as pending.");
      }
      refresh();
    } catch (error) {
      const message = formatOrganizationError(error, "Unable to save this staff profile.");
      setFormError(message);
      notify(message, "error");
    } finally {
      setSaving(false);
    }
  }

  async function confirmDeactivation() {
    if (!selectedStaff) return;
    setSaving(true);
    setDetailError(null);
    try {
      const result = await deactivateStaff(selectedStaff.id);
      setSelectedStaff(result);
      setDialogMode("detail");
      notify("Staff employment deactivated.");
      refresh();
    } catch (error) {
      const message = formatOrganizationError(error, "Unable to deactivate this staff profile.");
      setDetailError(message);
      notify(message, "error");
    } finally {
      setSaving(false);
    }
  }

  async function confirmProvisioning() {
    if (!selectedStaff) return;
    setSaving(true);
    setDetailError(null);
    try {
      await provisionStaff(selectedStaff.id);
      setDialogMode("detail");
      notify("Login account provisioned and activation instructions sent.");
      refresh();
    } catch (error) {
      const message = formatOrganizationError(error, "Unable to provision this staff account.");
      setDetailError(message);
      notify(message, "error");
    } finally {
      setSaving(false);
    }
  }

  const dialogTitle = dialogMode === "create"
    ? "Add staff"
    : dialogMode === "edit"
      ? "Edit staff profile"
      : dialogMode === "deactivate"
        ? "Deactivate staff"
        : dialogMode === "provision"
          ? "Provision login account"
        : "Staff profile";

  return (
    <>
      <PageHeader
        title="Staff directory"
        description="Search and manage staff profiles."
        actions={
          <>
            <Link href="/staff/imports" className="button-secondary">Import CSV</Link>
            {canCreate ? <button type="button" className="button-primary" onClick={() => { setFormError(null); setDialogMode("create"); }}>Add staff</button> : null}
          </>
        }
      />

      {devOpen ? <p className="field-error mb-4">Development navigation only. Organization data is not connected in this mode.</p> : null}
      {!canRead && !devOpen ? <p role="alert" className="field-error mb-4">Your account does not have permission to read staff records.</p> : null}

      {canRead ? (
        <>
          {departmentsError ? <p role="alert" className="field-error mb-3">{departmentsError}</p> : null}
          <div className="mb-4 flex flex-wrap items-end gap-3">
            <label className="relative min-w-[220px] flex-1">
              <span className="sr-only">Search staff by number, name, or email</span>
              <Search size={16} aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted)]" />
              <input
                className="form-input pl-9"
                placeholder="Search number, name, or email"
                maxLength={100}
                value={search}
                onChange={(event) => { setSearch(event.target.value); setPage(0); }}
              />
            </label>
            <label className="w-full sm:w-52">
              <span className="sr-only">Filter by department</span>
              <select className="form-select" value={departmentId} onChange={(event) => { setDepartmentId(event.target.value); setPage(0); }}>
                <option value="">All departments</option>
                {departments.map((department) => <option key={department.id} value={department.id}>{department.name}</option>)}
              </select>
            </label>
            <label className="w-full sm:w-44">
              <span className="sr-only">Filter by employment status</span>
              <select className="form-select" value={status} onChange={(event) => {
                const value = event.target.value;
                setStatus(value === "" || isStaffStatus(value) ? value : "");
                setPage(0);
              }}>
                <option value="">All statuses</option>
                <option value="pending">Pending</option>
                <option value="active">Active</option>
                <option value="inactive">Inactive</option>
                <option value="deactivated">Deactivated</option>
              </select>
            </label>
            <span className="ml-auto self-center text-sm text-[var(--muted)]">
              {loading ? <span className="skeleton inline-block w-20 align-middle" aria-hidden="true" /> : `${total} ${total === 1 ? "record" : "records"}`}
            </span>
          </div>

          {listError ? <div className="mb-4"><ErrorBanner message={listError} onRetry={refresh} /></div> : null}
          <section className="surface-card" aria-label="Staff records">
            {loading ? (
              <LoadingTable columns={6} />
            ) : staff.length > 0 ? (
              <div className="max-h-[min(60vh,640px)] overflow-auto">
                <table className="data-table staff-directory-table">
                  <caption className="sr-only">Staff directory records</caption>
                  <thead><tr>
                    <th scope="col" className="h-10">Staff ID</th>
                    <th scope="col" className="h-10">Name</th>
                    <th scope="col" className="h-10">Department</th>
                    <th scope="col" className="h-10">Work email</th>
                    <th scope="col" className="h-10">Status</th>
                    <th scope="col" className="h-10"><span className="sr-only">Actions</span></th>
                  </tr></thead>
                  <tbody>
                    {staff.map((record) => (
                      <tr key={record.id}>
                        <td className="font-medium">ID:{record.staff_id}</td>
                        <td>
                          <span className="block font-medium">{record.full_name}</span>
                          {record.job_title ? <span className="text-xs text-[var(--muted)]">{record.job_title}</span> : null}
                        </td>
                        <td>{record.department_id ? departmentsById.get(record.department_id) ?? "Department" : "Unassigned"}</td>
                        <td>{record.work_email ?? "—"}</td>
                        <td><StatusPill label={record.employment_status} tone={getStatusTone(record.employment_status)} /></td>
                        <td><button type="button" className="button-secondary" onClick={() => void openStaff(record.id)}>View</button></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <EmptyState action={canCreate ? <button type="button" className="button-primary" onClick={() => setDialogMode("create")}>Add staff</button> : null}>
                {total === 0 && !debouncedSearch && !departmentId && !status ? "No staff records yet." : "No staff records match these filters."}
              </EmptyState>
            )}
          </section>

          <nav aria-label="Staff pages" className="mt-4 flex items-center justify-between gap-4">
            <p className="text-sm text-[var(--muted)]">
              {total === 0 ? "No records" : `Showing ${page * pageSize + 1}–${Math.min((page + 1) * pageSize, total)} of ${total}`}
            </p>
            <div className="flex gap-2">
              <button type="button" className="button-secondary" disabled={page === 0 || loading} onClick={() => setPage((value) => value - 1)}>Previous</button>
              <button type="button" className="button-secondary" disabled={(page + 1) * pageSize >= total || loading} onClick={() => setPage((value) => value + 1)}>Next</button>
            </div>
          </nav>
        </>
      ) : null}

      <Dialog
        open={dialogMode !== null}
        title={dialogTitle}
        onClose={closeDialog}
        actions={dialogMode === "detail" && selectedStaff ? (
          <>
            {canUpdate ? <button type="button" className="button-secondary" onClick={() => { setFormError(null); setDialogMode("edit"); }}>Edit</button> : null}
            {canDeactivate && selectedStaff.employment_status !== "inactive" && selectedStaff.employment_status !== "deactivated" ? (
              <button type="button" className="button-danger" onClick={() => setDialogMode("deactivate")}>Deactivate</button>
            ) : null}
            {canProvision && selectedStaff.employment_status === "pending" && !selectedStaff.account_id ? (
              <button
                type="button"
                className="button-primary"
                disabled={!selectedStaff.work_email}
                title={!selectedStaff.work_email ? "Add a work email before provisioning" : undefined}
                onClick={() => { setDetailError(null); setDialogMode("provision"); }}
              >
                Provision account
              </button>
            ) : null}
            <button type="button" className="button-secondary" onClick={closeDialog}>Done</button>
          </>
        ) : null}
      >
        {dialogMode === "create" || dialogMode === "edit" ? (
          <StaffEditor
            staff={dialogMode === "edit" ? selectedStaff ?? undefined : undefined}
            departments={departments}
            saving={saving}
            error={formError}
            onSubmit={(payload) => void saveStaff(payload)}
          />
        ) : null}
        {dialogMode === "detail" ? (
          detailLoading ? (
            <div role="status" aria-label="Loading profile" aria-busy="true" className="space-y-4 py-2">
              <div className="skeleton h-6 w-1/2" />
              <div className="skeleton w-4/5" />
              <div className="skeleton w-2/3" />
            </div>
          ) : null
        ) : null}
        {dialogMode === "detail" && detailError ? <p role="alert" className="field-error">{detailError}</p> : null}
        {dialogMode === "detail" && selectedStaff ? (
          <dl className="workspace-detail-list">
            <div><dt>Official staff ID</dt><dd>ID:{selectedStaff.staff_id}</dd></div>
            <div><dt>Name</dt><dd>{selectedStaff.full_name}</dd></div>
            <div><dt>Department</dt><dd>{selectedStaff.department_id ? departmentsById.get(selectedStaff.department_id) ?? "Department" : "Unassigned"}</dd></div>
            <div><dt>Work email</dt><dd>{selectedStaff.work_email ?? "—"}</dd></div>
            <div><dt>Job title</dt><dd>{selectedStaff.job_title ?? "—"}</dd></div>
            <div><dt>Employment status</dt><dd><StatusPill label={selectedStaff.employment_status} tone={getStatusTone(selectedStaff.employment_status)} /></dd></div>
          </dl>
        ) : null}
        {dialogMode === "deactivate" ? (
          <div className="space-y-4">
            <p>Deactivate <strong>{selectedStaff?.full_name}</strong>? This ends their employment and may suspend their login account and revoke existing sessions.</p>
            {detailError ? <p role="alert" className="field-error">{detailError}</p> : null}
            <div className="flex justify-end gap-2">
              <button type="button" className="button-secondary" onClick={() => { setDetailError(null); setDialogMode("detail"); }}>Cancel</button>
              <button type="button" className="button-danger" disabled={saving} onClick={() => void confirmDeactivation()}>{saving ? "Deactivating…" : "Confirm deactivation"}</button>
            </div>
          </div>
        ) : null}
        {dialogMode === "provision" ? (
          <div className="space-y-4">
            <p>
              Provision a login for <strong>{selectedStaff?.full_name}</strong> at{" "}
              <strong>{selectedStaff?.work_email}</strong>? This creates an inactive account,
              assigns the permitted staff role, and sends activation instructions.
            </p>
            {detailError ? <p role="alert" className="field-error">{detailError}</p> : null}
            <div className="flex justify-end gap-2">
              <button type="button" className="button-secondary" disabled={saving} onClick={() => { setDetailError(null); setDialogMode("detail"); }}>Cancel</button>
              <button type="button" className="button-primary" disabled={saving} onClick={() => void confirmProvisioning()}>
                {saving ? "Provisioning…" : "Confirm provisioning"}
              </button>
            </div>
          </div>
        ) : null}
      </Dialog>
    </>
  );
}
