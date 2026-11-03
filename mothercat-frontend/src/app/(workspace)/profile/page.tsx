import { BadgeCheck, Mail, ShieldCheck, UserRound } from "lucide-react";

import { PageHeader, Panel } from "@/components/workspace/ui";
import { requireAuth } from "@/lib/auth/session";

function getInitials(name: string) {
  return name
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? "")
    .join("") || "U";
}

export default async function PersonalProfilePage() {
  const session = await requireAuth("/profile");
  const roles = Array.from(new Set([session.role, ...session.roles].filter(Boolean)));

  return (
    <>
      <PageHeader
        title="My profile"
        description="Your personal details and access in this workspace."
      />

      <div className="personal-profile-layout">
        <section className="surface-card personal-profile-hero">
          <div className="personal-profile-avatar-wrap">
            <div className="personal-profile-avatar" aria-hidden="true">{getInitials(session.displayName)}</div>
            <span className="staff-avatar-status is-active" role="img" aria-label="Active status" />
          </div>
          <div className="personal-profile-heading">
            <h2>{session.displayName}</h2>
            <p>{session.email || "No email address on file"}</p>
          </div>
        </section>

        <Panel title="Personal information" className="personal-profile-panel">
          <dl className="personal-profile-details">
            <div className="personal-profile-detail">
              <dt><UserRound size={16} aria-hidden="true" /> Full name</dt>
              <dd>{session.displayName}</dd>
            </div>
            <div className="personal-profile-detail">
              <dt><BadgeCheck size={16} aria-hidden="true" /> Staff ID</dt>
              <dd>{session.staffId}</dd>
            </div>
            <div className="personal-profile-detail">
              <dt><Mail size={16} aria-hidden="true" /> Email address</dt>
              <dd>{session.email || "Not provided"}</dd>
            </div>
          </dl>
        </Panel>

        <Panel title="Workspace access" className="personal-profile-panel">
          <div className="personal-profile-access">
            <div className="personal-profile-access-icon"><ShieldCheck size={19} aria-hidden="true" /></div>
            <div>
              <p className="personal-profile-access-label">Your role{roles.length === 1 ? "" : "s"}</p>
              <div className="personal-profile-roles">
                {roles.map((role) => <span key={role} className="personal-profile-role">{role}</span>)}
              </div>
            </div>
          </div>
          <p className="personal-profile-access-note">
            Your role determines which workspace features and actions are available to you.
          </p>
        </Panel>
      </div>
    </>
  );
}
