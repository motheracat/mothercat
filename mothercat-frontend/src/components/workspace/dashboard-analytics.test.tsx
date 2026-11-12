import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DashboardAnalytics } from "@/components/workspace/dashboard-analytics";

vi.mock("@/lib/analytics/api", () => ({
  getCompanyActivity: vi.fn(async (range: "7d" | "30d" | "90d") => ({
    range,
    totalActivities: 1,
    changePercent: 0,
    timeline: [{ date: "2026-11-12", label: "Nov 12", departments: [{ name: "Construction", value: 1 }] }],
    departments: [{ name: "Construction", activities: 1, tasksCompleted: 0, activeStaff: 1, trend: "steady" }],
    activityTypes: [{ name: "Updated", count: 1 }],
    heatmap: ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].map((day) => ({ day, hours: Array(8).fill(0) })),
  })),
  getMyActivity: vi.fn(async (range: "7d" | "30d" | "90d") => ({
    range,
    activityScore: 0,
    scoreLabel: "No activity yet",
    changePercent: 0,
    scoreFactors: [],
    dailyTrend: [],
    taskBreakdown: [],
    contributionWeeks: [],
    achievements: [],
  })),
  getPending: vi.fn(async () => ({
    totalPending: 0,
    byType: [],
    aging: [],
    departments: [],
    topItems: [],
  })),
}));

beforeEach(() => {
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
    configurable: true,
    value(this: HTMLDialogElement) {
      this.setAttribute("open", "");
      this.querySelector<HTMLElement>("button")?.focus();
    },
  });
  Object.defineProperty(HTMLDialogElement.prototype, "close", {
    configurable: true,
    value(this: HTMLDialogElement) {
      this.removeAttribute("open");
    },
  });
});

afterEach(() => {
  cleanup();
  Reflect.deleteProperty(HTMLDialogElement.prototype, "showModal");
  Reflect.deleteProperty(HTMLDialogElement.prototype, "close");
});

describe("dashboard analytics", () => {
  it("opens analytics in the glass modal, switches ranges, and restores card focus", async () => {
    render(<DashboardAnalytics />);
    const card = await screen.findByRole("button", { name: /Department activity/ });
    const focusSpy = vi.spyOn(card, "focus");
    fireEvent.click(card);

    const dialog = await screen.findByRole("dialog", { name: "Department activity" });
    expect(await screen.findByRole("heading", { name: "Activity over time" })).toBeInTheDocument();

    const weekRange = screen.getByRole("button", { name: "7d" });
    fireEvent.click(weekRange);
    expect(weekRange).toHaveAttribute("aria-pressed", "true");

    fireEvent.click(screen.getByRole("button", { name: "Close dialog" }));
    await waitFor(() => {
      expect(dialog).not.toHaveAttribute("open");
      expect(focusSpy).toHaveBeenCalled();
    });
  });

  it("opens the personal and pending expanded views", async () => {
    render(<DashboardAnalytics />);

    fireEvent.click(await screen.findByRole("button", { name: /My activity/ }));
    expect(await screen.findByRole("heading", { name: "How your activity score is calculated" })).toBeInTheDocument();
    fireEvent(screen.getByRole("dialog"), new Event("cancel", { cancelable: true }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Pending analysis/ }));
    expect(await screen.findByRole("heading", { name: "Needs attention" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close dialog" }));
  });
});
