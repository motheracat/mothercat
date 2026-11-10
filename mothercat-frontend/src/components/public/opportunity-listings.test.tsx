import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { OpportunityListings } from "@/components/public/opportunity-listings";
import { opportunityStoreKey } from "@/lib/opportunities/demo-store";
import { installMemoryLocalStorage } from "@/test-utils/memory-local-storage";

beforeEach(() => {
  cleanup();
  installMemoryLocalStorage();
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
    configurable: true,
    value(this: HTMLDialogElement) {
      this.open = true;
    },
  });
});

afterEach(() => {
  cleanup();
  Reflect.deleteProperty(HTMLDialogElement.prototype, "showModal");
});

describe("OpportunityListings", () => {
  it("submits a SIWES application and records the selected document in demo storage", async () => {
    render(<OpportunityListings />);
    expect(await screen.findByText("Graduate Civil Engineer")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /apply for this placement/i }));
    fireEvent.change(screen.getByLabelText("Full name"), { target: { value: "Student Example" } });
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "student@example.edu" } });
    fireEvent.change(screen.getByLabelText("Phone"), { target: { value: "+2348000000000" } });
    fireEvent.change(screen.getByLabelText("Institution"), { target: { value: "Example University" } });
    fireEvent.change(screen.getByLabelText("Course / programme"), { target: { value: "Civil Engineering" } });
    fireEvent.change(screen.getByLabelText("Matriculation number"), { target: { value: "ENG/2024/001" } });
    const file = new File(["demo document"], "siwes-letter.pdf", { type: "application/pdf" });
    fireEvent.change(screen.getByLabelText(/Supporting document/), { target: { files: [file] } });
    fireEvent.submit(screen.getByRole("button", { name: "Submit demo application" }).closest("form")!);

    expect(await screen.findByText("Demo application submitted.")).toBeInTheDocument();
    const saved = JSON.parse(window.localStorage.getItem(opportunityStoreKey) ?? "{}") as {
      applications: Array<{ fullName: string; fileName: string }>;
    };
    expect(saved.applications).toHaveLength(1);
    expect(saved.applications[0]).toMatchObject({
      fullName: "Student Example",
      fileName: "siwes-letter.pdf",
    });
  });
});
