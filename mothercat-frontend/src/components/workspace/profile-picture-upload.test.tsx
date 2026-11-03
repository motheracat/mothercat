import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ProfilePictureUpload } from "@/components/workspace/profile-picture-upload";
import {
  listStaff,
  OrganizationApiError,
  uploadStaffProfilePicture,
  type StaffMember,
} from "@/lib/organization/api";

vi.mock("@/lib/organization/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/organization/api")>();
  return {
    ...actual,
    listStaff: vi.fn(),
    uploadStaffProfilePicture: vi.fn(),
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
  job_title: null,
  employment_status: "active",
};

function renderUploader(props: Partial<React.ComponentProps<typeof ProfilePictureUpload>> = {}) {
  return render(
    <ProfilePictureUpload
      staffNumber="23847"
      name="Ada Okafor"
      profileImageUrl={null}
      avatarWrapClassName="avatar-wrap"
      avatarClassName="avatar"
      {...props}
    />,
  );
}

beforeEach(() => {
  vi.mocked(listStaff).mockReset();
  vi.mocked(uploadStaffProfilePicture).mockReset();
});

afterEach(cleanup);

describe("ProfilePictureUpload", () => {
  it("uses only the exact staff number match and displays the persisted upload URL", async () => {
    vi.mocked(listStaff).mockResolvedValue({
      staff: [
        { ...staffRecord, id: "other-record", staff_id: "238470" },
        staffRecord,
      ],
      total: 2,
      pagination: { limit: 100, offset: 0, total: 2 },
    });
    vi.mocked(uploadStaffProfilePicture).mockResolvedValue({
      ...staffRecord,
      profile_image_url: "https://res.cloudinary.com/example/profile.png",
    });
    const file = new File(["image"], "profile.png", { type: "image/png" });

    renderUploader();
    await waitFor(() => expect(listStaff).toHaveBeenCalledWith({ limit: 100, offset: 0, search: "23847" }));
    fireEvent.change(screen.getByLabelText("Upload profile picture"), { target: { files: [file] } });

    await waitFor(() => expect(uploadStaffProfilePicture).toHaveBeenCalledWith("staff-record-uuid", file));
    expect(await screen.findByText("Profile picture updated.")).toBeInTheDocument();
    await waitFor(() => expect(document.querySelector("img")?.getAttribute("src")).toContain("res.cloudinary.com"));
    expect(screen.queryByText("AO")).not.toBeInTheDocument();
  });

  it("shows an actionable error and never uploads when an exact staff-number match is missing", async () => {
    vi.mocked(listStaff).mockResolvedValue({
      staff: [{ ...staffRecord, staff_id: "238470" }],
      total: 1,
      pagination: { limit: 100, offset: 0, total: 1 },
    });

    renderUploader();

    expect(await screen.findByRole("alert")).toHaveTextContent("identity account alone is not enough");
    expect(screen.getByLabelText("Profile picture unavailable")).toBeDisabled();
    expect(uploadStaffProfilePicture).not.toHaveBeenCalled();
  });

  it("retains the initials fallback and rejects empty, unsupported, and oversized files", async () => {
    vi.mocked(listStaff).mockResolvedValue({
      staff: [staffRecord],
      total: 1,
      pagination: { limit: 100, offset: 0, total: 1 },
    });
    renderUploader();
    const input = await screen.findByLabelText("Upload profile picture");

    fireEvent.change(input, { target: { files: [new File([], "empty.png", { type: "image/png" })] } });
    expect(await screen.findByRole("alert")).toHaveTextContent("non-empty");
    fireEvent.change(input, { target: { files: [new File(["text"], "notes.txt", { type: "text/plain" })] } });
    expect(await screen.findByRole("alert")).toHaveTextContent("Choose a JPEG, PNG, or GIF image.");
    fireEvent.change(input, {
      target: { files: [new File([new Uint8Array(5 * 1024 * 1024 + 1)], "large.png", { type: "image/png" })] },
    });
    expect(await screen.findByRole("alert")).toHaveTextContent("no larger than 5 MiB");
    expect(screen.getByText("AO")).toBeInTheDocument();
    expect(uploadStaffProfilePicture).not.toHaveBeenCalled();
  });

  it("falls back to initials when the persisted image cannot be loaded", () => {
    renderUploader({ staffId: staffRecord.id, profileImageUrl: "https://cdn.example.org/broken.png" });

    fireEvent.error(document.querySelector("img") as HTMLImageElement);

    expect(screen.getByText("AO")).toBeInTheDocument();
  });

  it("prevents duplicate submissions while an upload is in progress", async () => {
    vi.mocked(listStaff).mockResolvedValue({
      staff: [staffRecord],
      total: 1,
      pagination: { limit: 100, offset: 0, total: 1 },
    });
    let finishUpload!: (staff: StaffMember) => void;
    vi.mocked(uploadStaffProfilePicture).mockReturnValue(new Promise((resolve) => { finishUpload = resolve; }));
    renderUploader({ staffNumber: undefined, staffId: staffRecord.id });
    const input = screen.getByLabelText("Upload profile picture");
    const file = new File(["image"], "profile.png", { type: "image/png" });

    fireEvent.change(input, { target: { files: [file] } });
    await screen.findByText("Uploading profile picture…");
    fireEvent.change(input, { target: { files: [file] } });
    expect(uploadStaffProfilePicture).toHaveBeenCalledTimes(1);
    finishUpload(staffRecord);
    expect(await screen.findByText("Profile picture updated.")).toBeInTheDocument();
  });

  it.each([
    [400, "unsupported_image_type", "Choose a valid JPEG, PNG, or GIF image."],
    [413, "request_too_large", "no larger than 5 MiB"],
    [401, "unauthorized", "Your session has expired. Sign in again."],
    [403, "permission_denied", "You do not have permission to change this profile picture."],
    [404, "staff_not_found", "This staff record could not be found."],
    [502, "image_upload_failed", "Support reference: support-ref"],
    [503, "service_unavailable", "temporarily unavailable"],
  ])("shows an understandable message for upload failure %s/%s", async (status, code, message) => {
    vi.mocked(uploadStaffProfilePicture).mockRejectedValue(
      new OrganizationApiError("Upload failed", status, code, status === 502 ? "support-ref" : undefined),
    );
    renderUploader({ staffId: staffRecord.id });
    fireEvent.change(screen.getByLabelText("Upload profile picture"), {
      target: { files: [new File(["image"], "profile.png", { type: "image/png" })] },
    });

    expect(await screen.findByRole("alert")).toHaveTextContent(message);
  });

  it("keeps the previous image when the upload service fails", async () => {
    vi.mocked(uploadStaffProfilePicture).mockRejectedValue(
      new OrganizationApiError("Upload failed", 503, "service_unavailable"),
    );
    renderUploader({ staffId: staffRecord.id, profileImageUrl: "https://cdn.example.org/previous.png" });
    fireEvent.change(screen.getByLabelText("Change profile picture"), {
      target: { files: [new File(["image"], "profile.png", { type: "image/png" })] },
    });

    expect(await screen.findByRole("alert")).toHaveTextContent("temporarily unavailable");
    expect(document.querySelector("img")?.getAttribute("src")).toContain("previous.png");
  });
});
