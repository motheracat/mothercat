import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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
  listStaff: vi.fn(),
}));

vi.mock("@/lib/access/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/access/api")>();
  return { ...actual, ...accessMocks };
});

vi.mock("@/lib/organization/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/organization/api")>();
  return { ...actual, listStaff: accessMocks.listStaff };
});

vi.mock("next/image", () => ({
  default: ({ src, className }: { src: string; className?: string }) => (
    <span data-image-source={src} className={className} />
  ),
}));

vi.mock("@/components/workspace/dialog", () => ({
  Dialog: ({
    open,
    title,
    children,
    actions,
  }: {
    open: boolean;
    title: string;
    children: React.ReactNode;
    actions?: React.ReactNode;
  }) => open ? (
    <section role="dialog" aria-label={title}>
      {children}
      {actions}
    </section>
  ) : null,
}));

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
      profile_image_url: null,
      employment_status: "active",
      account_active: true,
      role_ids: ["staff"],
    }],
    total: 1,
  });
  accessMocks.listStaff.mockResolvedValue({
    staff: [],
    total: 0,
    pagination: { limit: 100, offset: 0, total: 0 },
  });
});

function renderAccessPage(canReadStaff = true) {
  return render(
    <AccessManagement
      canRead
      canManage
      canAssign
      canReadStaff={canReadStaff}
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
    const createDialog = screen.getByRole("dialog", { name: "Create a role" });
    fireEvent.change(screen.getByPlaceholderText("e.g. project_reviewer"), { target: { value: "project_reviewer" } });
    fireEvent.change(within(createDialog).getByLabelText("Display name"), { target: { value: "Project Reviewer" } });
    fireEvent.change(within(createDialog).getByLabelText("Description"), { target: { value: "Reviews project reports" } });
    fireEvent.click(within(createDialog).getByRole("button", { name: /create role/i }));

    await waitFor(() => {
      expect(accessMocks.createRole).toHaveBeenCalledWith({
        role_id: "project_reviewer",
        name: "Project Reviewer",
        description: "Reviews project reports",
      });
    });
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Create a role" })).not.toBeInTheDocument());
  });

  it("uses the supplied skyline image in the workspace security box", async () => {
    const { container } = renderAccessPage(false);
    await screen.findByText("WORKSPACE SECURITY");

    expect(container.querySelector(".access-control-hero-image")).toHaveAttribute(
      "data-image-source",
      "/images/Blueprint%20Construction%20Skyline.png",
    );
  });

  it("keeps role creation errors visible inside the modal", async () => {
    accessMocks.createRole.mockRejectedValue(new Error("Role creation failed."));
    renderAccessPage();

    fireEvent.click(await screen.findByRole("button", { name: "Create role" }));
    const createDialog = screen.getByRole("dialog", { name: "Create a role" });
    fireEvent.change(within(createDialog).getByPlaceholderText("e.g. project_reviewer"), { target: { value: "project_reviewer" } });
    fireEvent.change(within(createDialog).getByLabelText("Display name"), { target: { value: "Project Reviewer" } });
    fireEvent.click(within(createDialog).getByRole("button", { name: "Create role" }));

    expect(await within(createDialog).findByRole("alert")).toHaveTextContent("Unable to create the role.");
    expect(screen.getByRole("dialog", { name: "Create a role" })).toBeInTheDocument();
  });

  it("allows searching for a provisioned account and saving its role assignments", async () => {
    renderAccessPage();
    await screen.findByRole("button", { name: /^Auditor/ });
    await screen.findByText("Taylor Staff");
    expect(screen.getByRole("img", { name: "Active" })).toBeInTheDocument();
    expect(screen.queryByText("Account active")).not.toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Account" })).not.toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Search name, staff ID, or email"), { target: { value: "Taylor" } });
    fireEvent.click(screen.getByRole("button", { name: /^search$/i }));
    await waitFor(() => {
      expect(accessMocks.listUserRoleAssignments).toHaveBeenLastCalledWith({
        limit: 20,
        offset: 0,
        search: "Taylor",
      }, expect.any(AbortSignal));
    });

    fireEvent.click(screen.getByRole("button", { name: /View account for Taylor Staff/i }));

    fireEvent.click(screen.getByRole("checkbox", { name: /Auditor/ }));
    fireEvent.click(screen.getByRole("button", { name: /save assignments/i }));

    await waitFor(() => {
      expect(accessMocks.replaceUserRoles).toHaveBeenCalledWith("target-user", ["staff", "auditor"]);
    });
  });

  it("loads the matching staff profile picture instead of leaving an initials avatar", async () => {
    accessMocks.listStaff.mockResolvedValue({
      staff: [{
        id: "staff-record",
        account_id: "target-user",
        staff_id: "12345",
        department_id: null,
        full_name: "Taylor Staff",
        profile_image_url: "https://res.cloudinary.com/example/taylor.png",
        work_email: "taylor@example.com",
        job_title: null,
        employment_status: "active",
      }],
      total: 1,
      pagination: { limit: 100, offset: 0, total: 1 },
    });
    const { container } = renderAccessPage();
    expect(await screen.findByText("Taylor Staff")).toBeInTheDocument();
    await waitFor(() => {
      expect(container.querySelector(".access-person-avatar [data-image-source]")).toHaveAttribute(
        "data-image-source",
        "https://res.cloudinary.com/example/taylor.png",
      );
    });
    expect(accessMocks.listStaff).toHaveBeenCalledWith(
      { limit: 100, offset: 0, search: "12345" },
      expect.any(AbortSignal),
    );
  });

  it("uses a three-dot menu for role management and lets account rows expand", async () => {
    renderAccessPage();
    await screen.findByText("Taylor Staff");

    fireEvent.click(screen.getByRole("button", { name: "More actions for Taylor Staff" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Manage roles" }));

    expect(await screen.findByRole("region", { name: "Role assignments for Taylor Staff" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Close account for Taylor Staff" })).toHaveAttribute("aria-expanded", "true");
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
