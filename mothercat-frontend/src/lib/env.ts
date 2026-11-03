import { z } from "zod";

const appEnvSchema = z.enum(["development", "staging", "production"]);
const authModeSchema = z.enum(["strict", "dev-open"]);

const envSchema = z.object({
  APP_ENV: appEnvSchema.default("development"),
  API_BASE_URL: z.preprocess(
    (value) => (typeof value === "string" && value.trim() === "" ? undefined : value),
    z.string().trim().url().optional(),
  ),
  AUTH_MODE: authModeSchema.default("strict"),
  NEXT_PUBLIC_APP_NAME: z.string().default("Mothercat"),
  NEXT_PUBLIC_SITE_URL: z.string().url().default("http://localhost:3000"),
  NODE_ENV: z.enum(["development", "test", "production"]).default("development"),
});

const parsed = envSchema.safeParse(process.env);

if (!parsed.success) {
  const message = parsed.error.issues
    .map((issue) => `${issue.path.join(".") || "env"}: ${issue.message}`)
    .join("\n");

  throw new Error(`Invalid environment configuration:\n${message}`);
}

export const env = parsed.data;

if (env.AUTH_MODE === "dev-open" && (env.APP_ENV !== "development" || env.NODE_ENV !== "development")) {
  throw new Error("AUTH_MODE=dev-open is only allowed during local development.");
}

const isProduction = env.APP_ENV !== "development" || env.NODE_ENV === "production";
if (!env.API_BASE_URL && isProduction) {
  throw new Error("API_BASE_URL must be configured with the organization API origin before deployment.");
}

if (env.API_BASE_URL) {
  const apiBaseUrl = new URL(env.API_BASE_URL);
  if (
    apiBaseUrl.pathname !== "/" ||
    apiBaseUrl.search ||
    apiBaseUrl.hash ||
    apiBaseUrl.username ||
    apiBaseUrl.password
  ) {
    throw new Error("API_BASE_URL must be an origin without a path, query, credentials, or fragment.");
  }

  if (isProduction && apiBaseUrl.protocol !== "https:") {
    throw new Error("API_BASE_URL must use HTTPS outside local development.");
  }
}
