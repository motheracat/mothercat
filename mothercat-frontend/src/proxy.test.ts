import { describe, expect, it } from "vitest";
import { NextRequest } from "next/server";

import { proxy } from "@/proxy";

describe("security response headers", () => {
  it("allows profile pictures from Cloudinary in the image source policy", () => {
    const response = proxy(new NextRequest("https://frontend.example/profile"));
    const policy = response.headers.get("Content-Security-Policy");

    expect(policy).toContain("img-src 'self' data: blob: https://res.cloudinary.com");
  });
});
