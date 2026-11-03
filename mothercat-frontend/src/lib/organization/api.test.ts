import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  createStaff,
  formatOrganizationError,
  listStaff,
  OrganizationApiError,
  provisionAdmin,
  uploadStaffCsv,
  uploadStaffProfilePicture,
} from "@/lib/organization/api";

const staffRecord = {
  id: "staff-profile-uuid",
  account_id: null,
  staff_id: "23847",
  department_id: null,
  full_name: "Ada Okafor",
  profile_image_url: "https://cdn.example.org/ada.jpg",
  work_email: "ada@example.org",
  job_title: "Site Engineer",
  employment_status: "pending",
};

const fetchMock = vi.fn<typeof fetch>();

function jsonResponse(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("organization API client", () => {
  it("explains authorization failures without showing a misleading support reference", () => {
    const error = new OrganizationApiError(
      "You do not have permission to perform this action.",
      403,
      "permission_denied",
      "e866e69e9b586cdbb752cdca042e88fb",
    );

    expect(formatOrganizationError(error, "Unable to complete the request.")).toBe(
      "Your account isn't authorized for this action. If you're a Super Admin, sign out and back in. If the problem continues, ask an administrator to verify your API permissions.",
    );
  });

  it("searches and filters staff with browser-managed session credentials", async () => {
    fetchMock.mockResolvedValue(jsonResponse({
      staff: [staffRecord],
      pagination: { limit: 25, offset: 0, total: 1 },
    }));

    const result = await listStaff({
      limit: 25,
      offset: 0,
      search: "23847",
      departmentId: "department-uuid",
      employmentStatus: "pending",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/staff?limit=25&offset=0&search=23847&department_id=department-uuid&employment_status=pending",
      expect.objectContaining({ credentials: "include", cache: "no-store" }),
    );
    expect(result).toEqual({
      staff: [staffRecord],
      total: 1,
      pagination: { limit: 25, offset: 0, total: 1 },
    });
  });

  it("sends staff creation fields as JSON without changing the displayed number", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ staff: staffRecord }, 201));

    await createStaff({
      staff_id: "23847",
      full_name: "Ada Okafor",
      work_email: "ada@example.org",
      job_title: "Site Engineer",
      department_id: null,
    });

    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/staff");
    expect(options?.method).toBe("POST");
    expect(options?.credentials).toBe("include");
    expect(new Headers(options?.headers).get("Content-Type")).toBe("application/json");
    expect(new Headers(options?.headers).get("Accept")).toBe("application/json");
    expect(options?.body).toBe(JSON.stringify({
      staff_id: "23847",
      full_name: "Ada Okafor",
      work_email: "ada@example.org",
      job_title: "Site Engineer",
      department_id: null,
    }));
  });

  it("provisions a pending staff account as Admin", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ invitation_sent: true, role: "admin" }));

    await provisionAdmin("staff-profile-uuid");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/staff/staff-profile-uuid/provision-admin",
      expect.objectContaining({ method: "POST", credentials: "include", cache: "no-store" }),
    );
  });

  it("lets the browser set the multipart boundary for CSV uploads", async () => {
    fetchMock.mockResolvedValue(jsonResponse({
      import: {
        id: "import-uuid",
        status: "completed",
        created_at: "2026-11-03T12:00:00Z",
        result: { total_rows: 1, imported_rows: 1, error_count: 0 },
      },
    }, 201));

    await uploadStaffCsv(new File(["staff_id,full_name\n23847,Ada Okafor"], "staff.csv", { type: "text/csv" }));

    const [, options] = fetchMock.mock.calls[0];
    expect(options?.credentials).toBe("include");
    expect(new Headers(options?.headers).has("Content-Type")).toBe(false);
    expect(options?.body).toBeInstanceOf(FormData);
  });

  it("uploads a staff profile picture by record UUID and lets the browser set the multipart boundary", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ staff: staffRecord }));
    const file = new File(["image-bytes"], "profile.png", { type: "image/png" });

    await uploadStaffProfilePicture("staff-profile-uuid", file);

    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/staff/staff-profile-uuid/profile-picture");
    expect(options?.method).toBe("POST");
    expect(options?.credentials).toBe("include");
    expect(new Headers(options?.headers).get("Accept")).toBe("application/json");
    expect(new Headers(options?.headers).has("Content-Type")).toBe(false);
    expect(options?.body).toBeInstanceOf(FormData);
    expect((options?.body as FormData).get("file")).toBe(file);
  });

  it("retains safe API error metadata and failed import details", async () => {
    fetchMock.mockResolvedValue(jsonResponse({
      error: { code: "staff_import_validation_failed", request_id: "support-reference" },
      import: {
        id: "import-uuid",
        status: "failed",
        created_at: "2026-11-03T12:00:00Z",
        result: {
          total_rows: 1,
          imported_rows: 0,
          error_count: 1,
          errors: [{ row: 2, staff_id: "123", code: "invalid_staff_record", message: "Review this row." }],
        },
      },
    }, 422));

    let caught: unknown;
    try {
      await uploadStaffCsv(new File(["bad"], "staff.csv", { type: "text/csv" }));
    } catch (error) {
      caught = error;
    }

    expect(caught).toBeInstanceOf(OrganizationApiError);
    expect(caught).toMatchObject({
      status: 422,
      code: "staff_import_validation_failed",
      requestId: "support-reference",
    });
    expect((caught as OrganizationApiError).payload).toMatchObject({ import: { status: "failed" } });
  });
});
