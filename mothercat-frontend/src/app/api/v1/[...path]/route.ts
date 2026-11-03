import { env } from "@/lib/env";
import { proxyApiRequest } from "@/lib/api/proxy";

type RouteContext = {
  params: Promise<{ path: string[] }>;
};

async function forward(request: Request, context: RouteContext): Promise<Response> {
  const { path } = await context.params;
  return proxyApiRequest(request, path, env.API_BASE_URL);
}

export const GET = forward;
export const POST = forward;
export const PATCH = forward;
