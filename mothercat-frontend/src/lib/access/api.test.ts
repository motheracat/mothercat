import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createRole,
  listUserRoleAssignments,
  listRoles,
  replaceRolePermissions,
  replaceUserRoles,
} from "@/lib/access/api";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("Access API", () => {
  it("loads and validates roles from the same-origin API", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      roles: [{
        role_id: "staff",
        name: "Staff",
        description: "Basic staff access",
        is_system_role: true,
        permissions: ["staff.read"],
      }],
    }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listRoles()).resolves.toEqual([{
      role_id: "staff",
      name: "Staff",
      description: "Basic staff access",
      is_system_role: true,
      permissions: ["staff.read"],
    }]);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/v1/roles");
    expect(init.credentials).toBe("include");
    expect(init.cache).toBe("no-store");
  });

  it("submits a complete permission replacement through PUT", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      role_id: "auditor",
      permission_keys: ["audit.read", "staff.read"],
    }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(replaceRolePermissions("auditor", ["audit.read", "staff.read"]))
      .resolves.toEqual(["audit.read", "staff.read"]);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/v1/roles/auditor/permissions");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(String(init.body))).toEqual({ permission_keys: ["audit.read", "staff.read"] });
  });

  it("loads paginated staff accounts with their role assignments", async () => {
    const user = {
      user_id: "account-1",
      full_name: "Taylor Staff",
      staff_id: "12345",
      email: "taylor@example.com",
      profile_image_url: null,
      employment_status: "active",
      account_active: true,
      role_ids: ["staff", "auditor"],
    };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      users: [user],
      pagination: { limit: 20, offset: 0, total: 1 },
    }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listUserRoleAssignments({
      limit: 20,
      offset: 0,
      search: "Taylor Staff",
      roleId: "auditor",
    })).resolves.toEqual({ users: [user], total: 1 });
    const [url] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/v1/users/role-assignments?limit=20&offset=0&search=Taylor+Staff&role_id=auditor");
  });

  it("encodes identifiers and submits user role replacement", async () => {
    const userId = "00000000-0000-4000-8000-000000000001";
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      user_id: userId,
      role_ids: ["staff", "auditor"],
    }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(replaceUserRoles(userId, ["staff", "auditor"])).resolves.toEqual({
      user_id: userId,
      role_ids: ["staff", "auditor"],
    });
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe(`/api/v1/users/${userId}/roles`);
    expect(init.method).toBe("PUT");
    expect(JSON.parse(String(init.body))).toEqual({ role_ids: ["staff", "auditor"] });
  });

  it("surfaces API authorization codes and request IDs", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { code: "permission_denied", request_id: "request-123" },
    }), { status: 403 })));

    await expect(createRole({ role_id: "auditor", name: "Auditor", description: "" }))
      .rejects.toMatchObject({
        status: 403,
        code: "permission_denied",
        requestId: "request-123",
      });
  });
});
