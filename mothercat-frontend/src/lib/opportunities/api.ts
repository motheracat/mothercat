import { buildApiUrl } from "@/lib/api/config";
import { apiRoutes } from "@/lib/api/routes";
import { fetchWithRetry } from "@/lib/api/retry";

export type OpportunityType = "job" | "siwes";
export type OpportunityStatus = "draft" | "published" | "closed";
export type SiwesApplicationStatus = "submitted" | "under_review" | "shortlisted" | "accepted" | "rejected";

export type Opportunity = {
  id: string;
  type: OpportunityType;
  title: string;
  location: string;
  work_mode: string;
  summary: string;
  description: string;
  requirements: string;
  application_deadline: string | null;
  status: OpportunityStatus;
  created_at: string;
  updated_at: string;
};

export type OpportunityPage = { limit: number; offset: number; total: number };
export type OpportunityListResult = { opportunities: Opportunity[]; page: OpportunityPage };

export type SiwesDocument = {
  file_name: string;
  content_type: string;
  size_bytes: number;
  download_available: boolean;
};

export type SiwesApplication = {
  id: string;
  opportunity_id: string;
  type: "siwes";
  full_name: string;
  email: string;
  phone: string;
  institution: string;
  course: string;
  matric_number: string;
  message: string;
  status: SiwesApplicationStatus;
  reviewer_note: string;
  document: SiwesDocument;
  submitted_at: string;
  reviewed_at: string | null;
};

export type SiwesApplicationListResult = {
  applications: SiwesApplication[];
  page: OpportunityPage;
};

export type OpportunityInput = Omit<
  Opportunity,
  "id" | "status" | "created_at" | "updated_at"
>;

export class OpportunityApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "OpportunityApiError";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function requiredString(record: Record<string, unknown>, key: string): string {
  if (typeof record[key] !== "string") {
    throw new OpportunityApiError("The opportunities service returned invalid data.", 502, "invalid_response");
  }
  return record[key];
}

function parsePage(value: unknown): OpportunityPage {
  if (
    !isRecord(value) ||
    typeof value.limit !== "number" ||
    typeof value.offset !== "number" ||
    typeof value.total !== "number"
  ) {
    throw new OpportunityApiError("The opportunities service returned invalid pagination data.", 502, "invalid_response");
  }
  return { limit: value.limit, offset: value.offset, total: value.total };
}

function parseOpportunity(value: unknown): Opportunity {
  if (!isRecord(value) || (value.type !== "job" && value.type !== "siwes") ||
    (value.status !== "draft" && value.status !== "published" && value.status !== "closed") ||
    (value.application_deadline !== undefined && value.application_deadline !== null &&
      typeof value.application_deadline !== "string")) {
    throw new OpportunityApiError("The opportunities service returned an invalid opportunity.", 502, "invalid_response");
  }
  return {
    id: requiredString(value, "id"),
    type: value.type,
    title: requiredString(value, "title"),
    location: requiredString(value, "location"),
    work_mode: requiredString(value, "work_mode"),
    summary: requiredString(value, "summary"),
    description: requiredString(value, "description"),
    requirements: requiredString(value, "requirements"),
    application_deadline: typeof value.application_deadline === "string" ? value.application_deadline : null,
    status: value.status,
    created_at: requiredString(value, "created_at"),
    updated_at: requiredString(value, "updated_at"),
  };
}

function parseDocument(value: unknown): SiwesDocument {
  if (
    !isRecord(value) ||
    typeof value.size_bytes !== "number" ||
    typeof value.download_available !== "boolean"
  ) {
    throw new OpportunityApiError("The applications service returned invalid document metadata.", 502, "invalid_response");
  }
  return {
    file_name: requiredString(value, "file_name"),
    content_type: requiredString(value, "content_type"),
    size_bytes: value.size_bytes,
    download_available: value.download_available,
  };
}

function parseApplication(value: unknown): SiwesApplication {
  const statuses: SiwesApplicationStatus[] = ["submitted", "under_review", "shortlisted", "accepted", "rejected"];
  if (
    !isRecord(value) ||
    value.type !== "siwes" ||
    !statuses.includes(value.status as SiwesApplicationStatus) ||
    (value.reviewed_at !== undefined && value.reviewed_at !== null && typeof value.reviewed_at !== "string")
  ) {
    throw new OpportunityApiError("The applications service returned an invalid application.", 502, "invalid_response");
  }
  return {
    id: requiredString(value, "id"),
    opportunity_id: requiredString(value, "opportunity_id"),
    type: "siwes",
    full_name: requiredString(value, "full_name"),
    email: requiredString(value, "email"),
    phone: requiredString(value, "phone"),
    institution: requiredString(value, "institution"),
    course: requiredString(value, "course"),
    matric_number: requiredString(value, "matric_number"),
    message: requiredString(value, "message"),
    status: value.status as SiwesApplicationStatus,
    reviewer_note: typeof value.reviewer_note === "string" ? value.reviewer_note : "",
    document: parseDocument(value.document),
    submitted_at: requiredString(value, "submitted_at"),
    reviewed_at: typeof value.reviewed_at === "string" ? value.reviewed_at : null,
  };
}

