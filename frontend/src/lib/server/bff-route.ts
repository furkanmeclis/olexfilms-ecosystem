import type { AuthRealm } from "@/lib/server/auth-tokens";
import { proxyToUpstream } from "@/lib/server/bff-proxy";

type RouteContext = {
  params: Promise<{ path: string[] }>;
};

/** Route handler of a realm's BFF (`/api/v1/*` panel, `/api/portal/v1/*` portal). */
export function bffRouteHandler(realm: AuthRealm) {
  return async function handle(request: Request, context: RouteContext) {
    const { path } = await context.params;
    try {
      return await proxyToUpstream(realm, path ?? [], request);
    } catch (err) {
      console.error(`[bff:${realm}]`, (path ?? []).join("/"), err);
      return new Response(
        JSON.stringify({ success: false, error: "internal_error" }),
        {
          status: 500,
          headers: { "Content-Type": "application/json" },
        },
      );
    }
  };
}
