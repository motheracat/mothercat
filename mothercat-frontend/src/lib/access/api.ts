import { buildApiUrl } from "@/lib/api/config";
import { fetchWithRetry } from "@/lib/api/retry";

export type AccessRole = {
  role_id: string;
  name: string;
  description: string;
  is_system_role: boolean;
  permissions: string[];
};

export type AccessPermission = {
  permission_key: string;
  description: string;
};

export type UserRoleAssignments = {
  user_id: string;
  role_ids: string[];
};

export type UserRoleAssignmentSummary = {
  user_id: string;
  full_name: string;
  staff_id: string;
  email: string;
  profile_image_url: string | null;
  employment_status: string;
  account_active: boolean;
  role_ids: string[];
};

export class AccessApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "AccessApiError";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function stringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === "string");
}

async function request(path: string, init?: RequestInit): Promise<unknown> {
  let response: Response;
  try {
    response = await fetchWithRetry(buildApiUrl(path), {
      ...init,
      headers: {
        Accept: "application/json",
        ...(init?.body ? { "Content-Type": "application/json" } : {}),
        ...init?.headers,
      },
      credentials: "include",
      cache: "no-store",
    });
  } catch {
    throw new AccessApiError("Could not reach the access service. Check your connection and try again.", 0, "network_error");
  }

  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    if (response.ok) {
      throw new AccessApiError("The access service returned an invalid response.", response.status, "invalid_response");
    }
    payload = null;
  }
  if (!response.ok) {
    const error = isRecord(payload) && isRecord(payload.error) ? payload.error : undefined;
    throw new AccessApiError(
      typeof error?.message === "string" ? error.message : "The request could not be completed.",
      response.status,
      typeof error?.code === "string" ? error.code : "request_failed",
      typeof error?.request_id === "string" ? error.request_id : undefined,
    );
  }
  return payload;
}

function parseRole(value: unknown): AccessRole {
  if (
    !isRecord(value) ||
    typeof value.role_id !== "string" ||
    typeof value.name !== "string" ||
    typeof value.description !== "string" ||
    typeof value.is_system_role !== "boolean" ||
    !stringArray(value.permissions)
  ) {
    throw new AccessApiError("The access service returned an invalid role.", 502, "invalid_response");
  }
  return {
    role_id: value.role_id,
    name: value.name,
    description: value.description,
    is_system_role: value.is_system_role,
    permissions: value.permissions,
  };
}

export async function listRoles(signal?: AbortSignal): Promise<AccessRole[]> {
  const payload = await request("/roles", { signal });
  if (!isRecord(payload) || !Array.isArray(payload.roles)) {
    throw new AccessApiError("The access service returned an invalid role list.", 502, "invalid_response");
  }
  return payload.roles.map(parseRole);
}

export async function listPermissions(signal?: AbortSignal): Promise<AccessPermission[]> {
  const payload = await request("/permissions", { signal });
  if (!isRecord(payload) || !Array.isArray(payload.permissions)) {
    throw new AccessApiError("The access service returned an invalid permission list.", 502, "invalid_response");
  }
  return payload.permissions.map((item) => {
    if (
      !isRecord(item) ||
      typeof item.permission_key !== "string" ||
      typeof item.description !== "string"
    ) {
      throw new AccessApiError("The access service returned an invalid permission.", 502, "invalid_response");
    }
    return { permission_key: item.permission_key, description: item.description };
  });
}

export async function createRole(input: {
  role_id: string;
  name: string;
  description: string;
}): Promise<AccessRole> {
  const payload = await request("/roles", { method: "POST", body: JSON.stringify(input) });
  if (!isRecord(payload) || !("role" in payload)) {
    throw new AccessApiError("The access service returned an invalid role.", 502, "invalid_response");
  }
  return parseRole(payload.role);
}

