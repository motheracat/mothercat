import { buildApiUrl } from "@/lib/api/config";
import { apiRoutes } from "@/lib/api/routes";

export type EmploymentStatus = "pending" | "active" | "inactive" | "deactivated";

function isEmploymentStatus(value: unknown): value is EmploymentStatus {
  return value === "pending" || value === "active" || value === "inactive" || value === "deactivated";
}

export type StaffMember = {
  id: string;
  account_id: string | null;
  staff_id: string;
  department_id: string | null;
  full_name: string;
  profile_image_url: string | null;
  work_email: string | null;
  job_title: string | null;
  employment_status: EmploymentStatus;
};

export type Department = {
  id: string;
  name: string;
  description: string | null;
};

export type StaffImportIssue = {
  row: number;
  staff_id?: string;
  code: string;
  message: string;
};

export type StaffImport = {
  id: string;
  status: string;
  created_at: string;
  completed_at?: string | null;
  result: {
    total_rows: number;
    imported_rows: number;
    error_count: number;
    errors?: StaffImportIssue[];
    errors_truncated?: boolean;
  };
};

export class OrganizationApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
    readonly requestId?: string,
    readonly payload?: unknown,
  ) {
    super(message);
    this.name = "OrganizationApiError";
  }
}

export class OrganizationNetworkError extends Error {
  constructor() {
    super("Could not reach the organization service. Check your connection and try again.");
    this.name = "OrganizationNetworkError";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function optionalString(value: unknown): value is string | null | undefined {
  return value === undefined || value === null || typeof value === "string";
}

async function request(path: string, init: RequestInit = {}): Promise<unknown> {
  const isFormData = typeof FormData !== "undefined" && init.body instanceof FormData;
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body !== undefined && !isFormData && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  let response: Response;
  try {
    response = await fetch(buildApiUrl(path), {
      ...init,
      headers,
      credentials: "include",
      cache: "no-store",
    });
  } catch {
    throw new OrganizationNetworkError();
  }

  let payload: unknown = null;
  try {
    payload = await response.json();
  } catch {
    if (response.ok) {
      throw new Error("The organization service returned an invalid response.");
    }
  }

  if (!response.ok) {
    const error = isRecord(payload) && isRecord(payload.error) ? payload.error : undefined;
    throw new OrganizationApiError(
      typeof error?.message === "string" ? error.message : "The request could not be completed.",
      response.status,
      typeof error?.code === "string" ? error.code : "request_failed",
      typeof error?.request_id === "string" ? error.request_id : undefined,
      payload,
    );
  }

  return payload;
}

function parseStaff(value: unknown): StaffMember {
  if (
    !isRecord(value) ||
    typeof value.id !== "string" ||
    typeof value.staff_id !== "string" ||
    typeof value.full_name !== "string" ||
    !optionalString(value.profile_image_url) ||
    !optionalString(value.account_id) ||
    !optionalString(value.department_id) ||
    !optionalString(value.work_email) ||
    !optionalString(value.job_title) ||
    !isEmploymentStatus(value.employment_status)
  ) {
    throw new Error("The organization service returned an invalid staff record.");
  }

  return {
    id: value.id,
    staff_id: value.staff_id,
    account_id: value.account_id ?? null,
    department_id: value.department_id ?? null,
    full_name: value.full_name,
    profile_image_url: value.profile_image_url ?? null,
    work_email: value.work_email ?? null,
    job_title: value.job_title ?? null,
    employment_status: value.employment_status,
  };
}

function parseDepartment(value: unknown): Department {
  if (
    !isRecord(value) ||
    typeof value.id !== "string" ||
    typeof value.name !== "string" ||
    !optionalString(value.description)
  ) {
    throw new Error("The organization service returned an invalid department.");
  }
  return { id: value.id, name: value.name, description: value.description ?? null };
}

export function parseStaffImport(value: unknown): StaffImport | null {
  if (!isRecord(value) || !isRecord(value.import)) return null;
  const record = value.import;
  if (!isRecord(record.result)) return null;
  const result = record.result;
  if (
    typeof record.id !== "string" ||
    typeof record.status !== "string" ||
    typeof record.created_at !== "string" ||
    typeof result.total_rows !== "number" ||
    typeof result.imported_rows !== "number" ||
    typeof result.error_count !== "number"
  ) {
    return null;
  }

  const errors = Array.isArray(result.errors)
    ? result.errors.filter(
        (issue): issue is StaffImportIssue =>
          isRecord(issue) &&
          typeof issue.row === "number" &&
          typeof issue.code === "string" &&
          typeof issue.message === "string" &&
          optionalString(issue.staff_id),
      )
    : undefined;

  return {
    id: record.id,
    status: record.status,
    created_at: record.created_at,
    completed_at: optionalString(record.completed_at) ? record.completed_at : undefined,
    result: {
      total_rows: result.total_rows,
      imported_rows: result.imported_rows,
      error_count: result.error_count,
      errors,
      errors_truncated: typeof result.errors_truncated === "boolean" ? result.errors_truncated : undefined,
    },
  };
}

export async function listStaff(params: {
  limit: number;
  offset: number;
  search?: string;
  departmentId?: string;
  employmentStatus?: EmploymentStatus | "";
}): Promise<{ staff: StaffMember[]; total: number }> {
  const query = new URLSearchParams({ limit: String(params.limit), offset: String(params.offset) });
  if (params.search) query.set("search", params.search);
  if (params.departmentId) query.set("department_id", params.departmentId);
  if (params.employmentStatus) query.set("employment_status", params.employmentStatus);
  const payload = await request(`${apiRoutes.staff.list}?${query}`);
  if (
    !isRecord(payload) ||
    !Array.isArray(payload.staff) ||
    !isRecord(payload.pagination) ||
    typeof payload.pagination.total !== "number"
  ) {
    throw new Error("The organization service returned an invalid staff list.");
  }
  return { staff: payload.staff.map(parseStaff), total: payload.pagination.total };
}

export async function getStaff(staffId: string): Promise<StaffMember> {
  const payload = await request(apiRoutes.staff.details(staffId));
  if (!isRecord(payload) || !("staff" in payload)) throw new Error("The organization service returned an invalid staff record.");
  return parseStaff(payload.staff);
}

export async function listDepartments(): Promise<Department[]> {
  const payload = await request(apiRoutes.departments);
  if (!isRecord(payload) || !Array.isArray(payload.departments)) {
    throw new Error("The organization service returned an invalid department list.");
  }
  return payload.departments.map(parseDepartment);
}

export async function createStaff(input: {
  staff_id: string;
  full_name: string;
  work_email: string | null;
  job_title: string | null;
  department_id: string | null;
}): Promise<StaffMember> {
  const payload = await request(apiRoutes.staff.list, { method: "POST", body: JSON.stringify(input) });
  if (!isRecord(payload) || !("staff" in payload)) throw new Error("The organization service returned an invalid staff record.");
  return parseStaff(payload.staff);
}

export async function updateStaff(
  staffId: string,
  input: Partial<Pick<StaffMember, "full_name" | "work_email" | "job_title" | "department_id">>,
): Promise<StaffMember> {
  const payload = await request(apiRoutes.staff.details(staffId), { method: "PATCH", body: JSON.stringify(input) });
  if (!isRecord(payload) || !("staff" in payload)) throw new Error("The organization service returned an invalid staff record.");
  return parseStaff(payload.staff);
}

export async function deactivateStaff(staffId: string): Promise<StaffMember> {
  const payload = await request(apiRoutes.staff.deactivate(staffId), { method: "POST" });
  if (!isRecord(payload) || !("staff" in payload)) throw new Error("The organization service returned an invalid staff record.");
  return parseStaff(payload.staff);
}

export async function provisionStaff(staffId: string): Promise<void> {
  const payload = await request(apiRoutes.staff.provision(staffId), { method: "POST" });
  if (!isRecord(payload) || payload.invitation_sent !== true) {
    throw new Error("The organization service did not confirm that an invitation was sent.");
  }
}

export async function createDepartment(input: { name: string; description: string | null }): Promise<Department> {
  const payload = await request(apiRoutes.departments, { method: "POST", body: JSON.stringify(input) });
  if (!isRecord(payload) || !("department" in payload)) throw new Error("The organization service returned an invalid department.");
  return parseDepartment(payload.department);
}

export async function updateDepartment(
  departmentId: string,
  input: Partial<Pick<Department, "name" | "description">>,
): Promise<Department> {
  const payload = await request(apiRoutes.departmentDetails(departmentId), { method: "PATCH", body: JSON.stringify(input) });
  if (!isRecord(payload) || !("department" in payload)) throw new Error("The organization service returned an invalid department.");
  return parseDepartment(payload.department);
}

export async function uploadStaffCsv(file: File): Promise<StaffImport> {
  const form = new FormData();
  form.append("file", file);
  const payload = await request(apiRoutes.staff.imports, { method: "POST", body: form });
  const result = parseStaffImport(payload);
  if (!result) throw new Error("The organization service returned an invalid import result.");
  return result;
}

export async function getStaffImport(importId: string): Promise<StaffImport> {
  const payload = await request(apiRoutes.staff.importDetails(importId));
  const result = parseStaffImport(payload);
  if (!result) throw new Error("The organization service returned an invalid import result.");
  return result;
}

export function formatOrganizationError(error: unknown, fallback: string): string {
  if (error instanceof OrganizationNetworkError) return error.message;
  if (!(error instanceof OrganizationApiError)) {
    return error instanceof Error ? error.message : fallback;
  }

  const messages: Record<string, string> = {
    unauthorized: "Your session has expired. Sign in again.",
    duplicate_staff_id: "That staff number is already in use.",
    department_name_conflict: "A department with that name already exists.",
    staff_import_validation_failed: "The CSV contains rows that could not be imported.",
    import_file_too_large: "The CSV file exceeds the 5 MiB upload limit.",
    invalid_import_upload: "Choose a valid CSV file and try again.",
    service_unavailable: "The organization service is temporarily unavailable. Try again later.",
    identity_unavailable: "The identity service is temporarily unavailable. Try again later.",
    audit_unavailable: "The audit service is temporarily unavailable. Try again later.",
  };
  if (error.status === 403 || error.code === "permission_denied") {
    return "Your account isn't authorized for this action. If you're a Super Admin, sign out and back in. If the problem continues, ask an administrator to verify your API permissions.";
  }

  const message =
    messages[error.code] ??
    (error.status === 401
      ? "Your session has expired. Sign in again."
      : error.status >= 500
        ? "The request could not be completed. Try again later."
        : fallback);
  return error.status >= 500 && error.requestId ? `${message} Support reference: ${error.requestId}` : message;
}
