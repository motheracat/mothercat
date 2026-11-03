import { ShieldCheck } from "lucide-react";

import { PageHeader, Panel } from "@/components/workspace/ui";
import { PersonalProfileDetails } from "@/components/workspace/personal-profile-details";
import { requireAuth } from "@/lib/auth/session";

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
        <PersonalProfileDetails
          staffNumber={session.staffId}
          displayName={session.displayName}
          email={session.email}
        />

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
