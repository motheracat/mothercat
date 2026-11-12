import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OpportunityManagement } from "@/components/workspace/opportunity-management";

const api = vi.hoisted(() => ({
  changeOpportunityStatus: vi.fn(),
  createOpportunity: vi.fn(),
  formatOpportunityError: vi.fn((_error: unknown, fallback: string) => fallback),
  getSiwesApplication: vi.fn(),
  getSiwesDocument: vi.fn(),
  listManagedOpportunities: vi.fn(),
  listSiwesApplications: vi.fn(),
  updateOpportunity: vi.fn(),
  updateSiwesApplication: vi.fn(),
}));

vi.mock("@/lib/opportunities/api", () => api);

vi.mock("@/components/workspace/dialog", () => ({
  Dialog: ({
    open,
    title,
    children,
    actions,
  }: {
    open: boolean;
    title: string;
    children: ReactNode;
    actions?: ReactNode;
  }) => open ? (
    <section role="dialog" aria-label={title}>
      <h2>{title}</h2>
      {children}
      {actions}
    </section>
  ) : null,
}));

const application = {
  id: "00000000-0000-4000-8000-000000000002",
  opportunity_id: "00000000-0000-4000-8000-000000000001",
  type: "siwes" as const,
  full_name: "Student Example",
  email: "student@example.edu",
  phone: "+2348000000000",
  institution: "Example University",
  course: "Engineering",
  matric_number: "ENG/2024/001",
  message: "",
  status: "submitted" as const,
  reviewer_note: "",
  document: {
    file_name: "letter.pdf",
    content_type: "application/pdf",
    size_bytes: 123,
    download_available: true,
  },
  submitted_at: "2026-11-09T21:00:00Z",
  reviewed_at: null,
};

beforeEach(() => {
  api.listManagedOpportunities.mockResolvedValue({
    opportunities: [],
    page: { limit: 25, offset: 0, total: 0 },
  });
  api.listSiwesApplications.mockResolvedValue({
    applications: [application],
    page: { limit: 25, offset: 0, total: 1 },
  });
  api.getSiwesApplication.mockResolvedValue(application);
  api.getSiwesDocument.mockResolvedValue({
    blob: new Blob(["%PDF-1.7"], { type: "application/pdf" }),
    fileName: "letter.pdf",
    contentType: "application/pdf",
  });
  vi.stubGlobal("URL", Object.assign(class extends URL {}, {
    createObjectURL: vi.fn(() => "blob:siwes-document"),
    revokeObjectURL: vi.fn(),
  }));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("SIWES application document review", () => {
  it("opens the document preview in a modal without navigating to a new tab", async () => {
    render(
      <OpportunityManagement
        canPublish={false}
        canReviewApplications={false}
        canReviewSiwes
      />,
    );

    fireEvent.click(await screen.findByRole("button", { name: "Review" }));
    await screen.findByText("Student Example");
    fireEvent.click(await screen.findByRole("button", { name: "View document" }));

    const previewDialog = await screen.findByRole("dialog", { name: "letter.pdf" });
    expect(await screen.findByTitle("Preview of letter.pdf")).toHaveAttribute("src", "blob:siwes-document");
    expect(previewDialog.querySelector('a[download="letter.pdf"]')).toBeInTheDocument();
    expect(previewDialog.querySelector('a[target="_blank"]')).not.toBeInTheDocument();
    await waitFor(() => expect(api.getSiwesDocument).toHaveBeenCalledWith(application.id));
  });
});
