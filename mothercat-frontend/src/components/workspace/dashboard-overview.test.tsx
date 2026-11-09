import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DashboardOverview } from "@/components/workspace/dashboard-overview";
import { listAuditEvents } from "@/lib/audit/api";
import { listDepartments, listStaff, type StaffMember } from "@/lib/organization/api";

vi.mock("@/lib/audit/api", () => ({
  formatAuditError: (_error: unknown, fallback: string) => fallback,
  listAuditEvents: vi.fn(),
}));

vi.mock("@/lib/organization/api", () => ({
  formatOrganizationError: (_error: unknown, fallback: string) => fallback,
  listDepartments: vi.fn(),
  listStaff: vi.fn(),
}));

vi.mock("next/image", () => ({
  default: ({ src, className }: { src: string; className?: string }) => (
    <span data-image-source={src} className={className} />
  ),
}));

function makeStaff(index: number): StaffMember {
  return {
    id: `staff-${index}`,
    account_id: `account-${index}`,
    staff_id: `${10000 + index}`,
    department_id: null,
    full_name: `Staff Member ${index}`,
    profile_image_url: `https://res.cloudinary.com/example/staff-${index}.png`,
    work_email: `staff${index}@example.org`,
    job_title: null,
    employment_status: "active",
  };
}

beforeEach(() => {
  cleanup();
  vi.mocked(listAuditEvents).mockReset();
  vi.mocked(listAuditEvents).mockResolvedValue({ events: [], total: 0, limit: 5, offset: 0 });
  vi.mocked(listDepartments).mockReset();
  vi.mocked(listStaff).mockReset();
  vi.mocked(listDepartments).mockResolvedValue([]);
  vi.mocked(listStaff).mockImplementation(async (params) => {
    const staff = Array.from({ length: 5 }, (_, index) => makeStaff(index + 1));
    return {
      staff: params.limit === 1 ? [] : staff,
      total: params.employmentStatus === "active" ? 18 : params.employmentStatus === "pending" ? 3 : 23,
      pagination: { limit: params.limit, offset: params.offset, total: 23 },
    };
  });
});

afterEach(() => {
  Reflect.deleteProperty(HTMLDialogElement.prototype, "showModal");
  vi.restoreAllMocks();
});

describe("DashboardOverview", () => {
  it("shows a capped, overlapping stack of fetched staff photos instead of the total staff count", async () => {
    render(<DashboardOverview canReadAudit={false} />);

    const peopleLink = await screen.findByRole("link", { name: "Browse staff profiles" });
    await waitFor(() => {
      expect(peopleLink.querySelectorAll("[data-image-source]")).toHaveLength(5);
    });

    expect(peopleLink.querySelector(".dashboard-people-more")).toBeInTheDocument();
    expect(screen.queryByText("Total staff")).not.toBeInTheDocument();
    expect(screen.queryByText("23")).not.toBeInTheDocument();
    expect(listStaff).toHaveBeenCalledWith({ limit: 5, offset: 0 });
  });

  it("shows the latest audit events on the dashboard for readers", async () => {
    vi.mocked(listAuditEvents).mockResolvedValue({
      events: [{
        id: "event-1",
        actor_type: "user",
        actor_id: "account-1",
        actor_name: "Ada Okafor",
        action: "staff.updated",
        resource_type: "staff_member",
        resource_id: "staff-1",
        resource_name: "Sam Rivera",
        occurred_at: "2026-11-04T10:00:00Z",
        details: null,
      }],
      total: 1,
      limit: 5,
      offset: 0,
    });

    render(<DashboardOverview canReadAudit />);

    expect(await screen.findByText("Staff Updated")).toBeInTheDocument();
    expect(screen.getByText("Ada Okafor")).toBeInTheDocument();
    expect(screen.getByText("Sam Rivera")).toBeInTheDocument();
    expect(listAuditEvents).toHaveBeenCalledWith({ limit: 5, offset: 0 }, expect.any(AbortSignal));
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.open = true;
      },
    });
    fireEvent.click(screen.getByRole("button", { name: /view details for staff updated by ada okafor/i }));
    expect(screen.getByRole("dialog", { name: "Audit event details" })).toBeInTheDocument();
    expect(screen.getByText("Change details")).toBeInTheDocument();
  });

  it("does not request audit events for accounts without permission", async () => {
    render(<DashboardOverview canReadAudit={false} />);

    expect(await screen.findByText(/available to accounts with the audit\.read permission/i)).toBeInTheDocument();
    expect(listAuditEvents).not.toHaveBeenCalled();
  });
});
