import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  getSiwesDocument,
  listPublicOpportunities,
  listSiwesApplications,
  submitSiwesApplication,
} from "@/lib/opportunities/api";

const opportunity = {
  id: "00000000-0000-4000-8000-000000000001",
  type: "siwes",
  title: "Industrial training",
  location: "Lagos",
  work_mode: "On-site",
  summary: "Supervised training",
  description: "Placement details",
  requirements: "",
  application_deadline: "2027-02-28",
  status: "published",
  created_at: "2026-11-01T09:00:00Z",
  updated_at: "2026-11-01T09:00:00Z",
};

const application = {
  id: "00000000-0000-4000-8000-000000000002",
  opportunity_id: opportunity.id,
  type: "siwes",
  full_name: "Student Example",
  email: "student@example.edu",
  phone: "+2348000000000",
  institution: "Example University",
  course: "Engineering",
  matric_number: "ENG/2024/001",
  message: "",
  status: "submitted",
  document: {
    file_name: "letter.pdf",
    content_type: "application/pdf",
    size_bytes: 123,
    download_available: true,
  },
  submitted_at: "2026-11-09T21:00:00Z",
};

const fetchMock = vi.fn<typeof fetch>();

function jsonResponse(payload: unknown, status = 200, headers?: HeadersInit) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "Content-Type": "application/json", ...Object.fromEntries(new Headers(headers)) },
  });
}

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("opportunities API client", () => {
  it("loads real public opportunities without using browser storage", async () => {
    fetchMock.mockResolvedValue(jsonResponse({
      opportunities: [opportunity],
      page: { limit: 100, offset: 0, total: 1 },
    }));

    await expect(listPublicOpportunities({ type: "siwes", limit: 100 })).resolves.toMatchObject({
      opportunities: [{ id: opportunity.id, application_deadline: "2027-02-28" }],
      page: { total: 1 },
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/opportunities?limit=100&offset=0&type=siwes",
      expect.objectContaining({ credentials: "include", cache: "no-store" }),
    );
  });

  it("uploads a real SIWES document as multipart without overriding its boundary", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ application }, 201));
    const file = new File(["%PDF-1.7"], "letter.pdf", { type: "application/pdf" });

    await expect(submitSiwesApplication(opportunity.id, {
      full_name: "Student Example",
      email: "student@example.edu",
      phone: "+2348000000000",
      institution: "Example University",
      course: "Engineering",
      matric_number: "ENG/2024/001",
      message: "",
      document: file,
    })).resolves.toMatchObject({ id: application.id });

    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe(`/api/v1/opportunities/${opportunity.id}/siwes-applications`);
    expect(options?.method).toBe("POST");
    expect(new Headers(options?.headers).has("Content-Type")).toBe(false);
    expect(options?.body).toBeInstanceOf(FormData);
    expect((options?.body as FormData).get("document")).toBe(file);
  });

  it("loads reviewer applications and retrieves documents for the in-app preview", async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ applications: [application], page: { limit: 25, offset: 0, total: 1 } }))
      .mockResolvedValueOnce(new Response(new Blob(["private file"]), {
        status: 200,
        headers: { "Content-Disposition": 'inline; filename="letter.pdf"', "Content-Type": "application/pdf" },
      }));

    await expect(listSiwesApplications({ status: "submitted" })).resolves.toMatchObject({
      applications: [{ id: application.id, document: { download_available: true } }],
    });
    const file = await getSiwesDocument(application.id);
    expect(file.fileName).toBe("letter.pdf");
    expect(file.contentType).toBe("application/pdf");
    expect(await file.blob.text()).toBe("private file");
    expect(fetchMock.mock.calls[1][0]).toBe(`/api/v1/siwes-applications/${application.id}/document`);
  });
});
