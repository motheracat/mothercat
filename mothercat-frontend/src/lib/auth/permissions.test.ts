import { describe, expect, it } from "vitest";

import type { AuthSession } from "@/lib/auth/types";
import { hasPermission } from "@/lib/auth/permissions";

const session: AuthSession = {
  userId: "user-uuid",
  staffId: "12345",
  displayName: "Alex Admin",
  email: "alex@example.org",
  role: "Staff",
  roles: [],
  permissions: [],
  isActive: true,
};

describe("hasPermission", () => {
  it.each(["Super Admin", "super_admin", "SUPERADMIN"])("recognizes %s as a frontend superadmin role", (role) => {
    expect(hasPermission({ ...session, role }, "staff.create")).toBe(true);
  });

  it("recognizes a superadmin role in the roles list", () => {
    expect(hasPermission({ ...session, roles: ["super_admin"] }, "departments.manage")).toBe(true);
  });

  it("does not grant permissions to inactive superadmins", () => {
    expect(hasPermission({ ...session, role: "Super Admin", isActive: false }, "staff.create")).toBe(false);
  });

  it("does not grant permissions to regular users without matching grants", () => {
    expect(hasPermission(session, "staff.create")).toBe(false);
  });
});
