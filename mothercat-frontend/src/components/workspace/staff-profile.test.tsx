import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { StaffProfile } from "@/components/workspace/staff-profile";
import { WorkspaceContextProvider } from "@/components/workspace/workspace-context";
import {
  getStaff,
  listDepartments,
  type StaffMember,
} from "@/lib/organization/api";

vi.mock("@/lib/organization/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/organization/api")>();
  return {
    ...actual,
    getStaff: vi.fn(),
    listDepartments: vi.fn(),
  };
});

const staffRecord: StaffMember = {
  id: "staff-record-uuid",
  account_id: "account-uuid",
  staff_id: "23847",
  department_id: null,
  full_name: "Ada Okafor",
  profile_image_url: null,
  work_email: "ada@example.org",
  job_title: "Engineer",
  employment_status: "active",
};

const basePermissions = {
  canRead: true,
  canUpdate: false,
  canDeactivate: false,
  canProvision: false,
  canProvisionAdmin: false,
};

function renderStaffProfile(userId: string, canUpdate = false) {
  return render(
    <WorkspaceContextProvider session={null} devOpen>
      <StaffProfile
        staffId={staffRecord.id}
        userId={userId}
        permissions={{ ...basePermissions, canUpdate }}
      />
    </WorkspaceContextProvider>,
  );
}

beforeEach(() => {
  vi.mocked(getStaff).mockReset();
  vi.mocked(getStaff).mockResolvedValue(staffRecord);
  vi.mocked(listDepartments).mockReset();
  vi.mocked(listDepartments).mockResolvedValue([]);
});

afterEach(cleanup);

describe("StaffProfile editing permissions", () => {
  it("allows a linked staff member to edit their own name and email only", async () => {
    renderStaffProfile("account-uuid");

    fireEvent.click(await screen.findByRole("button", { name: "Edit profile" }));

    expect(screen.getByLabelText("Full name")).toHaveValue("Ada Okafor");
    expect(screen.getByLabelText("Work email")).toHaveValue("ada@example.org");
    expect(screen.queryByLabelText("Job title")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Department")).not.toBeInTheDocument();
  });

  it("does not allow a staff member to edit someone else's profile", async () => {
    renderStaffProfile("another-account");

    await screen.findByText("ID:23847");
    expect(screen.queryByRole("button", { name: "Edit profile" })).not.toBeInTheDocument();
  });

  it("shows organization-managed fields to a user with staff.update", async () => {
    renderStaffProfile("another-account", true);

    fireEvent.click(await screen.findByRole("button", { name: "Edit profile" }));

    expect(screen.getByLabelText("Job title")).toHaveValue("Engineer");
    expect(screen.getByLabelText("Department")).toBeInTheDocument();
  });
});
