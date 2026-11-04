"use client";

import Image from "next/image";
import {
  ChevronLeft,
  ChevronRight,
  Ellipsis,
  KeyRound,
  Plus,
  RefreshCw,
  Save,
  Search,
  ShieldCheck,
  UserRoundCog,
  UsersRound,
} from "lucide-react";
import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { FormEvent } from "react";

import { Dialog } from "@/components/workspace/dialog";
import { EmptyState, ErrorBanner, PageHeader, StatusPill } from "@/components/workspace/ui";
import {
  AccessApiError,
  createRole,
  formatAccessError,
  listUserRoleAssignments,
  listPermissions,
  listRoles,
  replaceRolePermissions,
  replaceUserRoles,
  updateRole,
} from "@/lib/access/api";
import type { AccessPermission, AccessRole, UserRoleAssignmentSummary } from "@/lib/access/api";
import { listStaff } from "@/lib/organization/api";

type Props = {
  canRead: boolean;
  canManage: boolean;
  canAssign: boolean;
  canReadStaff: boolean;
  permissions: string[];
  currentUserId: string;
};

function isDuplicateRoleError(error: unknown) {
  return error instanceof AccessApiError && error.status === 409;
}

function AccessPersonAvatar({ name, imageUrl }: { name: string; imageUrl: string | null }) {
  const [failedImageUrl, setFailedImageUrl] = useState<string | null>(null);
  const canShowImage = imageUrl && (imageUrl.startsWith("/") || /^https?:\/\//i.test(imageUrl));

  return (
    <span className="access-person-avatar" aria-hidden="true">
      {canShowImage && failedImageUrl !== imageUrl ? (
        <Image src={imageUrl} alt="" width={42} height={42} unoptimized onError={() => setFailedImageUrl(imageUrl)} />
      ) : name.trim().charAt(0).toUpperCase() || "?"}
    </span>
  );
}

export function AccessManagement({
  canRead,
  canManage,
  canAssign,
  canReadStaff,
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
  const [createError, setCreateError] = useState<string | null>(null);
  const [people, setPeople] = useState<UserRoleAssignmentSummary[]>([]);
  const [staffImages, setStaffImages] = useState<Record<string, string>>({});
  const [staffImageLookupFailed, setStaffImageLookupFailed] = useState(false);
  const [peopleSearchDraft, setPeopleSearchDraft] = useState("");
  const [peopleSearch, setPeopleSearch] = useState("");
  const [peopleRoleFilter, setPeopleRoleFilter] = useState("");
  const [peopleOffset, setPeopleOffset] = useState(0);
  const [peopleTotal, setPeopleTotal] = useState(0);
  const [peopleLoading, setPeopleLoading] = useState(canRead);
  const [peopleError, setPeopleError] = useState(false);
  const [expandedUserId, setExpandedUserId] = useState("");
  const [menuUserId, setMenuUserId] = useState("");
  const [selectedAssignment, setSelectedAssignment] = useState<UserRoleAssignmentSummary | null>(null);
  const [userRoleDraft, setUserRoleDraft] = useState<string[]>([]);
  const [loading, setLoading] = useState(canRead);
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
  const peoplePageSize = 20;

  useEffect(() => {
    if (!canRead) return;
    const controller = new AbortController();
    void listUserRoleAssignments({
      limit: peoplePageSize,
      offset: peopleOffset,
      ...(peopleSearch ? { search: peopleSearch } : {}),
      ...(peopleRoleFilter ? { roleId: peopleRoleFilter } : {}),
    }, controller.signal)
      .then(({ users, total }) => {
        setPeople(users);
        setPeopleTotal(total);
        setPeopleError(false);
      })
      .catch((loadError: unknown) => {
        if (!controller.signal.aborted) {
          setPeopleError(true);
          setError(formatAccessError(loadError, "Unable to load staff role assignments."));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setPeopleLoading(false);
      });
    return () => controller.abort();
  }, [canRead, peopleOffset, peopleRoleFilter, peopleSearch, refreshKey]);

  useEffect(() => {
    if (!canReadStaff || people.length === 0) return;
    const controller = new AbortController();
    let nextIndex = 0;
    let hadLookupFailure = false;
    const workers = Array.from({ length: Math.min(4, people.length) }, async () => {
      while (nextIndex < people.length && !controller.signal.aborted) {
        const person = people[nextIndex];
        nextIndex += 1;
        const imageUrl = person.profile_image_url;
        if (imageUrl) {
          setStaffImages((current) => ({ ...current, [person.user_id]: imageUrl }));
          continue;
        }
        try {
          const result = await listStaff({
            limit: 100,
            offset: 0,
            search: person.staff_id,
          }, controller.signal);
          const staffMember = result.staff.find((member) => member.staff_id === person.staff_id);
          const staffImageUrl = staffMember?.profile_image_url;
          if (staffImageUrl && !controller.signal.aborted) {
            setStaffImages((current) => ({ ...current, [person.user_id]: staffImageUrl }));
          }
        } catch {
          if (controller.signal.aborted) return;
          hadLookupFailure = true;
        }
      }
    });
    void Promise.all(workers).then(() => {
      if (!controller.signal.aborted) setStaffImageLookupFailed(hadLookupFailure);
    });
    return () => controller.abort();
  }, [canReadStaff, people]);

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

  const reload = useCallback(() => {
    setError(null);
    setNotice(null);
    setStaffImageLookupFailed(false);
    setLoading(true);
    setPeopleLoading(true);
    setRefreshKey((key) => key + 1);
  }, []);

  async function submitCreateRole(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setCreateError(null);
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
      setPeopleLoading(true);
      setNotice(`Role “${created.name}” created. Add its permissions below.`);
      setRefreshKey((key) => key + 1);
    } catch (saveError: unknown) {
      setCreateError(formatAccessError(saveError, isDuplicateRoleError(saveError)
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

  async function searchPeople(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const nextSearch = peopleSearchDraft.trim();
    setPeopleLoading(true);
    setError(null);
    setNotice(null);
    if (nextSearch === peopleSearch && peopleOffset === 0) {
      setRefreshKey((key) => key + 1);
    }
    setPeopleSearch(nextSearch);
    setPeopleOffset(0);
    setExpandedUserId("");
    setSelectedAssignment(null);
  }

  async function saveUserRoles() {
    if (!selectedAssignment || selectedAssignment.user_id === currentUserId) return;
    setSaving(true);
    setError(null);
    setNotice(null);
    try {
      const saved = await replaceUserRoles(selectedAssignment.user_id, userRoleDraft);
      setUserRoleDraft(saved.role_ids);
      setPeople((current) => current.map((person) => person.user_id === saved.user_id
        ? { ...person, role_ids: saved.role_ids }
        : person));
      setSelectedAssignment((current) => current ? { ...current, role_ids: saved.role_ids } : current);
      setNotice(`Role assignments updated for ${selectedAssignment.full_name}.`);
    } catch (saveError: unknown) {
      setError(formatAccessError(saveError, "Unable to update role assignments."));
    } finally {
      setSaving(false);
    }
  }

  function toggleAccount(person: UserRoleAssignmentSummary) {
    if (expandedUserId === person.user_id) {
      setExpandedUserId("");
      setSelectedAssignment(null);
      setMenuUserId("");
      return;
    }
    setExpandedUserId(person.user_id);
    setSelectedAssignment(person);
    setUserRoleDraft(person.role_ids);
    setMenuUserId("");
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
          <button type="button" className="button-primary" onClick={() => { setCreateError(null); setCreateOpen(true); }}>
            <Plus size={17} aria-hidden="true" />
            Create role
          </button>
        ) : null}
      />

      <section className="access-control-hero" aria-label="Access control overview">
        <Image
          src="/images/Blueprint%20Construction%20Skyline.png"
          alt=""
          fill
          priority
          sizes="(max-width: 760px) 100vw, 1280px"
          className="access-control-hero-image"
        />
        <span className="access-control-hero-overlay" aria-hidden="true" />
        <div className="access-control-hero-icon"><ShieldCheck size={23} aria-hidden="true" /></div>
        <div className="access-control-hero-copy">
          <span>WORKSPACE SECURITY</span>
          <p>Shape access around responsibilities. Review roles, permissions, and account assignments in one place.</p>
        </div>
        <div className="access-control-hero-badge"><span aria-hidden="true" /> Policy controls</div>
      </section>

      {error ? <ErrorBanner message={error} onRetry={reload} /> : null}
      {canReadStaff && staffImageLookupFailed ? (
        <p role="status" className="access-image-fallback-notice">
          Some staff photos could not be loaded. Initials are shown for those accounts.
        </p>
      ) : null}
      {notice ? <p role="status" className="access-notice">{notice}</p> : null}

      <section className="surface-card access-directory-card" aria-labelledby="access-directory-heading">
        <div className="access-directory-intro">
          <div className="access-directory-title">
            <div className="access-heading-icon"><UsersRound size={19} aria-hidden="true" /></div>
            <div className="access-heading-copy">
              <h2 className="surface-title" id="access-directory-heading">People &amp; access</h2>
              <p>See who has access beyond the standard staff role, and manage assignments in place.</p>
            </div>
          </div>
          <button type="button" className="button-ghost access-refresh" onClick={reload} disabled={loading || peopleLoading} aria-label="Refresh access data">
            <RefreshCw size={16} aria-hidden="true" />
          </button>
        </div>
        <div className="access-summary-grid">
          <div className="access-summary-card">
            <span className="access-summary-icon"><UsersRound size={17} aria-hidden="true" /></span>
            <span className="access-summary-label">Accounts matching filters</span>
            <strong>{peopleTotal}</strong>
          </div>
          <div className="access-summary-card">
            <span className="access-summary-icon access-summary-icon-accent"><ShieldCheck size={17} aria-hidden="true" /></span>
            <span className="access-summary-label">With additional roles <small>on this page</small></span>
            <strong>{people.filter((person) => person.role_ids.some((roleId) => roleId !== "staff")).length}</strong>
          </div>
          <div className="access-summary-card">
            <span className="access-summary-icon access-summary-icon-accent"><KeyRound size={17} aria-hidden="true" /></span>
            <span className="access-summary-label">Roles available</span>
            <strong>{roles.length}</strong>
          </div>
        </div>
        <form className="access-directory-controls" onSubmit={searchPeople}>
          <label className="access-directory-search">
            <span className="sr-only">Search staff accounts</span>
            <Search size={17} aria-hidden="true" />
            <input
              value={peopleSearchDraft}
              onChange={(event) => setPeopleSearchDraft(event.target.value)}
              placeholder="Search name, staff ID, or email"
            />
          </label>
          <label className="access-directory-role-filter">
            <span className="sr-only">Filter by role</span>
            <select
              className="form-select"
              value={peopleRoleFilter}
              onChange={(event) => {
                setPeopleLoading(true);
                setError(null);
                setNotice(null);
                setPeopleRoleFilter(event.target.value);
                setPeopleOffset(0);
                setExpandedUserId("");
                setSelectedAssignment(null);
              }}
            >
              <option value="">All roles</option>
              {roles.map((role) => <option key={role.role_id} value={role.role_id}>{role.name}</option>)}
            </select>
          </label>
          <button type="submit" className="button-secondary" disabled={peopleLoading}>
            {peopleLoading ? "Loading…" : "Search"}
          </button>
        </form>
        <div className="access-directory-table-wrap" aria-busy={peopleLoading}>
          <table className="data-table access-directory-table">
            <thead>
              <tr>
                <th scope="col">Staff member</th>
                <th scope="col">Assigned roles</th>
                {canAssign ? <th scope="col"><span className="sr-only">Actions</span></th> : null}
              </tr>
            </thead>
            <tbody>
              {peopleLoading ? (
                <tr><td colSpan={canAssign ? 3 : 2} className="access-directory-state">Loading staff access…</td></tr>
              ) : people.length ? people.map((person) => {
                const expanded = expandedUserId === person.user_id;
                const personImageUrl = person.profile_image_url || staffImages[person.user_id];
                return (
                  <Fragment key={person.user_id}>
                    <tr className={`access-account-row${expanded ? " is-expanded" : ""}`}>
                      <td>
                        <button
                          type="button"
                          className="access-person-trigger"
                          aria-label={`${expanded ? "Close" : "View"} account for ${person.full_name}`}
                          aria-expanded={expanded}
                          aria-controls={`assignment-${person.user_id}`}
                          onClick={() => toggleAccount(person)}
                        >
                          <span className="access-person-cell">
                            <span className="access-person-avatar-wrap">
                              <AccessPersonAvatar name={person.full_name} imageUrl={personImageUrl} />
                              <span
                                className={`staff-avatar-status access-person-status-dot ${person.account_active && person.employment_status === "active" ? "is-active" : "is-inactive"}`}
                                role="img"
                                aria-label={person.account_active && person.employment_status === "active" ? "Active" : "Inactive"}
                              />
                            </span>
                            <span className="access-person-copy">
                              <strong>{person.full_name}</strong>
                              <span>{person.email || "No email address"} · ID {person.staff_id}</span>
                            </span>
                          </span>
                        </button>
                      </td>
                      <td>
                        <div className="access-role-chips">
                          {person.role_ids.length ? person.role_ids.map((roleId) => {
                            const role = roles.find((item) => item.role_id === roleId);
                            return (
                              <span className={`access-role-chip${roleId === "staff" ? " is-base-role" : ""}`} key={roleId}>
                                {role?.name ?? roleId}
                              </span>
                            );
                          }) : <span className="access-no-roles">No roles assigned</span>}
                        </div>
                      </td>
                      {canAssign ? (
                        <td className="access-directory-action-cell">
                          <div className="access-account-menu-wrap">
                            <button
                              type="button"
                              className="access-account-menu-trigger"
                              aria-expanded={menuUserId === person.user_id}
                              aria-controls={`account-menu-${person.user_id}`}
                              aria-label={`More actions for ${person.full_name}`}
                              onClick={() => setMenuUserId((current) => current === person.user_id ? "" : person.user_id)}
                            >
                              <Ellipsis size={20} aria-hidden="true" />
                            </button>
                            {menuUserId === person.user_id ? (
                              <div className="access-account-menu" id={`account-menu-${person.user_id}`} role="menu">
                                <button
                                  type="button"
                                  role="menuitem"
                                  onClick={() => toggleAccount(person)}
                                >
                                  <UserRoundCog size={16} aria-hidden="true" />
                                  {expanded ? "Close account details" : "Manage roles"}
                                </button>
                              </div>
                            ) : null}
                          </div>
                        </td>
                      ) : null}
                    </tr>
                    {expanded ? (
                      <tr className="access-expanded-row">
                        <td colSpan={canAssign ? 3 : 2}>
                          <section className="access-inline-editor" id={`assignment-${person.user_id}`} aria-label={`Role assignments for ${person.full_name}`}>
                            {!canAssign ? (
                              <p className="access-role-readonly">This account has {person.role_ids.length} assigned {person.role_ids.length === 1 ? "role" : "roles"}. Your permissions do not allow changing assignments.</p>
                            ) : person.user_id === currentUserId ? (
                              <p className="access-warning" role="note">You cannot change roles for your own account.</p>
                            ) : (
                              <>
                                <div className="access-inline-editor-heading">
                                  <div>
                                    <h3>Manage roles for {person.full_name}</h3>
                                    <p>Select the complete set of roles this account should have.</p>
                                  </div>
                                  <span>{userRoleDraft.length} selected</span>
                                </div>
                                <div className="access-assignment-list">
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
                                <div className="access-inline-editor-footer">
                                  <p>The selected set replaces all current assignments. Privilege escalation and removal of the last Super Admin are blocked.</p>
                                  <button type="button" className="button-primary" disabled={saving} onClick={() => void saveUserRoles()}>
                                    <Save size={16} aria-hidden="true" /> {saving ? "Saving…" : "Save assignments"}
                                  </button>
                                </div>
                              </>
                            )}
                          </section>
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                );
              }) : (
                <tr><td colSpan={canAssign ? 4 : 3} className="access-directory-state">
                  <span className="access-directory-empty-icon"><UserRoundCog size={21} aria-hidden="true" /></span>
                  <strong>{peopleError ? "Unable to load staff access" : "No provisioned accounts found"}</strong>
                  <span>{peopleError ? "Refresh the page and try again." : "Try a different search or role filter."}</span>
                </td></tr>
              )}
            </tbody>
          </table>
        </div>
        <div className="access-directory-pagination">
          <span>
            {peopleTotal ? `Showing ${peopleOffset + 1}–${Math.min(peopleOffset + people.length, peopleTotal)} of ${peopleTotal}` : "No accounts"}
          </span>
          <div>
            <button type="button" className="button-ghost" aria-label="Previous page" disabled={peopleLoading || peopleOffset === 0} onClick={() => {
              setPeopleLoading(true);
              setPeopleOffset((offset) => Math.max(0, offset - peoplePageSize));
            }}>
              <ChevronLeft size={17} aria-hidden="true" />
            </button>
            <button type="button" className="button-ghost" aria-label="Next page" disabled={peopleLoading || peopleOffset + peoplePageSize >= peopleTotal} onClick={() => {
              setPeopleLoading(true);
              setPeopleOffset((offset) => offset + peoplePageSize);
            }}>
              <ChevronRight size={17} aria-hidden="true" />
            </button>
          </div>
        </div>
      </section>

      <Dialog
        open={createOpen}
        title="Create a role"
        onClose={() => {
          if (!saving) setCreateOpen(false);
        }}
        actions={(
          <>
            <button type="button" className="button-secondary" disabled={saving} onClick={() => setCreateOpen(false)}>Cancel</button>
            <button type="submit" form="create-role-form" className="button-primary" disabled={saving}>
              <Plus size={16} aria-hidden="true" /> {saving ? "Creating…" : "Create role"}
            </button>
          </>
        )}
      >
        {createError ? <p role="alert" className="field-error mb-4">{createError}</p> : null}
        <form id="create-role-form" className="access-create-modal-form" onSubmit={submitCreateRole}>
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
          <label className="workspace-field">
            <span className="field-label">Description</span>
            <textarea className="form-input form-textarea" value={newRoleDescription} onChange={(event) => setNewRoleDescription(event.target.value)} maxLength={2000} />
          </label>
          <p className="access-create-modal-helper">New roles start with no permissions. Add access only after creating the role.</p>
        </form>
      </Dialog>

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

    </div>
  );
}
