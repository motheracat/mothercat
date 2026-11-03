"use client";

import { useEffect, useState, type FormEvent } from "react";
import { ArrowLeft, BriefcaseBusiness, Building2, Mail, UserRound } from "lucide-react";
import Link from "next/link";

import { PageHeader, WorkspacePageSkeleton } from "@/components/workspace/ui";
import { useWorkspace } from "@/components/workspace/workspace-context";
import {
  deactivateStaff,
  formatOrganizationError,
  getStaff,
  listDepartments,
  provisionAdmin,
  provisionStaff,
  updateStaff,
  type Department,
  type StaffMember,
} from "@/lib/organization/api";

type StaffProfilePermissions = {
  canRead: boolean;
  canUpdate: boolean;
  canDeactivate: boolean;
  canProvision: boolean;
  canProvisionAdmin: boolean;
};

function initials(name: string) {
  return name.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? "").join("") || "S";
}

function StaffProfileForm({
  staff,
  departments,
  busy,
  error,
  onSave,
  onCancel,
}: {
  staff: StaffMember;
  departments: Department[];
  busy: boolean;
  error: string | null;
  onSave: (values: Pick<StaffMember, "full_name" | "work_email" | "job_title" | "department_id">) => Promise<void>;
  onCancel: () => void;
}) {
  const [name, setName] = useState(staff.full_name);
  const [email, setEmail] = useState(staff.work_email ?? "");
  const [jobTitle, setJobTitle] = useState(staff.job_title ?? "");
  const [departmentId, setDepartmentId] = useState(staff.department_id ?? "");

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void onSave({
      full_name: name.trim(),
      work_email: email.trim() || null,
      job_title: jobTitle.trim() || null,
      department_id: departmentId || null,
    });
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center overflow-y-auto bg-slate-950/40 p-4" role="presentation">
      <section role="dialog" aria-modal="true" aria-labelledby="profile-edit-title" className="my-auto w-full max-w-xl rounded-xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-xl">
        <h2 id="profile-edit-title" className="text-xl font-semibold">Edit staff profile</h2>
        <p className="mt-1 text-sm text-[var(--muted)]">Staff number and employment status cannot be changed here.</p>
        {error ? <div role="alert" className="error-banner mt-4"><p className="text-sm">{error}</p></div> : null}
        <form className="mt-5 grid gap-4 sm:grid-cols-2" onSubmit={submit}>
          <label className="block text-sm font-medium sm:col-span-2">
            Full name
            <input className="form-input mt-1" value={name} onChange={(event) => setName(event.target.value)} required />
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
            <button type="submit" className="button-primary" disabled={busy}>{busy ? "Saving…" : "Save changes"}</button>
          </div>
        </form>
      </section>
    </div>
  );
}

