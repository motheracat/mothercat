import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { WorkspaceShell } from "@/components/workspace/workspace-shell";
import type { AuthSession } from "@/lib/auth/types";
import { listStaff, type StaffMember } from "@/lib/organization/api";

vi.mock("next/image", () => ({
  default: ({
    src,
    className,
  }: {
    src: string;
    className?: string;
  }) => <span data-image-source={src} className={className} />,
}));

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...props
  }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...props}>{children}</a>
  ),
}));

vi.mock("next/navigation", () => ({
  usePathname: () => "/dashboard",
  useRouter: () => ({ replace: vi.fn() }),
}));

vi.mock("@/components/workspace/sidebar", () => ({
  WorkspaceSidebar: () => null,
}));

vi.mock("@/lib/auth/client", () => ({
  formatAuthError: (_error: unknown, fallback: string) => fallback,
  logout: vi.fn(),
}));

vi.mock("@/lib/organization/api", () => ({
  listStaff: vi.fn(),
}));

const session: AuthSession = {
  userId: "account-uuid",
  staffId: "23847",
  displayName: "Ada Okafor",
  email: "ada@example.org",
  role: "Staff",
  roles: ["Staff"],
  permissions: [],
  isActive: true,
};

const staffRecord: StaffMember = {
  id: "staff-record-uuid",
  account_id: session.userId,
  staff_id: session.staffId,
  department_id: null,
  full_name: session.displayName,
  profile_image_url: "https://res.cloudinary.com/example/profile.png",
  work_email: session.email,
  job_title: null,
  employment_status: "active",
};

function renderShell() {
  return render(
    <WorkspaceShell session={session} devOpen={false}>
      <p>Dashboard content</p>
    </WorkspaceShell>,
  );
}

describe("WorkspaceShell profile avatar", () => {
  beforeEach(() => {
    vi.mocked(listStaff).mockReset();
  });

  it("loads the image from the exact matching staff record", async () => {
    vi.mocked(listStaff).mockResolvedValue({
      staff: [staffRecord],
      total: 1,
      pagination: { limit: 100, offset: 0, total: 1 },
    });
    const { container } = renderShell();

    await waitFor(() => {
      expect(container.querySelector(".user-menu-avatar [data-image-source]")).toHaveAttribute("data-image-source", staffRecord.profile_image_url);
    });
    expect(listStaff).toHaveBeenCalledWith({ limit: 100, offset: 0, search: session.staffId });

    fireEvent.click(screen.getByRole("button", { name: `User menu for ${session.displayName}` }));
    await waitFor(() => {
      expect(document.querySelector(".workspace-modal-avatar [data-image-source]")).toHaveAttribute(
        "data-image-source",
        staffRecord.profile_image_url,
      );
    });
  });

  it("does not display another staff member's image when search results are not an exact match", async () => {
    vi.mocked(listStaff).mockResolvedValue({
      staff: [{ ...staffRecord, staff_id: "238470" }],
      total: 1,
      pagination: { limit: 100, offset: 0, total: 1 },
    });
    const { container } = renderShell();

    await waitFor(() => expect(listStaff).toHaveBeenCalled());
    expect(container.querySelector(".user-menu-avatar [data-image-source]")).not.toBeInTheDocument();
    expect(container.querySelector(".user-menu-avatar")).toHaveTextContent("AO");
  });

  it("updates the navbar image after the signed-in user's picture changes", async () => {
    vi.mocked(listStaff).mockResolvedValue({
      staff: [staffRecord],
      total: 1,
      pagination: { limit: 100, offset: 0, total: 1 },
    });
    const { container } = renderShell();
    await waitFor(() => expect(container.querySelector(".user-menu-avatar [data-image-source]")).toBeInTheDocument());

    window.dispatchEvent(new CustomEvent("mothercat:staff-profile-updated", {
      detail: { ...staffRecord, profile_image_url: "https://res.cloudinary.com/example/new-profile.png" },
    }));

    await waitFor(() => {
      expect(container.querySelector(".user-menu-avatar [data-image-source]")).toHaveAttribute(
        "data-image-source",
        "https://res.cloudinary.com/example/new-profile.png",
      );
    });
  });
});
