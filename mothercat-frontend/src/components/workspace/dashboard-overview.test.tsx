import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { DashboardOverview } from "@/components/workspace/dashboard-overview";
import { listDepartments, listStaff, type StaffMember } from "@/lib/organization/api";

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

describe("DashboardOverview", () => {
  it("shows a capped, overlapping stack of fetched staff photos instead of the total staff count", async () => {
    render(<DashboardOverview />);

    const peopleLink = await screen.findByRole("link", { name: "Browse staff profiles" });
    await waitFor(() => {
      expect(peopleLink.querySelectorAll("[data-image-source]")).toHaveLength(5);
    });

    expect(peopleLink.querySelector(".dashboard-people-more")).toBeInTheDocument();
    expect(screen.queryByText("Total staff")).not.toBeInTheDocument();
    expect(screen.queryByText("23")).not.toBeInTheDocument();
    expect(listStaff).toHaveBeenCalledWith({ limit: 5, offset: 0 });
  });
});
