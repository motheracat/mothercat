import { z } from "zod";

import { buildApiUrl } from "@/lib/api/config";
import { apiRoutes } from "@/lib/api/routes";

const staffStatusSchema = z.enum(["pending", "active", "inactive", "deactivated"]);
const staffSchema = z.object({
  id: z.string().uuid(),
  account_id: z.string().uuid().nullable().optional(),
  staff_id: z.string().regex(/^\d{4,5}$/),
  department_id: z.string().uuid().nullable().optional(),
  full_name: z.string(),
  work_email: z.string().nullable().optional(),
  job_title: z.string().nullable().optional(),
  employment_status: staffStatusSchema,
});
const departmentSchema = z.object({
  id: z.string().uuid(),
  name: z.string(),
  description: z.string().nullable(),
});
const importIssueSchema = z.object({
  row: z.number().int(),
  staff_id: z.string().optional(),
  code: z.string(),
  message: z.string(),
});
const staffImportSchema = z.object({
  id: z.string().uuid(),
  status: z.enum(["completed", "failed"]),
  created_at: z.string(),
  completed_at: z.string().nullable().optional(),
  result: z.object({
    total_rows: z.number().int(),
    imported_rows: z.number().int(),
    error_count: z.number().int(),
    errors_truncated: z.boolean().optional(),
    errors: z.array(importIssueSchema).optional(),
  }),
});
const staffListResponseSchema = z.object({
  staff: z.array(staffSchema),
  pagination: z.object({
    limit: z.number().int(),
    offset: z.number().int(),
    total: z.number().int(),
  }),
});
const staffResponseSchema = z.object({ staff: staffSchema });
const departmentsResponseSchema = z.object({ departments: z.array(departmentSchema) });
const departmentResponseSchema = z.object({ department: departmentSchema });
const importResponseSchema = z.object({ import: staffImportSchema });
const provisionResponseSchema = z.object({
  staff: staffSchema,
  invitation_sent: z.boolean(),
});
const importFailureResponseSchema = z.object({
  error: z.object({
    code: z.string(),
    request_id: z.string(),
  }),
  import: staffImportSchema,
});
const apiErrorSchema = z.object({
  error: z.object({
    code: z.string().optional(),
    request_id: z.string().optional(),
  }).optional(),
});

export type StaffStatus = z.infer<typeof staffStatusSchema>;
export type StaffRecord = z.infer<typeof staffSchema>;
export type DepartmentRecord = z.infer<typeof departmentSchema>;
export type StaffImportBatch = z.infer<typeof staffImportSchema>;
export type StaffListFilter = {
  limit: number;
  offset: number;
  search?: string;
  department_id?: string;
  employment_status?: StaffStatus;
};
export type StaffCreatePayload = {
  staff_id: string;
  full_name: string;
  work_email: string | null;
  job_title: string | null;
  department_id: string | null;
};
export type StaffUpdatePayload = Partial<Omit<StaffCreatePayload, "staff_id">>;
export type DepartmentPayload = {
  name: string;
  description: string | null;
};
export type DepartmentUpdatePayload = Partial<DepartmentPayload>;

export class OrganizationApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "OrganizationApiError";
  }
}

function parseError(payload: unknown, status: number): OrganizationApiError {
  const parsed = apiErrorSchema.safeParse(payload);
  const error = parsed.success ? parsed.data.error : undefined;
  return new OrganizationApiError(
    "The request could not be completed.",
    status,
    error?.code ?? "request_failed",
    error?.request_id,
  );
}

async function requestApi<T>(
  path: string,
  schema: z.ZodType<T>,
  init: RequestInit = {},
  timeoutMilliseconds = 15_000,
): Promise<T> {
  const controller = new AbortController();
  const abortOnSignal = () => controller.abort();
  if (init.signal?.aborted) controller.abort();
  else init.signal?.addEventListener("abort", abortOnSignal, { once: true });

  let timedOut = false;
  const timeout = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, timeoutMilliseconds);

  let response: Response;
  try {
    response = await fetch(buildApiUrl(path), {
      ...init,
      headers: {
        Accept: "application/json",
        ...(init.body instanceof FormData ? {} : { "Content-Type": "application/json" }),
        ...init.headers,
      },
      credentials: "include",
      cache: "no-store",
      signal: controller.signal,
    });
  } catch {
    if (timedOut) {
      throw new OrganizationApiError("The request timed out. Try again.", 504, "request_timeout");
    }
    throw new OrganizationApiError("The service could not be reached. Check your connection and try again.", 0, "network_error");
  } finally {
    clearTimeout(timeout);
    init.signal?.removeEventListener("abort", abortOnSignal);
  }

  let payload: unknown;
  try {
    const text = await response.text();
    payload = text === "" ? null : JSON.parse(text);
  } catch {
    if (!response.ok) {
      throw new OrganizationApiError("The request could not be completed.", response.status, "invalid_error_response");
    }
    throw new OrganizationApiError("The service returned an invalid response.", 502, "invalid_response");
  }

  if (!response.ok) {
    throw parseError(payload, response.status);
  }

  const parsed = schema.safeParse(payload);
  if (!parsed.success) {
    throw new OrganizationApiError("The service returned an invalid response.", 502, "invalid_response");
  }
  return parsed.data;
}

