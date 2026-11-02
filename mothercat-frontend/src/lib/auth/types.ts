export type AuthSession = {
  userId: string;
  staffId: string;
  displayName: string;
  email: string;
  role: string;
  roles: string[];
  permissions: string[];
  isActive: boolean;
};

export type IdentityUser = {
  id: string;
  staff_id: string;
  display_name: string;
  email: string;
  role: string;
  roles: string[];
  permissions: string[];
};

export type LoginCredentials = {
  staffId: string;
  password: string;
};

export type PasswordResetRequest = {
  staffId: string;
};

export type PasswordCompletionPayload = {
  token: string;
  password: string;
  confirmPassword: string;
};

export type PasswordChangePayload = {
  currentPassword: string;
  password: string;
  confirmPassword: string;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === "string");
}

export function parseAuthSession(payload: unknown): AuthSession {
  if (!isRecord(payload) || !isRecord(payload.user)) {
    throw new Error("The authentication service returned an invalid user response.");
  }

  const user = payload.user;
  if (
    typeof user.id !== "string" ||
    typeof user.staff_id !== "string" ||
    typeof user.display_name !== "string" ||
    typeof user.email !== "string" ||
    typeof user.role !== "string" ||
    !isStringArray(user.roles) ||
    !isStringArray(user.permissions)
  ) {
    throw new Error("The authentication service returned an invalid user response.");
  }

  return {
    userId: user.id,
    staffId: user.staff_id,
    displayName: user.display_name,
    email: user.email,
    role: user.role,
    roles: user.roles,
    permissions: user.permissions,
    isActive: true,
  };
}
