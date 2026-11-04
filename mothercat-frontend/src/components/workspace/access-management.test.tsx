import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AccessManagement } from "@/components/workspace/access-management";

const accessMocks = vi.hoisted(() => ({
  listRoles: vi.fn(),
  listPermissions: vi.fn(),
  createRole: vi.fn(),
  updateRole: vi.fn(),
  replaceRolePermissions: vi.fn(),
  getUserRoles: vi.fn(),
  replaceUserRoles: vi.fn(),
  listUserRoleAssignments: vi.fn(),
}));

vi.mock("@/lib/access/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/access/api")>();
  return { ...actual, ...accessMocks };
});

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  accessMocks.listRoles.mockResolvedValue([
    {
      role_id: "staff",
      name: "Staff",
      description: "Basic staff access",
      is_system_role: true,
      permissions: ["staff.read"],
    },
    {
      role_id: "auditor",
      name: "Auditor",
      description: "Reviews audit activity",
      is_system_role: false,
      permissions: ["audit.read"],
    },
  ]);
  accessMocks.listPermissions.mockResolvedValue([
    { permission_key: "staff.read", description: "Read staff records" },
    { permission_key: "audit.read", description: "Read audit events" },
    { permission_key: "settings.manage", description: "Manage settings" },
  ]);
  accessMocks.replaceRolePermissions.mockImplementation(async (_roleId: string, keys: string[]) => keys);
  accessMocks.getUserRoles.mockResolvedValue({ user_id: "target-user", role_ids: ["staff"] });
  accessMocks.replaceUserRoles.mockImplementation(async (userId: string, roleIds: string[]) => ({
    user_id: userId,
    role_ids: roleIds,
  }));
  accessMocks.listUserRoleAssignments.mockResolvedValue({
    users: [{
      user_id: "target-user",
      full_name: "Taylor Staff",
      staff_id: "12345",
      email: "taylor@example.com",
      employment_status: "active",
      account_active: true,
      role_ids: ["staff"],
    }],
    total: 1,
  });
});

function renderAccessPage() {
  return render(
    <AccessManagement
      canRead
      canManage
      canAssign
      permissions={["roles.read", "roles.manage", "user_roles.assign", "staff.read", "audit.read"]}
      currentUserId="current-user"
    />,
  );
}

describe("AccessManagement", () => {
  it("loads roles and saves a changed custom role permission set", async () => {
    renderAccessPage();

    fireEvent.click(await screen.findByRole("button", { name: /Auditor/ }));
    fireEvent.click(screen.getByLabelText(/staff\.read/i));
    fireEvent.click(screen.getByRole("button", { name: /save permissions/i }));

    await waitFor(() => {
      expect(accessMocks.replaceRolePermissions).toHaveBeenCalledWith("auditor", ["audit.read", "staff.read"]);
    });
    expect(screen.getByRole("status")).toHaveTextContent("Role permissions saved.");
  });

  it("creates a role and resets the creation form after success", async () => {
    accessMocks.createRole.mockResolvedValue({
      role_id: "project_reviewer",
      name: "Project Reviewer",
      description: "Reviews project reports",
      is_system_role: false,
      permissions: [],
    });
    renderAccessPage();

    fireEvent.click(await screen.findByRole("button", { name: /create role/i }));
    fireEvent.change(screen.getByPlaceholderText("e.g. project_reviewer"), { target: { value: "project_reviewer" } });
    fireEvent.change(screen.getAllByLabelText("Display name")[0], { target: { value: "Project Reviewer" } });
    fireEvent.change(screen.getAllByLabelText("Description")[0], { target: { value: "Reviews project reports" } });
    fireEvent.click(screen.getByRole("button", { name: /create role/i }));

    await waitFor(() => {
      expect(accessMocks.createRole).toHaveBeenCalledWith({
        role_id: "project_reviewer",
        name: "Project Reviewer",
        description: "Reviews project reports",
      });
    });
  });

  it("allows searching for a provisioned account and saving its role assignments", async () => {
    renderAccessPage();
    await screen.findByRole("button", { name: /^Auditor/ });
    await screen.findByText("Taylor Staff");

    fireEvent.change(screen.getByPlaceholderText("Search name, staff ID, or email"), { target: { value: "Taylor" } });
    fireEvent.click(screen.getByRole("button", { name: /^search$/i }));
    await waitFor(() => {
      expect(accessMocks.listUserRoleAssignments).toHaveBeenLastCalledWith({
        limit: 20,
        offset: 0,
        search: "Taylor",
      }, expect.any(AbortSignal));
    });

    fireEvent.click(screen.getByRole("button", { name: /Edit roles for Taylor Staff/i }));

    fireEvent.click(screen.getByRole("checkbox", { name: /Auditor/ }));
    fireEvent.click(screen.getByRole("button", { name: /save assignments/i }));

    await waitFor(() => {
      expect(accessMocks.replaceUserRoles).toHaveBeenCalledWith("target-user", ["staff", "auditor"]);
    });
  });

  it("shows an empty state when the staff access search returns no results", async () => {
    renderAccessPage();
    await screen.findByText("Taylor Staff");
    accessMocks.listUserRoleAssignments.mockResolvedValueOnce({ users: [], total: 0 });

    const searchInput = screen.getByPlaceholderText("Search name, staff ID, or email");
    fireEvent.change(searchInput, { target: { value: "Missing" } });
    fireEvent.click(screen.getByRole("button", { name: /^search$/i }));

    expect(await screen.findByText("No provisioned accounts found")).toBeInTheDocument();
  });

  it("does not present a failed directory load as an empty result", async () => {
    accessMocks.listUserRoleAssignments.mockRejectedValueOnce(new Error("network failure"));
    renderAccessPage();

    expect(await screen.findByText("Unable to load staff access")).toBeInTheDocument();
    expect(screen.queryByText("No provisioned accounts found")).not.toBeInTheDocument();
  });
});
