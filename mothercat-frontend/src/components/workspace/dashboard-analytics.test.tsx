import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DashboardAnalytics } from "@/components/workspace/dashboard-analytics";
import { getCompanyActivity, getMyActivity, getPending } from "@/lib/analytics/api";

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
  it("returns deterministic mock responses for each supported range", async () => {
    const responses = await Promise.all((["7d", "30d", "90d"] as const).map(async (range) => ({
      company: await getCompanyActivity(range),
      personal: await getMyActivity(range),
      pending: await getPending(range),
    })));

    for (const { company, personal, pending } of responses) {
      expect(company.departments.reduce((sum, item) => sum + item.activities, 0)).toBe(company.totalActivities);
      expect(company.timeline.reduce(
        (sum, point) => sum + point.departments.reduce((pointSum, item) => pointSum + item.value, 0),
        0,
      )).toBe(company.totalActivities);
      expect(company.activityTypes.reduce((sum, item) => sum + item.count, 0)).toBe(company.totalActivities);
      expect(personal.dailyTrend.reduce((sum, item) => sum + item.value, 0)).toBe(
        personal.scoreFactors.find((item) => item.name === "Activities performed")?.value,
      );
      expect(personal.contributionWeeks).toHaveLength(12);
      expect(pending.totalPending).toBe(pending.byType.reduce((sum, item) => sum + item.count, 0));
      expect(pending.totalPending).toBe(pending.aging.reduce((sum, item) => sum + item.count, 0));
      expect(pending.totalPending).toBe(pending.departments.reduce((sum, item) => sum + item.count, 0));
    }

    expect(responses[1].company.departments.map((item) => item.name)).toContain("Civil Engineering");
    expect(responses[0].company.totalActivities).not.toBe(responses[1].company.totalActivities);
  });

  it("opens analytics in the glass modal, switches ranges, and restores card focus", async () => {
    render(<DashboardAnalytics />);
    const card = await screen.findByRole("button", { name: /Department activity/ });
    expect(screen.queryByText("Demo data")).not.toBeInTheDocument();
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
