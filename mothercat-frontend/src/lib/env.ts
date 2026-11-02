import { z } from "zod";

const appEnvSchema = z.enum(["development", "staging", "production"]);
const authModeSchema = z.enum(["strict", "dev-open"]);

const envSchema = z.object({
  APP_ENV: appEnvSchema.default("development"),
  API_BASE_URL: z
    .string()
    .trim()
    .url()
    .default("https://mothercat-api.onrender.com"),
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

if (env.APP_ENV !== "development" && new URL(env.API_BASE_URL).protocol !== "https:") {
  throw new Error("API_BASE_URL must use HTTPS outside local development.");
}
