import { auth } from "@/auth";
import { portalSession } from "@/auth-portal";
import { ConsentScreen } from "@/features/mcp";

export const dynamic = "force-dynamic";

/**
 * MCP OAuth consent screen (TEC-403). Go's /oauth/authorize redirects here
 * with ?request=<uuid>; the screen decides with the panel or the customer
 * portal session (by the requested endpoint), signing in first if needed.
 */
export default async function OAuthConsentPage({
  searchParams,
}: {
  searchParams: Promise<{ request?: string | string[] }>;
}) {
  const { request } = await searchParams;
  const [panel, portal] = await Promise.all([auth(), portalSession()]);
  return (
    <ConsentScreen
      requestId={typeof request === "string" ? request : ""}
      sessions={{ panel: Boolean(panel?.user), portal: Boolean(portal?.user) }}
    />
  );
}