async function request(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  const isFormData = typeof FormData !== "undefined" && init.body instanceof FormData;
  if (init.body !== undefined && !isFormData) headers.set("Content-Type", "application/json");
  let response: Response;
  try {
    response = await fetchWithRetry(buildApiUrl(path), {
      ...init,
      headers,
      credentials: "include",
      cache: "no-store",
    });
  } catch {
    throw new OpportunityApiError("Could not reach the opportunities service. Check your connection and try again.", 0, "network_error");
  }
  if (!response.ok) {
    let payload: unknown;
    try {
      payload = await response.json();
    } catch {
      payload = null;
    }
    const error = isRecord(payload) && isRecord(payload.error) ? payload.error : undefined;
    throw new OpportunityApiError(
      typeof error?.message === "string" ? error.message : "The request could not be completed.",
      response.status,
      typeof error?.code === "string" ? error.code : "request_failed",
      typeof error?.request_id === "string" ? error.request_id : response.headers.get("X-Request-ID") ?? undefined,
    );
  }
  return response;
}

async function jsonResponse(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    throw new OpportunityApiError("The opportunities service returned an invalid response.", 502, "invalid_response");
  }
}

function parseOpportunityEnvelope(payload: unknown): Opportunity {
  if (!isRecord(payload) || !("opportunity" in payload)) {
    throw new OpportunityApiError("The opportunities service returned an invalid response.", 502, "invalid_response");
  }
  return parseOpportunity(payload.opportunity);
}

export function formatOpportunityError(error: unknown, fallback: string): string {
  if (!(error instanceof OpportunityApiError)) return fallback;
  if (error.status === 401) return "Your session expired. Sign in again.";
  if (error.status === 403 || error.code === "permission_denied") return "You don’t have permission to perform this action.";
  if (error.code === "applications_closed") return "Applications for this placement are closed.";
  if (error.code === "rate_limited" || error.status === 429) return "Too many submissions. Please try again later.";
  if (error.code === "unsafe_document") return "The uploaded document did not pass the security scan. Choose a different file.";
  if (error.code === "document_scanning_unavailable" || error.status === 503) {
    return "Secure document checking is temporarily unavailable. Please try again later.";
  }
  if (error.code === "document_too_large" || error.status === 413) return "The document must be no larger than 10 MiB.";
  if (error.code === "invalid_opportunity" || error.code === "invalid_application") return "Check the form fields and try again.";
  if (error.code === "opportunity_not_publishable") return "Complete the required fields and set a future deadline before publishing.";
  return error.requestId ? `${fallback} (Request ID: ${error.requestId})` : fallback;
}

function listQuery(params: { type?: OpportunityType; status?: OpportunityStatus; limit?: number; offset?: number }) {
  const query = new URLSearchParams({
    limit: String(params.limit ?? 50),
    offset: String(params.offset ?? 0),
  });
  if (params.type) query.set("type", params.type);
  if (params.status) query.set("status", params.status);
  return query.toString();
}

export async function listPublicOpportunities(
  params: { type?: OpportunityType; limit?: number; offset?: number } = {},
  signal?: AbortSignal,
): Promise<OpportunityListResult> {
  const response = await request(`${apiRoutes.opportunities.list}?${listQuery(params)}`, { signal });
  const payload = await jsonResponse(response);
  if (!isRecord(payload) || !Array.isArray(payload.opportunities)) {
    throw new OpportunityApiError("The opportunities service returned an invalid list.", 502, "invalid_response");
  }
  return { opportunities: payload.opportunities.map(parseOpportunity), page: parsePage(payload.page) };
}

export async function getPublicOpportunity(id: string, signal?: AbortSignal): Promise<Opportunity> {
  const response = await request(apiRoutes.opportunities.details(encodeURIComponent(id)), { signal });
  return parseOpportunityEnvelope(await jsonResponse(response));
}