export function formatOrganizationError(error: unknown, fallback: string): string {
  if (!(error instanceof OrganizationApiError)) return fallback;

  const messages: Record<string, string> = {
    unauthorized: "Your session has expired. Sign in again.",
    permission_denied: "You do not have permission to perform this action.",
    invalid_origin: "This request was blocked by the security policy. Refresh the page and try again.",
    duplicate_staff_id: "That staff ID is already in use.",
    department_name_conflict: "A department with that name already exists.",
    staff_not_found: "That staff record no longer exists. Refresh the list.",
    department_not_found: "That department no longer exists. Refresh the list.",
    invalid_query: "The search filters are invalid. Review them and try again.",
    import_file_too_large: "The CSV exceeds the 5 MiB upload limit.",
    request_too_large: "The request exceeds the allowed size.",
    staff_import_validation_failed: "Some CSV rows need correction. Review the row issues below.",
    request_timeout: "The request took too long. Try again.",
    upstream_timeout: "The service took too long to respond. Try again.",
    upstream_unavailable: "The organization service is temporarily unavailable.",
    service_unavailable: "The organization service is temporarily unavailable.",
    audit_unavailable: "The required audit service is temporarily unavailable. No change was made.",
    invalid_response: "The service returned an unexpected response. Refresh and try again.",
    work_email_required: "Add a valid work email before provisioning an account.",
    staff_not_provisionable: "This staff profile cannot be provisioned in its current state.",
    account_provisioning_conflict: "A login account already exists or could not be provisioned.",
  };
  const message = messages[error.code] ?? (error.status >= 500 ? "The service could not complete the request." : fallback);
  return error.requestId ? `${message} Reference: ${error.requestId}` : message;
}

export async function listStaff(filter: StaffListFilter, signal?: AbortSignal) {
  const params = new URLSearchParams({
    limit: String(filter.limit),
    offset: String(filter.offset),
  });
  if (filter.search) params.set("search", filter.search);
  if (filter.department_id) params.set("department_id", filter.department_id);
  if (filter.employment_status) params.set("employment_status", filter.employment_status);
  return requestApi(
    `${apiRoutes.staff.list}?${params.toString()}`,
    staffListResponseSchema,
    { signal },
  );
}

export async function getStaff(staffId: string, signal?: AbortSignal) {
  return requestApi(apiRoutes.staff.details(encodeURIComponent(staffId)), staffResponseSchema, { signal });
}

export async function createStaff(payload: StaffCreatePayload) {
  return requestApi(apiRoutes.staff.create, staffResponseSchema, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function updateStaff(staffId: string, payload: StaffUpdatePayload) {
  return requestApi(apiRoutes.staff.update(encodeURIComponent(staffId)), staffResponseSchema, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}

export async function deactivateStaff(staffId: string) {
  return requestApi(apiRoutes.staff.deactivate(encodeURIComponent(staffId)), staffResponseSchema, {
    method: "POST",
    body: undefined,
  });
}

export async function provisionStaff(staffId: string) {
  return requestApi(apiRoutes.staff.provision(encodeURIComponent(staffId)), provisionResponseSchema, {
    method: "POST",
    body: undefined,
  });
}

export async function listDepartments(signal?: AbortSignal) {
  return requestApi(apiRoutes.departments.list, departmentsResponseSchema, { signal });
}

export async function createDepartment(payload: DepartmentPayload) {
  return requestApi(apiRoutes.departments.create, departmentResponseSchema, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function updateDepartment(departmentId: string, payload: DepartmentUpdatePayload) {
  return requestApi(apiRoutes.departments.update(encodeURIComponent(departmentId)), departmentResponseSchema, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}

export async function submitStaffImport(file: File) {
  const formData = new FormData();
  formData.append("file", file, file.name);
  const response = await fetch(buildApiUrl(apiRoutes.staff.imports), {
    method: "POST",
    headers: { Accept: "application/json" },
    credentials: "include",
    cache: "no-store",
    body: formData,
    signal: AbortSignal.timeout(30_000),
  });
  let payload: unknown;
  try {
    const text = await response.text();
    payload = text === "" ? null : JSON.parse(text);
  } catch {
    throw new OrganizationApiError("The service returned an invalid response.", 502, "invalid_response");
  }
  if (response.status === 422) {
    const parsed = importFailureResponseSchema.safeParse(payload);
    if (!parsed.success) {
      throw new OrganizationApiError("The service returned an invalid import result.", 502, "invalid_response");
    }
    return { batch: parsed.data.import, requestId: parsed.data.error.request_id, validationFailed: true };
  }
  if (!response.ok) throw parseError(payload, response.status);

  const parsed = importResponseSchema.safeParse(payload);
  if (!parsed.success) {
    throw new OrganizationApiError("The service returned an invalid import result.", 502, "invalid_response");
  }
  return { batch: parsed.data.import, requestId: undefined, validationFailed: false };
}

export async function getStaffImport(importId: string, signal?: AbortSignal) {
  return requestApi(
    apiRoutes.staff.importDetails(encodeURIComponent(importId)),
    importResponseSchema,
    { signal },
  );
}
