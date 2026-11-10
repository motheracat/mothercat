import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { OpportunityManagement } from "@/components/workspace/opportunity-management";
import { opportunityStoreKey } from "@/lib/opportunities/demo-store";
import { installMemoryLocalStorage } from "@/test-utils/memory-local-storage";

beforeEach(() => {
  cleanup();
  installMemoryLocalStorage();
});

afterEach(() => cleanup());

describe("OpportunityManagement", () => {
  it("creates a draft and publishes it to the shared demo listings", async () => {
    render(
      <OpportunityManagement
        canPublish
        canReviewApplications={false}
        canReviewSiwes={false}
      />,
    );

    expect(await screen.findByText("Graduate Civil Engineer")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Opportunity type"), { target: { value: "siwes" } });
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Student placement test" } });
    fireEvent.change(screen.getByLabelText("Location"), { target: { value: "Lagos" } });
    fireEvent.change(screen.getByLabelText("Work arrangement"), { target: { value: "On-site" } });
    fireEvent.change(screen.getByLabelText("Short summary"), { target: { value: "Practical project experience." } });
    fireEvent.change(screen.getByLabelText("Description"), { target: { value: "Join an engineering project team." } });
    fireEvent.change(screen.getByLabelText("Requirements"), { target: { value: "Currently enrolled in a relevant programme." } });
    fireEvent.change(screen.getByLabelText("Application deadline"), { target: { value: "2027-03-30" } });
    fireEvent.submit(screen.getByRole("button", { name: "Save as draft" }).closest("form")!);

    expect(await screen.findByText("Student placement test")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("saved as a draft");
    fireEvent.click(screen.getByRole("button", { name: "Publish" }));

    await waitFor(() => {
      const saved = JSON.parse(window.localStorage.getItem(opportunityStoreKey) ?? "{}") as {
        opportunities: Array<{ title: string; status: string }>;
      };
      expect(saved.opportunities.find((item) => item.title === "Student placement test")?.status).toBe("published");
    });
  });
});
