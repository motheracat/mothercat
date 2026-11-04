import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AuditLog } from "@/components/workspace/audit-log";

const auditMocks = vi.hoisted(() => ({
  listAuditEvents: vi.fn(),
  getAuditEvent: vi.fn(),
}));

vi.mock("@/lib/audit/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/audit/api")>();
  return { ...actual, ...auditMocks };
});

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

const event = {
  id: "00000000-0000-4000-8000-000000000001",
  actor_type: "user",
  actor_id: "account-1",
  action: "staff.updated",
  resource_type: "staff_member",
  resource_id: "staff-1",
  occurred_at: "2026-11-04T10:00:00Z",
  details: { field: "job_title", new_value: "Site Engineer" },
};

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  auditMocks.listAuditEvents.mockResolvedValue({
    events: [event],
    total: 1,
    limit: 25,
    offset: 0,
  });
  auditMocks.getAuditEvent.mockResolvedValue(event);
});

describe("AuditLog", () => {
  it("loads real audit events and opens event details", async () => {
    render(<AuditLog canRead />);

    expect(await screen.findByText("staff.updated")).toBeInTheDocument();
    expect(screen.getByText("account-1")).toBeInTheDocument();
    expect(auditMocks.listAuditEvents).toHaveBeenCalledWith(
      { limit: 25, offset: 0 },
      expect.any(AbortSignal),
    );

    fireEvent.click(screen.getByRole("button", { name: "View details for staff.updated by account-1" }));
    expect(await screen.findByRole("dialog", { name: "Audit event details" })).toBeInTheDocument();
    expect(auditMocks.getAuditEvent).toHaveBeenCalledWith(event.id, expect.any(AbortSignal));
    expect(await screen.findByText(/"new_value": "Site Engineer"/)).toBeInTheDocument();
  });

  it("applies exact filters and resets to the first result page", async () => {
    auditMocks.listAuditEvents
      .mockResolvedValueOnce({ events: [event], total: 40, limit: 25, offset: 0 })
      .mockResolvedValueOnce({ events: [event], total: 1, limit: 25, offset: 0 });
    render(<AuditLog canRead />);
    await screen.findByText("staff.updated");

    fireEvent.change(screen.getByLabelText("Actor ID"), { target: { value: " account-1 " } });
    fireEvent.change(screen.getByLabelText("Resource type"), { target: { value: "staff_member" } });
    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));

    await waitFor(() => {
      expect(auditMocks.listAuditEvents).toHaveBeenLastCalledWith({
        limit: 25,
        offset: 0,
        actorId: "account-1",
        resourceType: "staff_member",
      }, expect.any(AbortSignal));
    });
  });

  it("does not call the API without audit permission", () => {
    render(<AuditLog canRead={false} />);
    expect(screen.getByText(/audit.read permission/i)).toBeInTheDocument();
    expect(auditMocks.listAuditEvents).not.toHaveBeenCalled();
  });

  it("shows a real empty state and distinguishes it from load errors", async () => {
    auditMocks.listAuditEvents.mockResolvedValue({ events: [], total: 0, limit: 25, offset: 0 });
    render(<AuditLog canRead />);
    expect(await screen.findByText("No audit events are available yet.")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("offers a retry instead of presenting a failed request as an empty result", async () => {
    auditMocks.listAuditEvents
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({ events: [event], total: 1, limit: 25, offset: 0 });
    render(<AuditLog canRead />);

    expect(await screen.findByText("Unable to load audit events.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("staff.updated")).toBeInTheDocument();
    expect(auditMocks.listAuditEvents).toHaveBeenCalledTimes(2);
  });

  it("validates date ranges before requesting results", async () => {
    render(<AuditLog canRead />);
    await screen.findByText("staff.updated");

    fireEvent.change(screen.getByLabelText("From"), { target: { value: "2026-11-04T10:00" } });
    fireEvent.change(screen.getByLabelText("To"), { target: { value: "2026-11-03T10:00" } });
    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent("The start date must be before the end date.");
    expect(auditMocks.listAuditEvents).toHaveBeenCalledTimes(1);
  });
});