export async function updateRole(
  roleId: string,
  input: { name?: string; description?: string },
): Promise<AccessRole> {
  const payload = await request(`/roles/${encodeURIComponent(roleId)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
  if (!isRecord(payload) || !("role" in payload)) {
    throw new AccessApiError("The access service returned an invalid role.", 502, "invalid_response");
  }
  return parseRole(payload.role);
}

export async function replaceRolePermissions(roleId: string, permissionKeys: string[]): Promise<string[]> {
  const payload = await request(`/roles/${encodeURIComponent(roleId)}/permissions`, {
    method: "PUT",
    body: JSON.stringify({ permission_keys: permissionKeys }),
  });
  if (
    !isRecord(payload) ||
    payload.role_id !== roleId ||
    !stringArray(payload.permission_keys)
  ) {
    throw new AccessApiError("The access service returned an invalid permission update.", 502, "invalid_response");
  }
  return payload.permission_keys;
}

export async function getUserRoles(userId: string, signal?: AbortSignal): Promise<UserRoleAssignments> {
  const payload = await request(`/users/${encodeURIComponent(userId)}/roles`, { signal });
  if (
    !isRecord(payload) ||
    payload.user_id !== userId ||
    !stringArray(payload.role_ids)
  ) {
    throw new AccessApiError("The access service returned invalid user role assignments.", 502, "invalid_response");
  }
  return { user_id: payload.user_id, role_ids: payload.role_ids };
}

export async function listUserRoleAssignments(
  params: { limit: number; offset: number; search?: string; roleId?: string },
  signal?: AbortSignal,
): Promise<{ users: UserRoleAssignmentSummary[]; total: number }> {
  const query = new URLSearchParams({
    limit: String(params.limit),
    offset: String(params.offset),
  });
  if (params.search) query.set("search", params.search);
  if (params.roleId) query.set("role_id", params.roleId);
  const payload = await request(`/users/role-assignments?${query}`, { signal });
  if (
    !isRecord(payload) ||
    !Array.isArray(payload.users) ||
    !isRecord(payload.pagination) ||
    typeof payload.pagination.total !== "number"
  ) {
    throw new AccessApiError("The access service returned an invalid user assignment list.", 502, "invalid_response");
  }
  const users = payload.users.map((item): UserRoleAssignmentSummary => {
    if (
      !isRecord(item) ||
      typeof item.user_id !== "string" ||
      typeof item.full_name !== "string" ||
      typeof item.staff_id !== "string" ||
      typeof item.email !== "string" ||
      typeof item.employment_status !== "string" ||
      typeof item.account_active !== "boolean" ||
      !stringArray(item.role_ids)
    ) {
      throw new AccessApiError("The access service returned an invalid user assignment.", 502, "invalid_response");
    }
    return {
      user_id: item.user_id,
      full_name: item.full_name,
      staff_id: item.staff_id,
      email: item.email,
      profile_image_url: typeof item.profile_image_url === "string" ? item.profile_image_url : null,
      employment_status: item.employment_status,
      account_active: item.account_active,
      role_ids: item.role_ids,
    };
  });
  return { users, total: payload.pagination.total };
}

export async function replaceUserRoles(userId: string, roleIds: string[]): Promise<UserRoleAssignments> {
  const payload = await request(`/users/${encodeURIComponent(userId)}/roles`, {
    method: "PUT",
    body: JSON.stringify({ role_ids: roleIds }),
  });
  if (
    !isRecord(payload) ||
    payload.user_id !== userId ||
    !stringArray(payload.role_ids)
  ) {
    throw new AccessApiError("The access service returned invalid user role assignments.", 502, "invalid_response");
  }
  return { user_id: payload.user_id, role_ids: payload.role_ids };
}

export function formatAccessError(error: unknown, fallback: string): string {
  if (!(error instanceof AccessApiError)) return fallback;
  switch (error.code) {
    case "unauthorized":
      return "Your session has expired. Sign in again to continue.";
    case "permission_denied":
      return "You don’t have permission to make that change, or you’re trying to delegate access you don’t hold.";
    case "invalid_origin":
      return "The request was blocked because it did not come from this site. Refresh the page and try again.";
    case "system_role_immutable":
      return "Built-in system roles cannot be changed.";
    case "last_active_superadmin":
      return "The last active Super Admin must remain assigned.";
    case "conflict":
      return "That role already exists. Refresh the list and choose a different role.";
    case "not_found":
      return "The role or account could not be found. Refresh the page and try again.";
    case "invalid_role":
    case "invalid_role_id":
    case "invalid_role_set":
    case "invalid_permission_set":
    case "invalid_request":
      return "Check the submitted information and try again.";
    default:
      return error.requestId ? `${fallback} (Request ID: ${error.requestId})` : fallback;
  }
}
