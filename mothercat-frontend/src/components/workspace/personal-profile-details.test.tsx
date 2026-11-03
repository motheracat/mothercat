import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { PersonalProfileDetails } from "@/components/workspace/personal-profile-details";
import {
  listStaff,
  updateStaff,
  type StaffMember,
} from "@/lib/organization/api";

vi.mock("@/lib/organization/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/organization/api")>();
  return {
    ...actual,
    listStaff: vi.fn(),
    updateStaff: vi.fn(),
  };
});

const staffRecord: StaffMember = {
  id: "staff-record-uuid",
  account_id: "account-uuid",
  staff_id: "23847",
  department_id: null,
  full_name: "Ada Okafor",
  profile_image_url: null,
  work_email: "ada@example.org",
  job_title: "Engineer",
  employment_status: "active",
};

function renderProfile() {
  return render(<PersonalProfileDetails staffNumber="23847" displayName="Ada Okafor" email="ada@example.org" />);
}

beforeEach(() => {
  vi.mocked(listStaff).mockReset();
  vi.mocked(updateStaff).mockReset();
});

afterEach(cleanup);

describe("PersonalProfileDetails", () => {
  it("copies the displayed staff ID and confirms the action", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    vi.mocked(listStaff).mockResolvedValue({
      staff: [staffRecord],
      total: 1,
      pagination: { limit: 100, offset: 0, total: 1 },
    });

    renderProfile();
    fireEvent.click(await screen.findByRole("button", { name: "Copy staff ID" }));

    await waitFor(() => expect(writeText).toHaveBeenCalledWith("23847"));
    expect(await screen.findByRole("status")).toHaveTextContent("Staff ID copied.");
  });

  it("explains when copying the staff ID is unavailable", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("Clipboard unavailable"));
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    vi.mocked(listStaff).mockResolvedValue({
      staff: [staffRecord],
      total: 1,
      pagination: { limit: 100, offset: 0, total: 1 },
    });

    renderProfile();
    fireEvent.click(await screen.findByRole("button", { name: "Copy staff ID" }));

    expect(await screen.findByRole("status")).toHaveTextContent("Select it and copy it manually");
  });

  it("shows both profile controls while the staff record is resolving", async () => {
    let resolveStaff!: (result: {
      staff: StaffMember[];
      total: number;
      pagination: { limit: number; offset: number; total: number };
    }) => void;
    vi.mocked(listStaff).mockReturnValue(new Promise((resolve) => { resolveStaff = resolve; }));

    renderProfile();

    expect(screen.getByRole("button", { name: "Checking staff record…" })).toBeDisabled();
    expect(screen.getByLabelText("Upload profile picture")).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent("Finding your staff record");

    resolveStaff({
      staff: [staffRecord],
      total: 1,
      pagination: { limit: 100, offset: 0, total: 1 },
    });

    await waitFor(() => expect(screen.getByRole("button", { name: "Edit profile" })).toBeEnabled());
    expect(screen.getByLabelText("Upload profile picture")).toBeEnabled();
  });

  it("uses an exact staff-number match and updates only self-service fields", async () => {
    vi.mocked(listStaff).mockResolvedValue({
      staff: [{ ...staffRecord, id: "other-record", staff_id: "238470" }, staffRecord],
      total: 2,
      pagination: { limit: 100, offset: 0, total: 2 },
    });
    vi.mocked(updateStaff).mockResolvedValue({
      ...staffRecord,
      full_name: "Ada N. Okafor",
      work_email: null,
    });

    renderProfile();
    expect(await screen.findByRole("button", { name: "Edit profile" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Edit profile" }));
    expect(screen.getByLabelText("Full name")).toHaveValue("Ada Okafor");
    expect(screen.getByLabelText("Work email")).toHaveValue("ada@example.org");
    expect(screen.queryByLabelText("Job title")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Department")).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Full name"), { target: { value: "Ada N. Okafor" } });
    fireEvent.change(screen.getByLabelText("Work email"), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(updateStaff).toHaveBeenCalledWith("staff-record-uuid", {
      full_name: "Ada N. Okafor",
      work_email: null,
    }));
    expect(await screen.findAllByText("Ada N. Okafor")).toHaveLength(2);
    expect(screen.queryByRole("button", { name: "Edit profile" })).toBeInTheDocument();
  });

  it("keeps edits open and preserves entered values when saving fails", async () => {
    vi.mocked(listStaff).mockResolvedValue({
      staff: [staffRecord],
      total: 1,
      pagination: { limit: 100, offset: 0, total: 1 },
    });
    vi.mocked(updateStaff).mockRejectedValue(new Error("The update could not be completed."));

    renderProfile();
    fireEvent.click(await screen.findByRole("button", { name: "Edit profile" }));
    fireEvent.change(screen.getByLabelText("Full name"), { target: { value: "New Name" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("The update could not be completed.");
    expect(screen.getByLabelText("Full name")).toHaveValue("New Name");
    expect(screen.getByRole("button", { name: "Save changes" })).toBeInTheDocument();
  });

  it("explains that editing is unavailable when no organization staff record matches", async () => {
    vi.mocked(listStaff).mockResolvedValue({
      staff: [{ ...staffRecord, staff_id: "238470" }],
      total: 1,
      pagination: { limit: 100, offset: 0, total: 1 },
    });

    renderProfile();

    expect(await screen.findByRole("alert")).toHaveTextContent("identity account alone is not enough");
    expect(screen.getByRole("button", { name: "Profile editing unavailable" })).toBeDisabled();
    expect(screen.getByLabelText("Profile picture unavailable")).toBeDisabled();
    expect(updateStaff).not.toHaveBeenCalled();
  });

  it("retries staff lookup without leaving My profile", async () => {
    vi.mocked(listStaff)
      .mockRejectedValueOnce(new Error("Staff lookup temporarily failed."))
      .mockResolvedValueOnce({
        staff: [staffRecord],
        total: 1,
        pagination: { limit: 100, offset: 0, total: 1 },
      });

    renderProfile();
    expect(await screen.findByRole("alert")).toHaveTextContent("Staff lookup temporarily failed.");
    fireEvent.click(await screen.findByRole("button", { name: "Retry profile lookup" }));

    await waitFor(() => expect(listStaff).toHaveBeenCalledTimes(2));
    expect(await screen.findByRole("button", { name: "Edit profile" })).toBeEnabled();
    expect(screen.getByLabelText("Upload profile picture")).toBeEnabled();
  });
});
