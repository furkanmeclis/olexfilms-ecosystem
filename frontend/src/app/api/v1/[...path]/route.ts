import { proxyToUpstream } from "@/lib/server/bff-proxy";

type RouteContext = {
  params: Promise<{ path: string[] }>;
};

/**
 * Long-lived SSE streams (AI chat and confirmed-action continuations) need a
 * raised function timeout on serverless hosts. The Go AI stream handlers
 * extend their own write deadline to 15 minutes (heartbeat every 15s), so
 * allow the same here; self-hosted `next start` ignores this value.
 */
export const maxDuration = 900;

async function handle(request: Request, context: RouteContext) {
  const { path } = await context.params;
  try {
    return await proxyToUpstream(path ?? [], request);
  } catch (err) {
    console.error("[api/v1]", (path ?? []).join("/"), err);
    return new Response(
      JSON.stringify({ success: false, error: "internal_error" }),
      {
        status: 500,
        headers: { "Content-Type": "application/json" },
      },
    );
  }
}

export const GET = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;
export const HEAD = handle;
export const OPTIONS = handle;
