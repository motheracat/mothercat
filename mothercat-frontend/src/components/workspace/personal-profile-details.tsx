"use client";

import { BadgeCheck, Mail, UserRound } from "lucide-react";
import { useCallback, useState, type FormEvent } from "react";

import { Panel } from "@/components/workspace/ui";
import { ProfilePictureUpload } from "@/components/workspace/profile-picture-upload";
import {
  formatOrganizationError,
  updateStaff,
  type StaffMember,
} from "@/lib/organization/api";

type PersonalProfileDetailsProps = {
  staffNumber: string;
  displayName: string;
  email: string;
};

function PersonalProfileForm({
  staff,
  busy,
  error,
  onSave,
  onCancel,
}: {
  staff: StaffMember;
  busy: boolean;
  error: string | null;
  onSave: (values: { full_name: string; work_email: string | null }) => Promise<void>;
  onCancel: () => void;
}) {
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formData = new FormData(event.currentTarget);
    onSave({
      full_name: String(formData.get("full_name") ?? "").trim(),
      work_email: String(formData.get("work_email") ?? "").trim() || null,
    });
  }

  return (
    <form className="personal-profile-edit-form" onSubmit={submit}>
      {error ? <div role="alert" className="error-banner"><p className="text-sm">{error}</p></div> : null}
      <label className="block text-sm font-medium">
        Full name
        <input className="form-input mt-1" name="full_name" defaultValue={staff.full_name} maxLength={200} required autoComplete="name" />
      </label>
      <label className="block text-sm font-medium">
        Work email
        <input className="form-input mt-1" name="work_email" type="email" defaultValue={staff.work_email ?? ""} maxLength={254} autoComplete="email" />
      </label>
      <div className="flex justify-end gap-2">
        <button type="button" className="button-secondary" onClick={onCancel} disabled={busy}>Cancel</button>
        <button type="submit" className="button-primary" disabled={busy}>{busy ? "Saving…" : "Save changes"}</button>
      </div>
    </form>
  );
}

export function PersonalProfileDetails({ staffNumber, displayName, email }: PersonalProfileDetailsProps) {
  const [staff, setStaff] = useState<StaffMember | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [lookupFailure, setLookupFailure] = useState<"missing" | "request" | null>(null);
  const [lookupRetryKey, setLookupRetryKey] = useState(0);
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const handleStaffResolved = useCallback((resolvedStaff: StaffMember) => {
    setStaff(resolvedStaff);
    setLoadError(null);
    setLookupFailure(null);
    setLoading(false);
  }, []);
  const handleStaffLookupError = useCallback((message: string, reason: "missing" | "request") => {
    setLoadError(message);
    setLookupFailure(reason);
    setLoading(false);
  }, []);

  async function saveProfile(values: { full_name: string; work_email: string | null }) {
    if (!staff) return;
    const changes: Partial<Pick<StaffMember, "full_name" | "work_email">> = {};
    if (values.full_name !== staff.full_name) changes.full_name = values.full_name;
    if (values.work_email !== staff.work_email) changes.work_email = values.work_email;

    setSaving(true);
    setFormError(null);
    try {
      if (Object.keys(changes).length > 0) setStaff(await updateStaff(staff.id, changes));
      setEditing(false);
    } catch (error) {
      setFormError(formatOrganizationError(error, "Unable to update your profile. Try again."));
    } finally {
      setSaving(false);
    }
  }

  const name = staff?.full_name ?? displayName;
  const workEmail = staff ? staff.work_email : email;

  return (
    <>
      <section className="surface-card personal-profile-hero">
        <ProfilePictureUpload
          key={lookupRetryKey}
          staffNumber={staffNumber}
          name={name}
          profileImageUrl={staff?.profile_image_url ?? null}
          avatarWrapClassName="personal-profile-avatar-wrap"
          avatarClassName="personal-profile-avatar"
          statusClassName="staff-avatar-status is-active"
          statusLabel="Active status"
          onStaffResolved={handleStaffResolved}
          onStaffLookupError={handleStaffLookupError}
          onStaffUpdated={setStaff}
        />
        <div className="personal-profile-heading">
          <h2>{name}</h2>
          <p>{workEmail || "No email address on file"}</p>
        </div>
      </section>

      <Panel
        title="Personal information"
        className="personal-profile-panel"
        action={!editing ? (
          <button
            type="button"
            className="button-secondary"
            disabled={loading || lookupFailure === "missing"}
            onClick={() => {
              if (!staff) {
                setLoadError(null);
                setLoading(true);
                setLookupFailure(null);
                setLookupRetryKey((key) => key + 1);
                return;
              }
              setFormError(null);
              setEditing(true);
            }}
          >
            {staff
              ? "Edit profile"
              : loading
                ? "Checking staff record…"
                : lookupFailure === "missing"
                  ? "Profile editing unavailable"
                  : "Retry profile lookup"}
          </button>
        ) : null}
      >
        {editing && staff ? (
          <PersonalProfileForm
            key={staff.id}
            staff={staff}
            busy={saving}
            error={formError}
            onSave={saveProfile}
            onCancel={() => { if (!saving) setEditing(false); }}
          />
        ) : (
          <dl className="personal-profile-details">
            <div className="personal-profile-detail">
              <dt><UserRound size={16} aria-hidden="true" /> Full name</dt>
              <dd>{name}</dd>
            </div>
            <div className="personal-profile-detail">
              <dt><BadgeCheck size={16} aria-hidden="true" /> Staff ID</dt>
              <dd>{staffNumber}</dd>
            </div>
            <div className="personal-profile-detail">
              <dt><Mail size={16} aria-hidden="true" /> Email address</dt>
              <dd>{workEmail || "Not provided"}</dd>
            </div>
          </dl>
        )}
        {loadError ? <p className="profile-picture-feedback" role="alert">{loadError}</p> : null}
      </Panel>
    </>
  );
}
