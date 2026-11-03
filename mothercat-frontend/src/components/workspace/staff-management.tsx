"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import { EllipsisVertical, Search, UserPlus, X } from "lucide-react";
import Link from "next/link";
import Image from "next/image";
import { createPortal } from "react-dom";

import {
  createStaff,
  deactivateStaff,
  formatOrganizationError,
  getStaff,
  listDepartments,
  listStaff,
  provisionStaff,
  updateStaff,
  type Department,
  type EmploymentStatus,
  type StaffMember,
} from "@/lib/organization/api";
import { EmptyState, PageHeader } from "@/components/workspace/ui";

const pageSize = 25;

type Permissions = {
  canRead: boolean;
  canCreate: boolean;
  canUpdate: boolean;
  canDeactivate: boolean;
  canImport: boolean;
  canProvision: boolean;
};

function getInitials(name: string) {
  return name.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? "").join("") || "S";
}

function StaffAvatar({ staff }: { staff: StaffMember }) {
  const [imageFailed, setImageFailed] = useState(false);
  const imageUrl = staff.profile_image_url;
  const canShowImage = imageUrl && (imageUrl.startsWith("/") || /^https?:\/\//i.test(imageUrl));

  return (
    <span className="staff-avatar-wrap" aria-hidden="true">
      {canShowImage && !imageFailed ? (
        <Image
          src={imageUrl}
          alt=""
          fill
          sizes="48px"
          unoptimized
          className="dashboard-team-photo"
          onError={() => setImageFailed(true)}
        />
      ) : (
        <span className="staff-avatar">{getInitials(staff.full_name)}</span>
      )}
      <span className={`staff-avatar-status ${staff.employment_status === "active" ? "is-active" : "is-inactive"}`} />
    </span>
  );
}

function StaffForm({
  staff,
  departments,
  busy,
  error,
  onSave,
  onCancel,
}: {
  staff: StaffMember | null;
  departments: Department[];
  busy: boolean;
  error: string | null;
  onSave: (input: {
    staff_id: string;
    full_name: string;
    work_email: string | null;
    job_title: string | null;
    department_id: string | null;
  }) => Promise<void>;
  onCancel: () => void;
}) {
  const [staffId, setStaffId] = useState(staff?.staff_id ?? "");
  const [fullName, setFullName] = useState(staff?.full_name ?? "");
  const [email, setEmail] = useState(staff?.work_email ?? "");
  const [jobTitle, setJobTitle] = useState(staff?.job_title ?? "");
  const [departmentId, setDepartmentId] = useState(staff?.department_id ?? "");

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void onSave({
      staff_id: staffId.trim(),
      full_name: fullName.trim(),
      work_email: email.trim() || null,
      job_title: jobTitle.trim() || null,
      department_id: departmentId || null,
    });
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center overflow-y-auto bg-slate-950/40 p-4" role="presentation">
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby="staff-form-title"
        className="my-auto w-full max-w-xl rounded-xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-xl"
      >
        <h2 id="staff-form-title" className="text-xl font-semibold">
          {staff ? "Edit staff profile" : "Add staff member"}
        </h2>
        <p className="mt-1 text-sm text-[var(--muted)]">
          {staff ? "Update profile details. The staff number and employment status cannot be changed." : "New profiles begin with pending employment status."}
        </p>
        {error ? <div role="alert" className="error-banner mt-4"><p className="text-sm">{error}</p></div> : null}
        <form className="mt-5 grid gap-4 sm:grid-cols-2" onSubmit={submit}>
          <label className="block text-sm font-medium sm:col-span-2">
            Official staff number
            <input
              className="form-input mt-1"
              value={staffId}
              onChange={(event) => setStaffId(event.target.value)}
              inputMode="numeric"
              pattern="[0-9]{4,5}"
              maxLength={5}
              required
              disabled={Boolean(staff)}
              aria-describedby="staff-id-hint"
            />
            <span id="staff-id-hint" className="mt-1 block text-xs font-normal text-[var(--muted)]">
              Enter 4 or 5 digits. The ID: prefix is only shown in the directory.
            </span>
          </label>
          <label className="block text-sm font-medium sm:col-span-2">
            Full name
            <input className="form-input mt-1" value={fullName} onChange={(event) => setFullName(event.target.value)} required />
          </label>
          <label className="block text-sm font-medium">
            Work email
            <input className="form-input mt-1" type="email" value={email} onChange={(event) => setEmail(event.target.value)} />
          </label>
          <label className="block text-sm font-medium">
            Job title
            <input className="form-input mt-1" value={jobTitle} onChange={(event) => setJobTitle(event.target.value)} />
          </label>
          <label className="block text-sm font-medium sm:col-span-2">
            Department
            <select className="form-select mt-1" value={departmentId} onChange={(event) => setDepartmentId(event.target.value)}>
              <option value="">No department</option>
              {departments.map((department) => <option key={department.id} value={department.id}>{department.name}</option>)}
            </select>
          </label>
          <div className="flex justify-end gap-2 sm:col-span-2">
            <button type="button" className="button-secondary" onClick={onCancel} disabled={busy}>Cancel</button>
            <button type="submit" className="button-primary" disabled={busy}>
              {busy ? "Saving…" : staff ? "Save changes" : "Create profile"}
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}

export function StaffManagement({ permissions }: { permissions: Permissions }) {
  const [staff, setStaff] = useState<StaffMember[]>([]);
  const [departments, setDepartments] = useState<Department[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [searchDraft, setSearchDraft] = useState("");
  const [search, setSearch] = useState("");
  const [departmentId, setDepartmentId] = useState("");
  const [status, setStatus] = useState<EmploymentStatus | "">("");
  const [refreshKey, setRefreshKey] = useState(0);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [editing, setEditing] = useState<StaffMember | null | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [rowBusyId, setRowBusyId] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [menuStaffId, setMenuStaffId] = useState<string | null>(null);
  const [menuPosition, setMenuPosition] = useState<{ top: number; left: number } | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuTriggerRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!permissions.canRead) return;
    let active = true;
    listDepartments()
      .then((result) => {
        if (active) setDepartments(result);
      })
      .catch((error: unknown) => {
        if (active) setLoadError(formatOrganizationError(error, "Unable to load departments."));
      });
    return () => { active = false; };
  }, [permissions.canRead, refreshKey]);

  useEffect(() => {
    if (!permissions.canRead) return;
    let active = true;
    listStaff({ limit: pageSize, offset, search, departmentId, employmentStatus: status })
      .then((result) => {
        if (!active) return;
        setStaff(result.staff);
        setTotal(result.total);
      })
      .catch((error: unknown) => {
        if (active) setLoadError(formatOrganizationError(error, "Unable to load the staff directory."));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => { active = false; };
  }, [permissions.canRead, offset, search, departmentId, status, refreshKey]);

  useEffect(() => {
    if (!menuStaffId) return;

    const previousOverflow = document.body.style.overflow;
    const previouslyFocused = menuTriggerRef.current ??
      (document.activeElement instanceof HTMLElement ? document.activeElement : null);
    document.body.style.overflow = "hidden";
    const firstItem =
      menuRef.current?.querySelector<HTMLElement>("[data-menu-first]") ??
      menuRef.current?.querySelector<HTMLElement>("button:not([disabled]), a[href]");
    firstItem?.focus();

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        setMenuStaffId(null);
        return;
      }

      if (event.key !== "Tab" || !menuRef.current) return;
      const focusable = Array.from(
        menuRef.current.querySelectorAll<HTMLElement>("button:not([disabled]), a[href]"),
      );
      if (focusable.length === 0) return;

      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && (document.activeElement === first || !menuRef.current.contains(document.activeElement))) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && (document.activeElement === last || !menuRef.current.contains(document.activeElement))) {
        event.preventDefault();
        first.focus();
      }
    }

    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      document.body.style.overflow = previousOverflow;
      if (previouslyFocused?.isConnected) previouslyFocused.focus();
    };
  }, [menuStaffId]);

  const pageNumber = Math.floor(offset / pageSize) + 1;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));

  function refreshDirectory() {
    setLoading(true);
    setLoadError(null);
    setRefreshKey((key) => key + 1);
  }

  function openActions(member: StaffMember, trigger: HTMLButtonElement) {
    const bounds = trigger.getBoundingClientRect();
    const optionCount =
      1 +
      Number(permissions.canUpdate) +
      Number(permissions.canProvision && member.employment_status === "pending" && !member.account_id && Boolean(member.work_email)) +
      Number(permissions.canDeactivate && member.employment_status !== "inactive" && member.employment_status !== "deactivated");
    const menuHeight = optionCount * 43 + 54;
    const menuWidth = 192;
    const topBelow = bounds.bottom + 8;
    const top = topBelow + menuHeight <= window.innerHeight - 12
      ? topBelow
      : Math.max(12, bounds.top - menuHeight - 8);
    const left = Math.max(12, Math.min(bounds.right - menuWidth, window.innerWidth - menuWidth - 12));

    menuTriggerRef.current = trigger;
    setMenuPosition({ top, left });
    setMenuStaffId(member.id);
  }

  async function saveStaff(input: {
    staff_id: string;
    full_name: string;
    work_email: string | null;
    job_title: string | null;
    department_id: string | null;
  }) {
    if (!permissions.canCreate && !permissions.canUpdate) return;
    setSaving(true);
    setFormError(null);
    setNotice(null);
    try {
      if (editing) {
        const changes: Partial<Pick<StaffMember, "full_name" | "work_email" | "job_title" | "department_id">> = {};
        if (input.full_name !== editing.full_name) changes.full_name = input.full_name;
        if (input.work_email !== editing.work_email) changes.work_email = input.work_email;
        if (input.job_title !== editing.job_title) changes.job_title = input.job_title;
        if (input.department_id !== editing.department_id) changes.department_id = input.department_id;
        if (Object.keys(changes).length > 0) await updateStaff(editing.id, changes);
        setNotice("Staff profile updated.");
      } else {
        await createStaff(input);
        setNotice("Staff profile created.");
      }
      setEditing(undefined);
      setLoading(true);
      setRefreshKey((key) => key + 1);
    } catch (error) {
      setFormError(formatOrganizationError(error, "Unable to save this staff profile."));
    } finally {
      setSaving(false);
    }
  }

  async function handleDeactivate(member: StaffMember) {
    if (!window.confirm(`Deactivate ${member.full_name}? This cannot be undone from this screen.`)) return;
    setRowBusyId(member.id);
    setActionError(null);
    setNotice(null);
    try {
      await deactivateStaff(member.id);
      setNotice(`${member.full_name} has been deactivated.`);
      setLoading(true);
      setRefreshKey((key) => key + 1);
    } catch (error) {
      setActionError(formatOrganizationError(error, "Unable to deactivate this staff profile."));
    } finally {
      setRowBusyId(null);
    }
  }

  async function handleProvision(member: StaffMember) {
    if (!window.confirm(`Send an activation invitation to ${member.work_email}?`)) return;
    setRowBusyId(member.id);
    setActionError(null);
    setNotice(null);
    try {
      await provisionStaff(member.id);
      setNotice(`An activation invitation was sent to ${member.work_email}.`);
      setLoading(true);
      setRefreshKey((key) => key + 1);
    } catch (error) {
      setActionError(formatOrganizationError(error, "Unable to send an activation invitation."));
    } finally {
      setRowBusyId(null);
    }
  }

  async function handleEdit(member: StaffMember) {
    setRowBusyId(member.id);
    setActionError(null);
    try {
      setEditing(await getStaff(member.id));
      setFormError(null);
    } catch (error) {
      setActionError(formatOrganizationError(error, "Unable to load this staff profile."));
    } finally {
      setRowBusyId(null);
    }
  }

  if (!permissions.canRead) {
    return (
      <>
        <PageHeader title="Staff directory" description="View and manage staff records." />
        <div role="alert" className="error-banner"><p className="text-sm">You do not have permission to view staff records.</p></div>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="Staff directory"
        description="Manage staff profiles, employment details, and department assignments."
        actions={
          <>
            {permissions.canImport ? <Link href="/staff/imports" className="button-secondary">Import CSV</Link> : null}
            {permissions.canCreate ? (
              <button type="button" className="button-primary" onClick={() => { setFormError(null); setEditing(null); }}>
                <UserPlus size={16} aria-hidden="true" /> Add staff
              </button>
            ) : null}
          </>
        }
      />

      <form
        className="staff-directory-toolbar mb-4"
        onSubmit={(event) => {
          event.preventDefault();
          refreshDirectory();
          setOffset(0);
          setSearch(searchDraft.trim().slice(0, 100));
        }}
      >
        <div className="staff-search-field">
          <label htmlFor="staff-directory-search" className="sr-only">Search staff by number, name, or email</label>
          <Search size={17} aria-hidden="true" className="staff-search-icon" />
          <input
            id="staff-directory-search"
            className="staff-search-input"
            placeholder="Search number, name, or email"
            value={searchDraft}
            maxLength={100}
            onChange={(event) => setSearchDraft(event.target.value)}
          />
          <button type="submit" className="staff-search-submit">Search</button>
        </div>
        <label className="w-full sm:w-48">
          <span className="sr-only">Filter by department</span>
          <select className="form-select" value={departmentId} onChange={(event) => { setLoading(true); setLoadError(null); setDepartmentId(event.target.value); setOffset(0); }}>
            <option value="">All departments</option>
            {departments.map((department) => <option key={department.id} value={department.id}>{department.name}</option>)}
          </select>
        </label>
        <label className="w-full sm:w-40">
          <span className="sr-only">Filter by employment status</span>
          <select className="form-select" value={status} onChange={(event) => { setLoading(true); setLoadError(null); setStatus(event.target.value as EmploymentStatus | ""); setOffset(0); }}>
            <option value="">All statuses</option>
            <option value="pending">Pending</option>
            <option value="active">Active</option>
            <option value="inactive">Inactive</option>
            <option value="deactivated">Deactivated</option>
          </select>
        </label>
        <span className="ml-auto self-center text-sm text-[var(--muted)]" aria-live="polite">
          {total} {total === 1 ? "staff member" : "staff members"}
        </span>
      </form>

      {notice ? <p role="status" className="mb-3 rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800">{notice}</p> : null}
      {actionError ? <div role="alert" className="error-banner mb-3"><p className="text-sm">{actionError}</p></div> : null}
      {loadError ? (
        <div role="alert" className="error-banner mb-3">
          <p className="text-sm">{loadError}</p>
          <button type="button" className="button-secondary shrink-0" onClick={refreshDirectory}>Retry</button>
        </div>
      ) : null}

      <section className="surface-card staff-directory-card">
        {loading ? (
          <p className="p-8 text-center text-sm text-[var(--muted)]" role="status">Loading staff…</p>
        ) : staff.length === 0 ? (
          <EmptyState
            action={
              <>
                {permissions.canCreate ? <button type="button" className="button-primary" onClick={() => { setFormError(null); setEditing(null); }}>Add staff</button> : null}
                {permissions.canImport ? <Link href="/staff/imports" className="button-secondary">Import CSV</Link> : null}
              </>
            }
          >
            {search || departmentId || status ? "No staff match these filters." : "No staff yet. Add a profile or import a CSV."}
          </EmptyState>
        ) : (
          <ul className="staff-directory-list">
            {staff.map((member) => (
              <li key={member.id} className="staff-directory-item">
                <Link href={`/staff/${encodeURIComponent(member.id)}`} className="staff-directory-profile">
                  <StaffAvatar staff={member} />
                  <span className="staff-directory-identity">
                    <span className="staff-directory-name">{member.full_name}</span>
                    <span className="staff-directory-meta">
                      ID:{member.staff_id}{member.work_email ? ` · ${member.work_email}` : ""}
                    </span>
                    <span className="sr-only">{member.employment_status === "active" ? "Active" : "Not active"}</span>
                  </span>
                </Link>
                <div className="staff-actions">
                  <button
                    type="button"
                    className="staff-actions-trigger"
                    aria-label={`Actions for ${member.full_name}`}
                    aria-haspopup="dialog"
                    aria-expanded={menuStaffId === member.id}
                    onClick={(event) => {
                      if (menuStaffId === member.id) {
                        setMenuStaffId(null);
                      } else {
                        openActions(member, event.currentTarget);
                      }
                    }}
                  >
                    <EllipsisVertical size={20} aria-hidden="true" />
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
        <div className="flex items-center justify-between gap-3 border-t border-[var(--border)] px-4 py-3">
          <span className="text-sm text-[var(--muted)]">Page {pageNumber} of {pageCount}</span>
          <div className="flex gap-2">
            <button type="button" className="button-secondary" disabled={offset === 0 || loading} onClick={() => { setLoading(true); setLoadError(null); setOffset((value) => Math.max(0, value - pageSize)); }}>Previous</button>
            <button type="button" className="button-secondary" disabled={offset + pageSize >= total || offset + pageSize > 1_000_000 || loading} onClick={() => { setLoading(true); setLoadError(null); setOffset((value) => value + pageSize); }}>Next</button>
          </div>
        </div>
      </section>

      {editing !== undefined ? (
        <StaffForm
          key={editing?.id ?? "new-staff"}
          staff={editing}
          departments={departments}
          busy={saving}
          error={formError}
          onSave={saveStaff}
          onCancel={() => { if (!saving) setEditing(undefined); }}
        />
      ) : null}

      {menuStaffId && menuPosition && typeof document !== "undefined"
        ? createPortal(
            <div
              className="staff-actions-backdrop"
              onClick={(event) => {
                if (event.target === event.currentTarget) setMenuStaffId(null);
              }}
            >
              {(() => {
                const member = staff.find((item) => item.id === menuStaffId);
                if (!member) return null;
                return (
                  <div
                    ref={menuRef}
                    className="staff-actions-menu"
                    role="dialog"
                    aria-modal="true"
                    aria-label={`Actions for ${member.full_name}`}
                    style={{ top: menuPosition.top, left: menuPosition.left }}
                  >
                    <div className="staff-actions-menu-header">
                      <span>Staff actions</span>
                      <button
                        type="button"
                        className="staff-actions-close"
                        aria-label="Close staff actions"
                        onClick={() => setMenuStaffId(null)}
                      >
                        <X size={17} aria-hidden="true" />
                      </button>
                    </div>
                    <Link data-menu-first href={`/staff/${encodeURIComponent(member.id)}`} onClick={() => setMenuStaffId(null)}>View profile</Link>
                    {permissions.canUpdate ? (
                      <button
                        type="button"
                        disabled={rowBusyId === member.id}
                        onClick={() => { setMenuStaffId(null); void handleEdit(member); }}
                      >
                        {rowBusyId === member.id ? "Loading…" : "Edit staff"}
                      </button>
                    ) : null}
                    {permissions.canProvision && member.employment_status === "pending" && !member.account_id && member.work_email ? (
                      <button type="button" disabled={rowBusyId === member.id} onClick={() => { setMenuStaffId(null); void handleProvision(member); }}>
                        Send invitation
                      </button>
                    ) : null}
                    {permissions.canDeactivate && member.employment_status !== "inactive" && member.employment_status !== "deactivated" ? (
                      <button type="button" className="is-danger" disabled={rowBusyId === member.id} onClick={() => { setMenuStaffId(null); void handleDeactivate(member); }}>
                        Deactivate
                      </button>
                    ) : null}
                  </div>
                );
              })()}
            </div>,
            document.body,
          )
        : null}
    </>
  );
}
