import { proxyMobileToUpstream } from "@/lib/server/mobile-proxy";

type RouteContext = {
  params: Promise<{ path: string[] }>;
};

/**
 * Mobile app API (TEC-91): Bearer passthrough to Go `/v1/mobile/*`. No
 * session cookie is read or written here; a request with cookies gets 400.
 * This static segment wins over the panel BFF's `/api/v1/[...path]`, which
 * itself refuses `mobile/*`.
 */
async function handle(request: Request, context: RouteContext) {
  const { path } = await context.params;
  try {
    return await proxyMobileToUpstream(path ?? [], request);
  } catch (err) {
    console.error("[bff:mobile]", (path ?? []).join("/"), err);
    return new Response(
      JSON.stringify({ success: false, error: "internal_error" }),
      { status: 500, headers: { "Content-Type": "application/json" } },
    );
  }
}

export const GET = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;
export const HEAD = handle;
