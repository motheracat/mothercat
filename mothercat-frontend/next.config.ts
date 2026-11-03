import type { NextConfig } from "next";

const configuredApiBaseUrl = process.env.API_BASE_URL;
const isProduction = process.env.NODE_ENV === "production" || process.env.APP_ENV === "production" || process.env.APP_ENV === "staging";
if (!configuredApiBaseUrl && isProduction) {
  throw new Error("API_BASE_URL must be configured with the organization API origin before deployment.");
}

if (configuredApiBaseUrl) {
  const parsedApiBaseUrl = new URL(configuredApiBaseUrl);
  if (
    parsedApiBaseUrl.pathname !== "/" ||
    parsedApiBaseUrl.search ||
    parsedApiBaseUrl.hash ||
    parsedApiBaseUrl.username ||
    parsedApiBaseUrl.password
  ) {
    throw new Error("API_BASE_URL must be an origin without a path, query, credentials, or fragment.");
  }
  if (isProduction && parsedApiBaseUrl.protocol !== "https:") {
    throw new Error("API_BASE_URL must use HTTPS outside local development.");
  }
}

const nextConfig: NextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  productionBrowserSourceMaps: false,
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          { key: "Strict-Transport-Security", value: "max-age=63072000; includeSubDomains; preload" },
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
          { key: "Cross-Origin-Opener-Policy", value: "same-origin" },
          { key: "Cross-Origin-Resource-Policy", value: "same-origin" },
          { key: "X-Frame-Options", value: "DENY" },
        ],
      },
    ];
  },
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
};

export default nextConfig;
