import type { AuthSession } from "@/lib/auth/types";

export type PermissionKey =
  | "staff.read"
  | "staff.create"
  | "staff.update"
  | "staff.deactivate"
  | "staff.import"
  | "departments.manage"
  | "roles.read"
  | "roles.manage"
  | "user_roles.assign"
  | "audit.read"
  | "tasks.create"
  | "tasks.assign"
  | "tasks.review"
  | "opportunities.publish"
  | "applications.review"
  | "it_applications.review"
  | "projects.manage"
  | "site_reports.review"
  | "settings.manage"
  | "*";

export function hasPermission(session: AuthSession | null, permission: PermissionKey | string): boolean {
  if (!session) {
    return false;
  }
  if (!session.isActive) {
    return false;
  }
  const perms = session.permissions ?? [];
  if (perms.includes("*")) {
    return true;
  }
  return perms.includes(permission);
}

export function hasAnyPermission(session: AuthSession | null, permissions: (PermissionKey | string)[]): boolean {
  if (!session || !session.isActive) {
    return false;
  }
  const perms = session.permissions ?? [];
  if (perms.includes("*")) {
    return true;
  }
  return permissions.some((p) => perms.includes(p));
}
