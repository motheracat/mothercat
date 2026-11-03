"use client";

import { KeyRound, Plus, RefreshCw, Save, Search, ShieldCheck, UserRoundCog } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { FormEvent } from "react";

import { EmptyState, ErrorBanner, PageHeader, StatusPill } from "@/components/workspace/ui";
import {
  AccessApiError,
  createRole,
  formatAccessError,
  getUserRoles,
  listPermissions,
  listRoles,
  replaceRolePermissions,
  replaceUserRoles,
  updateRole,
} from "@/lib/access/api";
import type { AccessPermission, AccessRole } from "@/lib/access/api";
import { listStaff } from "@/lib/organization/api";
import type { StaffMember } from "@/lib/organization/api";

type Props = {
  canRead: boolean;
  canManage: boolean;
  canAssign: boolean;
  permissions: string[];
  currentUserId: string;
};

function isDuplicateRoleError(error: unknown) {
  return error instanceof AccessApiError && error.status === 409;
}

export function AccessManagement({
  canRead,
  canManage,
  canAssign,
  permissions: actorPermissions,
  currentUserId,
}: Props) {
  const [roles, setRoles] = useState<AccessRole[]>([]);
  const [permissions, setPermissions] = useState<AccessPermission[]>([]);
  const [selectedRoleId, setSelectedRoleId] = useState("");
  const [permissionDraft, setPermissionDraft] = useState<string[]>([]);
  const [nameDraft, setNameDraft] = useState("");
  const [descriptionDraft, setDescriptionDraft] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [newRoleId, setNewRoleId] = useState("");
  const [newRoleName, setNewRoleName] = useState("");
  const [newRoleDescription, setNewRoleDescription] = useState("");
  const [staffSearch, setStaffSearch] = useState("");
  const [staffResults, setStaffResults] = useState<StaffMember[]>([]);
  const [selectedStaff, setSelectedStaff] = useState<StaffMember | null>(null);
  const [userRoleDraft, setUserRoleDraft] = useState<string[]>([]);
  const [loading, setLoading] = useState(canRead);
  const [staffLoading, setStaffLoading] = useState(false);
  const [assignmentLoading, setAssignmentLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const selectedRoleIdRef = useRef("");

  const selectedRole = useMemo(
    () => roles.find((role) => role.role_id === selectedRoleId) ?? null,
    [roles, selectedRoleId],
  );
  const actorPermissionSet = useMemo(() => new Set(actorPermissions), [actorPermissions]);
  const hasWildcard = actorPermissionSet.has("*");
  const canDelegatePermission = (key: string) => hasWildcard || actorPermissionSet.has(key);
  const canDelegateRole = (role: AccessRole) => role.permissions.every(canDelegatePermission);

  useEffect(() => {
    if (!canRead) {
      return;
    }
    const controller = new AbortController();
    void Promise.all([listRoles(controller.signal), listPermissions(controller.signal)])
      .then(([nextRoles, nextPermissions]) => {
        setRoles(nextRoles);
        setPermissions(nextPermissions);
        const nextSelectedRoleId = nextRoles.some((role) => role.role_id === selectedRoleIdRef.current)
          ? selectedRoleIdRef.current
          : nextRoles[0]?.role_id ?? "";
        const nextSelectedRole = nextRoles.find((role) => role.role_id === nextSelectedRoleId);
        selectedRoleIdRef.current = nextSelectedRoleId;
        setSelectedRoleId(nextSelectedRoleId);
        setPermissionDraft(nextSelectedRole?.permissions ?? []);
        setNameDraft(nextSelectedRole?.name ?? "");
        setDescriptionDraft(nextSelectedRole?.description ?? "");
      })
      .catch((loadError: unknown) => {
        if (!controller.signal.aborted) {
          setError(formatAccessError(loadError, "Unable to load roles and permissions."));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [canRead, refreshKey]);

  useEffect(() => {
    if (!selectedStaff?.account_id) {
      return;
    }
    const controller = new AbortController();
    void getUserRoles(selectedStaff.account_id, controller.signal)
      .then((assignment) => setUserRoleDraft(assignment.role_ids))
      .catch((loadError: unknown) => {
        if (!controller.signal.aborted) {
          setError(formatAccessError(loadError, "Unable to load this staff member’s role assignments."));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setAssignmentLoading(false);
      });
    return () => controller.abort();
  }, [selectedStaff]);

  const reload = useCallback(() => {
    setError(null);
    setNotice(null);
    setLoading(true);
    setRefreshKey((key) => key + 1);
  }, []);

  async function submitCreateRole(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setError(null);
    setNotice(null);
    try {
      const created = await createRole({
        role_id: newRoleId.trim(),
        name: newRoleName.trim(),
        description: newRoleDescription.trim(),
      });
      setCreateOpen(false);
      setNewRoleId("");
      setNewRoleName("");
      setNewRoleDescription("");
      selectedRoleIdRef.current = created.role_id;
      setSelectedRoleId(created.role_id);
      setPermissionDraft(created.permissions);
      setNameDraft(created.name);
      setDescriptionDraft(created.description);
      setLoading(true);
      setNotice(`Role “${created.name}” created. Add its permissions below.`);
      setRefreshKey((key) => key + 1);
    } catch (saveError: unknown) {
      setError(formatAccessError(saveError, isDuplicateRoleError(saveError)
        ? "That role key or display name is already in use."
        : "Unable to create the role."));
    } finally {
      setSaving(false);
    }
  }

  async function saveRoleDetails(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedRole || selectedRole.is_system_role) return;
    setSaving(true);
    setError(null);
    setNotice(null);
    try {
      const updated = await updateRole(selectedRole.role_id, {
        name: nameDraft.trim(),
        description: descriptionDraft.trim(),
      });
      setRoles((current) => current.map((role) => role.role_id === updated.role_id ? updated : role));
      setNotice("Role details saved.");
    } catch (saveError: unknown) {
      setError(formatAccessError(saveError, "Unable to save role details."));
    } finally {
      setSaving(false);
    }
  }

  async function savePermissions() {
    if (!selectedRole || selectedRole.is_system_role) return;
    setSaving(true);
    setError(null);
    setNotice(null);
    try {
      const savedPermissions = await replaceRolePermissions(selectedRole.role_id, permissionDraft);
      setRoles((current) => current.map((role) =>
        role.role_id === selectedRole.role_id ? { ...role, permissions: savedPermissions } : role,
      ));
      setPermissionDraft(savedPermissions);
      setNotice("Role permissions saved.");
    } catch (saveError: unknown) {
      setError(formatAccessError(saveError, "Unable to save role permissions."));
    } finally {
      setSaving(false);
    }
  }

  async function searchStaffAccounts(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setStaffLoading(true);
    setError(null);
    setStaffResults([]);
    setSelectedStaff(null);
    setUserRoleDraft([]);
    setAssignmentLoading(false);
    try {
      const result = await listStaff({
        limit: 50,
        offset: 0,
        ...(staffSearch.trim() ? { search: staffSearch.trim() } : {}),
      });
      setStaffResults(result.staff.filter((staff) => staff.account_id));
    } catch (searchError: unknown) {
      setError(formatAccessError(searchError, "Unable to search staff accounts."));
    } finally {
      setStaffLoading(false);
    }
  }

  async function saveUserRoles() {
    if (!selectedStaff?.account_id || selectedStaff.account_id === currentUserId) return;
    setSaving(true);
    setError(null);
    setNotice(null);
    try {
      const saved = await replaceUserRoles(selectedStaff.account_id, userRoleDraft);
      setUserRoleDraft(saved.role_ids);
      setNotice(`Role assignments updated for ${selectedStaff.full_name}.`);
    } catch (saveError: unknown) {
      setError(formatAccessError(saveError, "Unable to update role assignments."));
    } finally {
      setSaving(false);
    }
  }

  if (!canRead) {
    return (
      <>
        <PageHeader title="Roles & permissions" description="Review roles and their assigned access." />
        <section className="surface-card p-5">
          <EmptyState>
            Roles and permissions are available to accounts with the <code>roles.read</code> permission.
          </EmptyState>
        </section>
      </>
    );
  }

  return (
    <div className="access-management-page">
      <PageHeader
        title="Roles & permissions"
        description="Manage role access and account assignments."
        actions={canManage ? (
          <button type="button" className="button-primary" onClick={() => setCreateOpen((open) => !open)}>
            <Plus size={17} aria-hidden="true" />
            {createOpen ? "Close form" : "Create role"}
          </button>
        ) : null}
      />

      {error ? <ErrorBanner message={error} onRetry={reload} /> : null}
      {notice ? <p role="status" className="access-notice">{notice}</p> : null}

      {createOpen ? (
        <section className="surface-card access-create-card" aria-labelledby="create-role-heading">
          <div className="surface-card-header">
            <h2 className="surface-title" id="create-role-heading">Create a role</h2>
          </div>
          <form className="access-create-form" onSubmit={submitCreateRole}>
            <label className="workspace-field">
              <span className="field-label">Role key</span>
              <input
                className="form-input"
                value={newRoleId}
                onChange={(event) => setNewRoleId(event.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ""))}
                pattern="[a-z][a-z0-9_]{0,63}"
                maxLength={64}
                placeholder="e.g. project_reviewer"
                required
              />
            </label>
            <label className="workspace-field">
              <span className="field-label">Display name</span>
              <input className="form-input" value={newRoleName} onChange={(event) => setNewRoleName(event.target.value)} maxLength={100} required />
            </label>
            <label className="workspace-field access-create-description">
              <span className="field-label">Description</span>
              <input className="form-input" value={newRoleDescription} onChange={(event) => setNewRoleDescription(event.target.value)} maxLength={2000} />
            </label>
            <button type="submit" className="button-primary" disabled={saving}>
              <Plus size={16} aria-hidden="true" /> Create role
            </button>
          </form>
          <p className="access-helper">New roles start with no permissions. Add access only after creating the role.</p>
        </section>
      ) : null}

      <section className="access-role-layout" aria-label="Role management">
        <div className="surface-card access-role-list-card">
          <div className="surface-card-header">
            <h2 className="surface-title">Roles</h2>
            <button type="button" className="button-ghost access-refresh" onClick={reload} disabled={loading} aria-label="Refresh roles">
              <RefreshCw size={16} aria-hidden="true" />
            </button>
          </div>
          {loading ? (
            <div role="status" aria-busy="true" className="access-loading">Loading roles…</div>
          ) : roles.length ? (
            <ul className="access-role-list">
              {roles.map((role) => (
                <li key={role.role_id}>
                  <button
                    type="button"
                    className={`access-role-option${selectedRoleId === role.role_id ? " is-selected" : ""}`}
                    aria-pressed={selectedRoleId === role.role_id}
                    onClick={() => {
                      selectedRoleIdRef.current = role.role_id;
                      setSelectedRoleId(role.role_id);
                      setPermissionDraft(role.permissions);
                      setNameDraft(role.name);
                      setDescriptionDraft(role.description);
                    }}
                  >
                    <span className="access-role-option-main">
                      <span className="access-role-name">{role.name}</span>
                      <span className="access-role-description">{role.description || "No description"}</span>
                    </span>
                    <span className="access-role-option-meta">
                      <StatusPill label={role.is_system_role ? "System" : "Custom"} tone={role.is_system_role ? "neutral" : "success"} />
                      <span className="access-permission-count">{role.permissions.length} permissions</span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <EmptyState>No roles are available.</EmptyState>
          )}
        </div>

        <div className="access-role-details">
          {selectedRole ? (
            <>
              <section className="surface-card">
                <div className="surface-card-header access-section-heading">
                  <div className="access-heading-icon"><ShieldCheck size={18} aria-hidden="true" /></div>
                  <div className="access-heading-copy">
                    <h2 className="surface-title">{selectedRole.name}</h2>
                    <p>{selectedRole.is_system_role ? "Built-in role" : "Custom role"} · {selectedRole.role_id}</p>
                  </div>
                  <StatusPill label={selectedRole.is_system_role ? "Read-only" : "Editable"} tone={selectedRole.is_system_role ? "neutral" : "success"} />
                </div>
                <div className="access-role-editor">
                  {canManage && !selectedRole.is_system_role ? (
                    <form className="workspace-form" onSubmit={saveRoleDetails}>
                      <label className="workspace-field">
                        <span className="field-label">Display name</span>
                        <input className="form-input" value={nameDraft} onChange={(event) => setNameDraft(event.target.value)} maxLength={100} required />
                      </label>
                      <label className="workspace-field">
                        <span className="field-label">Description</span>
                        <textarea className="form-input form-textarea" value={descriptionDraft} onChange={(event) => setDescriptionDraft(event.target.value)} maxLength={2000} />
                      </label>
                      <div>
                        <button type="submit" className="button-secondary" disabled={saving || (nameDraft === selectedRole.name && descriptionDraft === selectedRole.description)}>
                          <Save size={16} aria-hidden="true" /> Save details
                        </button>
                      </div>
                    </form>
                  ) : (
                    <p className="access-role-readonly">{selectedRole.description || "No description."}</p>
                  )}
                </div>
              </section>

              <section className="surface-card">
                <div className="surface-card-header access-section-heading">
                  <div className="access-heading-icon"><KeyRound size={18} aria-hidden="true" /></div>
                  <div className="access-heading-copy">
                    <h2 className="surface-title">Permissions</h2>
                    <p>{selectedRole.permissions.length} currently assigned</p>
                  </div>
                  {canManage && !selectedRole.is_system_role ? (
                    <button type="button" className="button-primary" onClick={() => void savePermissions()} disabled={saving}>
                      <Save size={16} aria-hidden="true" /> Save permissions
                    </button>
                  ) : null}
                </div>
                <div className="access-permission-list">
                  {permissions.map((permission) => {
                    const checked = permissionDraft.includes(permission.permission_key);
                    const delegable = canDelegatePermission(permission.permission_key);
                    const disabled = !canManage || selectedRole.is_system_role || (!delegable && !checked) || saving;
                    return (
                      <label className={`access-permission-item${disabled ? " is-disabled" : ""}`} key={permission.permission_key}>
                        <input
                          type="checkbox"
                          checked={checked}
                          disabled={disabled}
                          onChange={(event) => setPermissionDraft((current) => event.target.checked
                            ? [...current, permission.permission_key]
                            : current.filter((key) => key !== permission.permission_key))}
                        />
                        <span className="access-permission-copy">
                          <span className="access-permission-key">{permission.permission_key}</span>
                          <span className="access-permission-description">{permission.description}</span>
                        </span>
                        {!delegable && !checked ? <span className="access-cannot-delegate">Not held by you</span> : null}
                      </label>
                    );
                  })}
                  {!permissions.length && !loading ? <EmptyState>No permissions are defined yet.</EmptyState> : null}
                </div>
                {canManage && !selectedRole.is_system_role ? (
                  <p className="access-helper">You can grant only permissions you currently hold. Changes replace the complete set of permissions for this role.</p>
                ) : null}
              </section>
            </>
          ) : (
            <section className="surface-card"><EmptyState>Select a role to view its permissions.</EmptyState></section>
          )}
        </div>
      </section>

      {canAssign ? (
        <section className="surface-card access-user-roles">
          <div className="surface-card-header access-section-heading">
            <div className="access-heading-icon"><UserRoundCog size={18} aria-hidden="true" /></div>
            <div className="access-heading-copy">
              <h2 className="surface-title">Assign roles to a staff account</h2>
              <p>Search for a provisioned account, then replace its role assignments.</p>
            </div>
          </div>
          <div className="access-user-role-body">
            <form className="access-staff-search" onSubmit={searchStaffAccounts}>
              <label className="workspace-field">
                <span className="field-label">Search staff</span>
                <input
                  className="form-input"
                  value={staffSearch}
                  onChange={(event) => setStaffSearch(event.target.value)}
                  placeholder="Name, staff ID, or email"
                />
              </label>
              <button type="submit" className="button-secondary" disabled={staffLoading}>
                <Search size={16} aria-hidden="true" /> {staffLoading ? "Searching…" : "Search"}
              </button>
            </form>
            {staffResults.length ? (
              <label className="workspace-field access-staff-result-select">
                <span className="field-label">Provisioned accounts</span>
                <select
                  className="form-select"
                  value={selectedStaff?.id ?? ""}
                  onChange={(event) => {
                    setUserRoleDraft([]);
                    const staff = staffResults.find((item) => item.id === event.target.value) ?? null;
                    setAssignmentLoading(Boolean(staff?.account_id && staff.account_id !== currentUserId));
                    setSelectedStaff(staff);
                  }}
                >
                  <option value="">Select a staff account</option>
                  {staffResults.map((staff) => (
                    <option key={staff.id} value={staff.id}>
                      {staff.full_name} · ID:{staff.staff_id}{staff.account_id === currentUserId ? " · Your account" : ""}
                    </option>
                  ))}
                </select>
              </label>
            ) : null}
            {staffResults.length === 0 && !staffLoading && staffSearch ? (
              <p className="access-helper">No provisioned accounts found. Search results include staff profiles that have an account.</p>
            ) : null}
            {selectedStaff?.account_id ? (
              selectedStaff.account_id === currentUserId ? (
                <p className="access-warning" role="note">You cannot change roles for your own account.</p>
              ) : (
                <div className="access-assignment-editor">
                  <p className="access-assignment-title">Roles for {selectedStaff.full_name}</p>
                  {assignmentLoading ? <p role="status" className="access-loading">Loading current assignments…</p> : null}
                  <div className="access-assignment-list" aria-busy={assignmentLoading}>
                    {roles.map((role) => {
                      const checked = userRoleDraft.includes(role.role_id);
                      const delegable = canDelegateRole(role);
                      const disabled = saving || (!delegable && !checked);
                      return (
                        <label className={`access-assignment-option${disabled ? " is-disabled" : ""}`} key={role.role_id}>
                          <input
                            type="checkbox"
                            checked={checked}
                            disabled={disabled}
                            onChange={(event) => setUserRoleDraft((current) => event.target.checked
                              ? [...current, role.role_id]
                              : current.filter((roleId) => roleId !== role.role_id))}
                          />
                          <span><strong>{role.name}</strong><small>{role.description || role.role_id}</small></span>
                          {!delegable && !checked ? <span className="access-cannot-delegate">Contains access you don’t hold</span> : null}
                        </label>
                      );
                    })}
                  </div>
                  <button type="button" className="button-primary" disabled={saving || assignmentLoading} onClick={() => void saveUserRoles()}>
                    <Save size={16} aria-hidden="true" /> {saving ? "Saving…" : "Save role assignments"}
                  </button>
                  <p className="access-helper">This replaces all selected account roles. The backend prevents privilege escalation and protects the last active Super Admin.</p>
                </div>
              )
            ) : null}
          </div>
        </section>
      ) : null}
    </div>
  );
}