export async function listManagedOpportunities(
  params: { type?: OpportunityType; status?: OpportunityStatus; limit?: number; offset?: number } = {},
  signal?: AbortSignal,
): Promise<OpportunityListResult> {
  const response = await request(`${apiRoutes.opportunities.manage}?${listQuery(params)}`, { signal });
  const payload = await jsonResponse(response);
  if (!isRecord(payload) || !Array.isArray(payload.opportunities)) {
    throw new OpportunityApiError("The opportunities service returned an invalid list.", 502, "invalid_response");
  }
  return { opportunities: payload.opportunities.map(parseOpportunity), page: parsePage(payload.page) };
}

export async function createOpportunity(input: OpportunityInput): Promise<Opportunity> {
  const response = await request(apiRoutes.opportunities.list, { method: "POST", body: JSON.stringify(input) });
  return parseOpportunityEnvelope(await jsonResponse(response));
}

export async function updateOpportunity(
  id: string,
  input: Partial<OpportunityInput>,
): Promise<Opportunity> {
  const response = await request(apiRoutes.opportunities.details(encodeURIComponent(id)), {
    method: "PATCH",
    body: JSON.stringify(input),
  });
  return parseOpportunityEnvelope(await jsonResponse(response));
}

export async function changeOpportunityStatus(
  id: string,
  action: "publish" | "unpublish" | "close",
): Promise<Opportunity> {
  const response = await request(apiRoutes.opportunities[action](encodeURIComponent(id)), {
    method: "POST",
    body: "{}",
  });
  return parseOpportunityEnvelope(await jsonResponse(response));
}

export async function submitSiwesApplication(opportunityId: string, input: {
  full_name: string;
  email: string;
  phone: string;
  institution: string;
  course: string;
  matric_number: string;
  message: string;
  document: File;
}): Promise<SiwesApplication> {
  const form = new FormData();
  for (const [key, value] of Object.entries(input)) {
    if (key !== "document") form.set(key, value);
  }
  form.set("document", input.document);
  const response = await request(apiRoutes.opportunities.applySiwes(encodeURIComponent(opportunityId)), {
    method: "POST",
    body: form,
  });
  const payload = await jsonResponse(response);
  if (!isRecord(payload) || !("application" in payload)) {
    throw new OpportunityApiError("The applications service returned an invalid response.", 502, "invalid_response");
  }
  return parseApplication(payload.application);
}

export async function listSiwesApplications(
  params: { opportunityId?: string; status?: SiwesApplicationStatus; limit?: number; offset?: number } = {},
  signal?: AbortSignal,
): Promise<SiwesApplicationListResult> {
  const query = new URLSearchParams({
    limit: String(params.limit ?? 25),
    offset: String(params.offset ?? 0),
  });
  if (params.opportunityId) query.set("opportunity_id", params.opportunityId);
  if (params.status) query.set("status", params.status);
  const response = await request(`${apiRoutes.opportunities.applications}?${query}`, { signal });
  const payload = await jsonResponse(response);
  if (!isRecord(payload) || !Array.isArray(payload.applications)) {
    throw new OpportunityApiError("The applications service returned an invalid list.", 502, "invalid_response");
  }
  return { applications: payload.applications.map(parseApplication), page: parsePage(payload.page) };
}

export async function getSiwesApplication(id: string, signal?: AbortSignal): Promise<SiwesApplication> {
  const response = await request(apiRoutes.opportunities.applicationDetails(encodeURIComponent(id)), { signal });
  const payload = await jsonResponse(response);
  if (!isRecord(payload) || !("application" in payload)) {
    throw new OpportunityApiError("The applications service returned an invalid response.", 502, "invalid_response");
  }
  return parseApplication(payload.application);
}

export async function updateSiwesApplication(
  id: string,
  input: { status?: SiwesApplicationStatus; reviewer_note?: string },
): Promise<SiwesApplication> {
  const response = await request(apiRoutes.opportunities.applicationDetails(encodeURIComponent(id)), {
    method: "PATCH",
    body: JSON.stringify(input),
  });
  const payload = await jsonResponse(response);
  if (!isRecord(payload) || !("application" in payload)) {
    throw new OpportunityApiError("The applications service returned an invalid response.", 502, "invalid_response");
  }
  return parseApplication(payload.application);
}

export async function downloadSiwesDocument(id: string): Promise<{ blob: Blob; fileName: string }> {
  const response = await request(apiRoutes.opportunities.applicationDocument(encodeURIComponent(id)));
  const disposition = response.headers.get("Content-Disposition") ?? "";
  const encodedName = disposition.match(/filename\*=UTF-8''([^;]+)/i)?.[1];
  const quotedName = disposition.match(/filename="?([^";]+)"?/i)?.[1];
  const fileName = encodedName
    ? decodeURIComponent(encodedName)
    : quotedName || `siwes-application-${id}`;
  return { blob: await response.blob(), fileName };
}