export function StaffProfile({ staffId, permissions }: { staffId: string; permissions: StaffProfilePermissions }) {
  const { notify } = useWorkspace();
  const [staff, setStaff] = useState<StaffMember | null>(null);
  const [departments, setDepartments] = useState<Department[]>([]);
  const [loadedStaffId, setLoadedStaffId] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<{ id: string; message: string } | null>(null);
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  useEffect(() => {
    if (!permissions.canRead) return;
    let active = true;
    Promise.all([getStaff(staffId), listDepartments()])
      .then(([profile, departmentList]) => {
        if (!active) return;
        setStaff(profile);
        setDepartments(departmentList);
        setLoadError(null);
        setLoadedStaffId(staffId);
      })
      .catch((error: unknown) => {
        if (active) {
          setLoadError({ id: staffId, message: formatOrganizationError(error, "Unable to load this staff profile.") });
          setLoadedStaffId(staffId);
        }
      });
    return () => { active = false; };
  }, [staffId, permissions.canRead]);

  async function saveProfile(values: Pick<StaffMember, "full_name" | "work_email" | "job_title" | "department_id">) {
    if (!staff) return;
    setSaving(true);
    setFormError(null);
    const changes: Partial<Pick<StaffMember, "full_name" | "work_email" | "job_title" | "department_id">> = {};
    if (values.full_name !== staff.full_name) changes.full_name = values.full_name;
    if (values.work_email !== staff.work_email) changes.work_email = values.work_email;
    if (values.job_title !== staff.job_title) changes.job_title = values.job_title;
    if (values.department_id !== staff.department_id) changes.department_id = values.department_id;

    try {
      const updated = Object.keys(changes).length > 0 ? await updateStaff(staff.id, changes) : staff;
      setStaff(updated);
      setEditing(false);
      notify("Staff profile updated.");
    } catch (error) {
      const message = formatOrganizationError(error, "Unable to update this staff profile.");
      setFormError(message);
      notify(message, "error");
    } finally {
      setSaving(false);
    }
  }

  async function handleDeactivate() {
    if (!staff || !window.confirm(`Deactivate ${staff.full_name}? This cannot be undone from this screen.`)) return;
    try {
      setStaff(await deactivateStaff(staff.id));
      notify("Staff employment has been deactivated.");
    } catch (error) {
      notify(formatOrganizationError(error, "Unable to deactivate this staff profile."), "error");
    }
  }

  async function handleProvision() {
    if (!staff?.work_email || !window.confirm(`Send an activation invitation to ${staff.work_email}?`)) return;
    try {
      await provisionStaff(staff.id);
      notify(`An activation invitation was sent to ${staff.work_email}.`);
    } catch (error) {
      notify(formatOrganizationError(error, "Unable to send an activation invitation."), "error");
    }
  }

  async function handleProvisionAdmin() {
    if (!staff?.work_email || !window.confirm(`Provision ${staff.full_name} as an Admin and send an activation invitation to ${staff.work_email}?`)) return;
    try {
      await provisionAdmin(staff.id);
      notify(`Admin activation invitation sent to ${staff.work_email}.`);
    } catch (error) {
      notify(formatOrganizationError(error, "Unable to provision this Admin account."), "error");
    }
  }

  const department = departments.find((item) => item.id === staff?.department_id);
  const isActive = staff?.employment_status === "active";
  const loading = loadedStaffId !== staffId;
  const currentLoadError = loadError?.id === staffId ? loadError.message : null;

  if (!permissions.canRead) {
    return (
      <>
        <PageHeader title="Staff profile" description="Staff record details." />
        <div role="alert" className="error-banner"><p className="text-sm">You do not have permission to view staff records.</p></div>
      </>
    );
  }

  if (loading) {
    return <WorkspacePageSkeleton />;
  }

  if (currentLoadError || !staff) {
    return (
      <>
        <PageHeader title="Staff profile" description="Staff record details." actions={<Link href="/staff" className="button-secondary"><ArrowLeft size={16} /> Staff directory</Link>} />
        <div role="alert" className="error-banner"><p className="text-sm">{currentLoadError ?? "Staff profile not found."}</p></div>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="Staff profile"
        description="Full employment and contact details for this staff member."
        actions={
          <>
            <Link href="/staff" className="button-secondary"><ArrowLeft size={16} aria-hidden="true" /> Staff directory</Link>
            {permissions.canUpdate ? <button type="button" className="button-primary" onClick={() => { setFormError(null); setEditing(true); }}>Edit profile</button> : null}
          </>
        }
      />
      <section className="surface-card staff-profile-card">
        <div className="staff-profile-heading">
          <span className="staff-avatar-wrap staff-avatar-wrap-large" aria-hidden="true">
            <span className="staff-avatar staff-avatar-large">{initials(staff.full_name)}</span>
            <span className={`staff-avatar-status ${isActive ? "is-active" : "is-inactive"}`} />
          </span>
          <div className="min-w-0 flex-1">
            <h2 className="staff-profile-name">{staff.full_name}</h2>
            <p className="mt-1 text-sm text-[var(--muted)]">ID:{staff.staff_id}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            {permissions.canProvision && staff.employment_status === "pending" && !staff.account_id && staff.work_email ? (
              <button type="button" className="button-secondary" onClick={() => void handleProvision()}>Send invitation</button>
            ) : null}
            {permissions.canProvisionAdmin && staff.employment_status === "pending" && !staff.account_id && staff.work_email ? (
              <button type="button" className="button-secondary" onClick={() => void handleProvisionAdmin()}>Provision as Admin</button>
            ) : null}
            {permissions.canDeactivate && staff.employment_status !== "inactive" && staff.employment_status !== "deactivated" ? (
              <button type="button" className="button-danger" onClick={() => void handleDeactivate()}>Deactivate</button>
            ) : null}
          </div>
        </div>

        <div className="staff-profile-details">
          <div className="staff-detail-item">
            <span className="staff-detail-icon"><Mail size={18} aria-hidden="true" /></span>
            <div><p className="staff-detail-label">Work email</p><p className="staff-detail-value">{staff.work_email || "Not provided"}</p></div>
          </div>
          <div className="staff-detail-item">
            <span className="staff-detail-icon"><BriefcaseBusiness size={18} aria-hidden="true" /></span>
            <div><p className="staff-detail-label">Job title</p><p className="staff-detail-value">{staff.job_title || "Not provided"}</p></div>
          </div>
          <div className="staff-detail-item">
            <span className="staff-detail-icon"><Building2 size={18} aria-hidden="true" /></span>
            <div><p className="staff-detail-label">Department</p><p className="staff-detail-value">{department?.name ?? "Not assigned"}</p></div>
          </div>
          <div className="staff-detail-item">
            <span className="staff-detail-icon"><UserRound size={18} aria-hidden="true" /></span>
            <div><p className="staff-detail-label">Account</p><p className="staff-detail-value">{staff.account_id ? "Login account associated" : "No login account"}</p></div>
          </div>
        </div>
      </section>

      {editing ? (
        <StaffProfileForm
          staff={staff}
          departments={departments}
          busy={saving}
          error={formError}
          onSave={saveProfile}
          onCancel={() => { if (!saving) setEditing(false); }}
        />
      ) : null}
    </>
  );
}
