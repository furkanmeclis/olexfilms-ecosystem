import { mcpPassthrough } from "@/lib/server/mcp-passthrough";

export const dynamic = "force-dynamic";

type RouteContext = { params: Promise<{ path: string[] }> };

/** MCP OAuth endpoints (TEC-400): passthrough to Go `/oauth/*`. */
async function handle(request: Request, context: RouteContext) {
  const { path } = await context.params;
  return mcpPassthrough(request, "/oauth", path ?? []);
}

export const GET = handle;
export const POST = handle;
export const OPTIONS = handle;
