import type { ServerListParams } from "@/components/entity";
import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";
import { portalRequest } from "@/features/portal/lib/portal-client";

type Schemas = components["schemas"];

export type OAuthConsent = Schemas["OAuthConsent"];
export type OAuthConsentOrganization = Schemas["OAuthConsentOrganization"];
export type OAuthDecision = { redirect_url: string };
export type OAuthGrant = Schemas["OAuthGrant"];
export type OAuthClientSummary = Schemas["OAuthClientSummary"];
export type AIActionCard = Schemas["AIActionCard"];
export type AIActionOutcome = Schemas["AIActionOutcome"];

export type McpRealm = OAuthConsent["realm"];
/** Which session answers: the panel BFF or the customer portal BFF. */
export type SessionRealm = "panel" | "portal";

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** GET /v1/oauth/grants: `realm` CSV, sort created_at | last_used_at | client_name. */
export type GrantListParams = ServerListParams & { realm?: string };
/** GET /v1/platform/oauth/clients: `status` CSV (active, revoked). */
export type ClientListParams = ServerListParams & { status?: string };
/** GET /v1/ai/pending-actions: `source` CSV (mcp, whatsapp). */
export type PendingActionListParams = ServerListParams & { source?: string };

export const MCP_REALMS = ["dealer", "user", "customer"] as const;
export const CLIENT_STATUSES = ["active", "revoked"] as const;
export const APPROVAL_SOURCES = ["mcp", "whatsapp"] as const;

const enc = encodeURIComponent;

function query(params: Record<string, unknown>) {
  const out = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null && value !== "") {
      out.set(key, String(value));
    }
  }
  const qs = out.toString();
  return qs ? `?${qs}` : "";
}

export const mcpService = {
  consent: (realm: SessionRealm, uuid: string) =>
    realm === "portal"
      ? portalRequest<OAuthConsent>(`portal/oauth/requests/${enc(uuid)}`)
      : platformRequest<OAuthConsent>("GET", `/v1/oauth/requests/${enc(uuid)}`),

  decide: (
    realm: SessionRealm,
    uuid: string,
    body: Schemas["OAuthDecideRequest"],
  ) =>
    realm === "portal"
      ? portalRequest<OAuthDecision>(
          `portal/oauth/requests/${enc(uuid)}/decide`,
          { method: "POST", body },
        )
      : platformRequest<OAuthDecision>(
          "POST",
          `/v1/oauth/requests/${enc(uuid)}/decide`,
          { body },
        ),

  grants: (realm: SessionRealm, params: GrantListParams) =>
    realm === "portal"
      ? portalRequest<Page<OAuthGrant>>(`portal/oauth/grants${query(params)}`)
      : platformRequest<Page<OAuthGrant>>("GET", "/v1/oauth/grants", {
          query: params,
        }),

  revokeGrant: (realm: SessionRealm, uuid: string) =>
    realm === "portal"
      ? portalRequest<void>(`portal/oauth/grants/${enc(uuid)}`, {
          method: "DELETE",
        })
      : platformRequest<void>("DELETE", `/v1/oauth/grants/${enc(uuid)}`),

  clients: (params: ClientListParams) =>
    platformRequest<Page<OAuthClientSummary>>(
      "GET",
      "/v1/platform/oauth/clients",
      { query: params },
    ),

  revokeClient: (uuid: string) =>
    platformRequest<void>("DELETE", `/v1/platform/oauth/clients/${enc(uuid)}`),

  pendingActions: (params: PendingActionListParams) =>
    platformRequest<Page<AIActionCard>>("GET", "/v1/ai/pending-actions", {
      query: params,
    }),

  confirmAction: (uuid: string) =>
    platformRequest<AIActionOutcome>(
      "POST",
      `/v1/ai/pending-actions/${enc(uuid)}/confirm`,
    ),

  cancelAction: (uuid: string) =>
    platformRequest<AIActionOutcome>(
      "POST",
      `/v1/ai/pending-actions/${enc(uuid)}/cancel`,
    ),
};
