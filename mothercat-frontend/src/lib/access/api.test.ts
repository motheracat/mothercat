import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createRole,
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
