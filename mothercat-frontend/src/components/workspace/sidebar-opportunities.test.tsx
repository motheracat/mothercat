import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { WorkspaceSidebar } from "@/components/workspace/sidebar";
import type { AuthSession } from "@/lib/auth/types";

vi.mock("next/image", () => ({
  default: ({ src, alt }: { src: string; alt: string }) => (
    <span aria-label={alt} data-image-source={src} />
  ),
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
}));

vi.mock("@/components/workspace/modal-menu", () => ({
  WorkspaceModalMenu: ({
    open,
    children,
    title,
  }: {
    open: boolean;
    children: React.ReactNode;
    title: string;
  }) => open ? <div role="dialog" aria-label={title}>{children}</div> : null,
}));

const session: AuthSession = {
  userId: "admin-account",
  staffId: "12345",
  displayName: "Admin User",
  email: "admin@example.org",
  role: "Admin",
  roles: ["Admin"],
  permissions: ["roles.manage"],
  isActive: true,
};

describe("WorkspaceSidebar opportunity access", () => {
  it("shows Opportunities & SIWES in desktop navigation and the mobile More menu for admins", () => {
    render(<WorkspaceSidebar session={session} signOut={<button type="button">Sign out</button>} />);

    expect(screen.getByRole("link", { name: "Opportunities & SIWES" })).toHaveAttribute(
      "href",
      "/opportunities/manage",
    );
    fireEvent.click(screen.getByRole("button", { name: "More navigation" }));
    expect(screen.getByRole("dialog", { name: "More navigation" })).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "Opportunities & SIWES" })).toHaveLength(2);
    expect(screen.getAllByRole("link", { name: "Opportunities & SIWES" })[1]).toHaveAttribute("href", "/opportunities/manage");
    expect(screen.getAllByRole("link", { name: "Password & security" })).toHaveLength(2);
    expect(screen.getAllByRole("link", { name: "Password & security" })[1]).toHaveAttribute("href", "/account");
  });
});
