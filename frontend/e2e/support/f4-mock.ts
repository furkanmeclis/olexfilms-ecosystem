import type { Page, Route } from "@playwright/test";

import { E2E_PORTAL } from "./constants";
import { ORG, USER, mockApi, type Json, type MockApi } from "./mock-api";

export const F4 = {
  assistantConversation: "0b9c4c1e-0000-4000-8000-000000004150",
  assistantAction: "0b9c4c1e-0000-4000-8000-000000004151",
  appointment: "0b9c4c1e-0000-4000-8000-000000004152",
  conversation: "0b9c4c1e-0000-4000-8000-000000004153",
  mcpRequest: "0b9c4c1e-0000-4000-8000-000000004154",
  mcpAction: "0b9c4c1e-0000-4000-8000-000000004155",
  campaign: "0b9c4c1e-0000-4000-8000-000000004156",
  distributor: "0b9c4c1e-0000-4000-8000-000000004157",
} as const;

const NOW = "2026-10-07T10:00:00Z";
const DISTRIBUTOR = {
  uuid: F4.distributor,
  slug: "dist",
  name: "Marmara Distribütör",
  role: "manager",
  logo_url: null,
  status: "active",
  access_ends_at: null,
  type: "distributor",
  brand: { slug: "olex", name: "Olex" },
  parent: null,
};

const envelope = (data: unknown) => ({ success: true, data, meta: {} });

function sse(events: [string, unknown][]) {
  return events
    .map(
      ([event, data]) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`,
    )
    .join("");
}

function stream(route: Route, body: string) {
  return route.fulfill({
    status: 200,
    headers: {
      "content-type": "text/event-stream",
      "cache-control": "no-cache",
    },
    body,
  });
}

function campaign(status: string, contents: Json[] = []): Json {
  return {
    uuid: F4.campaign,
    name: "Sonbahar bakım kampanyası",
    organization_uuid: ORG,
    organization_name: "Acme Bayi",
    channels: ["push", "whatsapp"],
    audience_filter: { audience_type: "customers" },
    status,
    timezone: "Europe/Istanbul",
    scheduled_at: null,
    submitted_at: status === "draft" ? null : NOW,
    approved_at: status === "approved" ? NOW : null,
    recipients_total: 24,
    recipients_pending: 24,
    recipients_sent: 0,
    recipients_failed: 0,
    recipients_skipped: 0,
    approver_organization_uuid: F4.distributor,
    approver_organization_name: "Marmara Distribütör",
    contents,
    events:
      status === "approved"
        ? [
            {
              uuid: "0b9c4c1e-0000-4000-8000-00000000415a",
              event_type: "submitted",
              from_status: "draft",
              to_status: "pending_approval",
              reason: null,
              actor_name: "E2E Dealer",
              created_at: "2026-10-07T10:05:00Z",
            },
            {
              uuid: "0b9c4c1e-0000-4000-8000-00000000415b",
              event_type: "approved",
              from_status: "pending_approval",
              to_status: "approved",
              reason: "Uygun.",
              actor_name: "E2E Distributor",
              created_at: "2026-10-07T10:10:00Z",
            },
          ]
        : [],
    created_at: NOW,
    updated_at: NOW,
  };
}

export class F4Mock {
  readonly api: MockApi;
  consented = false;
  conversationCreated = false;
  quotaExceeded = false;
  campaignStatus: "draft" | "pending_approval" | "approved" = "draft";
  campaignContents: Json[] = [];
  bodies: Record<string, unknown[]> = {};
  calls: string[] = [];
  portalCalls: string[] = [];
  portalConsented = true;

  constructor(api: MockApi) {
    this.api = api;
    api.features.push("ai_assistant", "campaigns");
    api.permissions.push(
      "ai.use",
      "ai.actions.confirm",
      "campaigns.read",
      "campaigns.write",
    );
  }

  makeDistributorUser() {
    this.api.memberships = [DISTRIBUTOR as Json];
    this.api.activeOrg = F4.distributor;
    this.api.permissions = ["campaigns.read", "campaigns.approve"];
    this.api.features = ["campaigns"];
  }

  private record(method: string, path: string, body: unknown) {
    if (body !== undefined) {
      (this.bodies[`${method} ${path}`] ??= []).push(body);
    }
  }

  installPanel() {
    this.api.extra.push(async ({ method, path, ok, route, body, url }) => {
      this.calls.push(`${method} ${path}${url.search}`);
      this.record(method, path, body);
      if (method === "GET" && path === "/v1/ai/status") {
        await ok({
          enabled: true,
          allowed: true,
          consent_required: !this.consented,
          consent: this.consented
            ? undefined
            : {
                uuid: "0b9c4c1e-0000-4000-8000-000000004158",
                kind: "ai_guidelines",
                locale: "en",
                version: 1,
                body: "## Guidelines\n\n- Check before creating records.",
                created_at: NOW,
              },
          quota: this.quotaExceeded
            ? { period: "2026-10", limit: 100, used: 100, remaining: 0 }
            : { period: "2026-10", limit: 1000, used: 200, remaining: 800 },
        });
        return true;
      }
      if (method === "POST" && path === "/v1/consents") {
        this.consented = true;
        await ok({}, 201);
        return true;
      }
      if (method === "GET" && path === "/v1/ai/conversations") {
        const items = this.conversationCreated
          ? [
              {
                uuid: F4.assistantConversation,
                title: "Randevu",
                message_count: 1,
                last_message_at: NOW,
                created_at: NOW,
                updated_at: NOW,
              },
            ]
          : [];
        await ok({ items, total: items.length, limit: 50, offset: 0 });
        return true;
      }
      if (method === "POST" && path === "/v1/ai/conversations") {
        this.conversationCreated = true;
        await ok(
          {
            uuid: F4.assistantConversation,
            title: "",
            message_count: 0,
            last_message_at: null,
            created_at: NOW,
            updated_at: NOW,
          },
          201,
        );
        return true;
      }
      if (
        method === "POST" &&
        path === `/v1/ai/conversations/${F4.assistantConversation}/messages`
      ) {
        await stream(
          route,
          sse([
            [
              "message_start",
              {
                conversation_uuid: F4.assistantConversation,
                message_uuid: "m1",
              },
            ],
            ["text_delta", { text: "Müsait bir zaman buldum. " }],
            ["text_delta", { text: "Randevu oluşturayım mı?" }],
            [
              "confirm",
              {
                action_uuid: F4.assistantAction,
                tool_use_id: "appointment-1",
                tool_name: "create_appointment",
                source: "panel",
                status: "pending",
                preview: {
                  action: "create_appointment",
                  summary: "Create appointment",
                  fields: [
                    { key: "customer", value: "Ayşe Yılmaz" },
                    { key: "starts_at", value: "2026-10-08 10:00" },
                  ],
                },
                expires_at: new Date(Date.now() + 900_000).toISOString(),
                created_at: NOW,
              },
            ],
            [
              "title",
              { conversation_uuid: F4.assistantConversation, title: "Randevu" },
            ],
          ]),
        );
        return true;
      }
      if (
        method === "POST" &&
        path === `/v1/ai/actions/${F4.assistantAction}/confirm`
      ) {
        await stream(
          route,
          sse([
            [
              "action",
              {
                action_uuid: F4.assistantAction,
                tool_use_id: "appointment-1",
                tool_name: "create_appointment",
                source: "panel",
                status: "confirmed",
                link: { kind: "appointment", uuid: F4.appointment },
              },
            ],
            ["text_delta", { text: "Randevu oluşturuldu." }],
            ["message_done", { message_uuid: "m2", status: "complete" }],
          ]),
        );
        return true;
      }
      if (method === "GET" && path === "/v1/ai/pending-actions") {
        const done =
          this.bodies[`POST /v1/ai/pending-actions/${F4.mcpAction}/confirm`];
        const items = done
          ? []
          : [
              {
                action_uuid: F4.mcpAction,
                tool_use_id: "mcp-1",
                tool_name: "create_lead",
                source: "mcp",
                status: "pending",
                preview: {
                  action: "create_lead",
                  summary: "MCP lead",
                  fields: [{ key: "contact_name", value: "MCP Ayşe" }],
                },
                expires_at: new Date(Date.now() + 900_000).toISOString(),
                created_at: NOW,
              },
            ];
        await ok({ items, total: items.length, limit: 20, offset: 0 });
        return true;
      }
      if (
        method === "POST" &&
        path === `/v1/ai/pending-actions/${F4.mcpAction}/confirm`
      ) {
        await ok({
          action_uuid: F4.mcpAction,
          tool_name: "create_lead",
          source: "mcp",
          status: "confirmed",
        });
        return true;
      }
      if (method === "GET" && path === `/v1/oauth/requests/${F4.mcpRequest}`) {
        await ok({
          request_uuid: F4.mcpRequest,
          client_id: "claude-client",
          client_name: "Claude",
          redirect_host: "claude.ai",
          resource: "/mcp/dealer",
          resource_url: "https://olexfilms.test/mcp/dealer",
          realm: "dealer",
          organizations: [
            {
              uuid: ORG,
              name: "Acme Bayi",
              type: "dealer",
              tool_count: 21,
            },
          ],
          expires_at: new Date(Date.now() + 600_000).toISOString(),
        });
        return true;
      }
      if (
        method === "POST" &&
        path === `/v1/oauth/requests/${F4.mcpRequest}/decide`
      ) {
        await ok({ redirect_url: "/oauth-consent-done?code=f4-code" });
        return true;
      }
      if (method === "POST" && path === "/v1/campaigns") {
        await ok(campaign("draft"), 201);
        return true;
      }
      if (method === "PATCH" && path === `/v1/campaigns/${F4.campaign}`) {
        await ok(campaign("draft", this.campaignContents));
        return true;
      }
      const content = path.match(/^\/v1\/campaigns\/[^/]+\/contents\/([^/]+)$/);
      if (method === "PUT" && content) {
        const item = {
          locale: decodeURIComponent(content[1]),
          title: body?.title,
          body: body?.body,
          deeplink: body?.deeplink ?? null,
          media: [],
          updated_at: NOW,
        };
        this.campaignContents = this.campaignContents.filter(
          (c) => c.locale !== item.locale,
        );
        this.campaignContents.push(item);
        await ok(item);
        return true;
      }
      if (method === "POST" && path === `/v1/campaigns/${F4.campaign}/submit`) {
        this.campaignStatus = "pending_approval";
        await ok(campaign("pending_approval", this.campaignContents));
        return true;
      }
      if (method === "GET" && path === "/v1/campaigns/approvals") {
        const items =
          this.campaignStatus === "pending_approval"
            ? [campaign("pending_approval", this.campaignContents)]
            : [];
        await ok({ items, total: items.length, limit: 20, offset: 0 });
        return true;
      }
      if (
        method === "POST" &&
        path === `/v1/campaigns/${F4.campaign}/preview`
      ) {
        await ok({
          total: 24,
          locales: [
            { locale: "tr", count: 16 },
            { locale: "de", count: 8 },
          ],
          channels: [
            { channel: "push", reachable: 20 },
            { channel: "whatsapp", reachable: 18 },
          ],
          unreachable: 4,
          excluded: { total: 3, no_consent: 2, opted_out: 1 },
          missing_locales: ["tr", "de"],
          sample: [],
        });
        return true;
      }
      if (
        method === "POST" &&
        path === `/v1/campaigns/${F4.campaign}/approve`
      ) {
        this.campaignStatus = "approved";
        await ok(campaign("approved", this.campaignContents));
        return true;
      }
      if (method === "GET" && path === `/v1/campaigns/${F4.campaign}`) {
        await ok(campaign(this.campaignStatus, this.campaignContents));
        return true;
      }
      if (
        method === "GET" &&
        path === `/v1/campaigns/${F4.campaign}/recipients`
      ) {
        await ok({ items: [], total: 0, limit: 20, offset: 0 });
        return true;
      }
      return false;
    });
  }
}

export async function installF4(page: Page): Promise<F4Mock> {
  const api = await mockApi(page);
  const f4 = new F4Mock(api);
  f4.installPanel();
  return f4;
}

export async function installF4Platform(page: Page) {
  const state = {
    assignedOrg: null as Json | null,
    aiMode: "auto",
    bodies: {} as Record<string, unknown>,
  };
  await page.route(
    (url) =>
      url.pathname.startsWith("/api/") &&
      !url.pathname.startsWith("/api/auth/"),
    async (route) => {
      const req = route.request();
      const url = new URL(req.url());
      const path = url.pathname.replace(/^\/api/, "");
      const method = req.method();
      const ok = (data: unknown, status = 200) =>
        route.fulfill({ status, json: envelope(data) });
      if (method === "GET" && path === "/v1/auth/me") {
        return ok({
          effective_locale: "en",
          effective_timezone: "Europe/Istanbul",
          user: {
            uuid: USER,
            email: "e2e@example.com",
            name: "E2E",
            surname: "Admin",
            status: "active",
            is_super_admin: true,
            email_verified: true,
            locale: "en",
            timezone: "Europe/Istanbul",
          },
          roles: [],
          permissions: [
            "platform.settings.read",
            "conversations.read",
            "conversations.reply",
            "conversations.manage",
          ],
          grants: {},
          active_organization_uuid: null,
          organization_roles: [],
          organizations: [],
          links: {
            profile: "",
            change_password: "",
            notification_preferences: "",
          },
          channels: { user: `user:${USER}` },
          realtime: { enabled: false, user_channel: `user:${USER}` },
        });
      }
      if (method === "GET" && path === "/v1/auth/step-up") {
        return ok({ valid: true, expires_at: null, methods: [] });
      }
      const conversation = {
        uuid: F4.conversation,
        channel: "whatsapp",
        contact_e164: "+905551112233",
        contact_name: "Portal Ziyaretçisi",
        status: "open",
        ai_mode: state.aiMode,
        ai_paused_until:
          state.aiMode === "paused" ? "2026-10-07T10:30:00Z" : null,
        assigned_user: null,
        assigned_org: state.assignedOrg,
        identity_kind: "visitor",
        identity_user: null,
        identity_org: null,
        locale: "tr",
        unread_count: 1,
        last_message_at: NOW,
        last_inbound_at: NOW,
        ai_consent_at: NOW,
        created_at: NOW,
        updated_at: NOW,
      };
      if (method === "GET" && path === "/v1/conversations/meta") {
        return ok({
          resource: "conversations",
          default_sort: "-last_message_at",
          bulk_actions: [],
        });
      }
      if (method === "GET" && path === "/v1/conversations") {
        return ok({ items: [conversation], total: 1, limit: 20, offset: 0 });
      }
      if (method === "GET" && path === `/v1/conversations/${F4.conversation}`) {
        return ok(conversation);
      }
      if (
        method === "PATCH" &&
        path === `/v1/conversations/${F4.conversation}`
      ) {
        const body = req.postDataJSON() as Json;
        state.bodies.patch = body;
        if (body.assigned_org_uuid) {
          state.assignedOrg = {
            uuid: body.assigned_org_uuid,
            name: "Acme Bayi",
            type: "dealer",
          };
        }
        if (body.ai_mode) state.aiMode = String(body.ai_mode);
        return ok({
          ...conversation,
          ai_mode: state.aiMode,
          assigned_org: state.assignedOrg,
        });
      }
      if (
        method === "POST" &&
        path === `/v1/conversations/${F4.conversation}/read`
      ) {
        return ok({ ...conversation, unread_count: 0 });
      }
      if (
        method === "GET" &&
        path === `/v1/conversations/${F4.conversation}/messages`
      ) {
        return ok({
          items: [
            {
              uuid: "0b9c4c1e-0000-4000-8000-00000000415c",
              conversation_uuid: F4.conversation,
              direction: "out",
              sender_type: "ai",
              status: "delivered",
              body: "Garanti bilgilerinizi kontrol edebilirim.",
              has_stored_media: false,
              send_attempts: 0,
              created_at: "2026-10-07T10:00:05Z",
              sender_user: null,
            },
            {
              uuid: "0b9c4c1e-0000-4000-8000-00000000415d",
              conversation_uuid: F4.conversation,
              direction: "in",
              sender_type: "contact",
              status: "received",
              body: "Garantim devam ediyor mu?",
              has_stored_media: false,
              send_attempts: 0,
              created_at: NOW,
              sender_user: null,
            },
          ],
          next_cursor: null,
        });
      }
      if (
        method === "POST" &&
        path === `/v1/conversations/${F4.conversation}/messages`
      ) {
        const body = req.postDataJSON() as Json;
        state.bodies.reply = body;
        state.aiMode = "paused";
        return ok(
          {
            conversation: { ...conversation, ai_mode: "paused" },
            message: {
              uuid: "0b9c4c1e-0000-4000-8000-00000000415e",
              conversation_uuid: F4.conversation,
              direction: "out",
              sender_type: "staff",
              status: "queued",
              body: body.body,
              has_stored_media: true,
              send_attempts: 0,
              created_at: "2026-10-07T10:01:00Z",
              sender_user: { uuid: USER, name: "E2E Admin" },
            },
          },
          201,
        );
      }
      if (method === "GET" && path === "/v1/platform/users") {
        return ok({ items: [], total: 0, limit: 100, offset: 0 });
      }
      if (method === "GET" && path === "/v1/organizations") {
        return ok({
          items: [{ uuid: ORG, name: "Acme Bayi", type: "dealer" }],
          total: 1,
          limit: 20,
          offset: 0,
        });
      }
      return route.fulfill({
        status: 404,
        json: { success: false, error: { code: "NOT_FOUND", message: path } },
      });
    },
  );
  return state;
}

export async function installF4Portal(page: Page) {
  const state = {
    conversations: false,
    calls: [] as string[],
  };
  await page.route("**/api/portal/v1/**", async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace(/^\/api\/portal\/v1\//, "");
    const method = req.method();
    state.calls.push(`${method} ${path}`);
    const ok = (data: unknown, status = 200) =>
      route.fulfill({ status, json: envelope(data) });
    if (method === "GET" && path === "auth/me") {
      return ok({
        user: {
          name: E2E_PORTAL.owner.name,
          surname: E2E_PORTAL.owner.surname,
          email: null,
        },
        roles: ["customer"],
      });
    }
    if (method === "GET" && path === "portal/consents/pending") {
      return ok({ items: [] });
    }
    if (method === "GET" && path === "portal/notification-preferences") {
      return ok({
        email_enabled: true,
        inapp_enabled: true,
        campaign_marketing_enabled: false,
        rules: [],
      });
    }
    if (method === "GET" && path === "oauth/grants") {
      return ok({ items: [], total: 0, limit: 10, offset: 0 });
    }
    if (method === "GET" && path === "portal/ai/status") {
      return ok({
        enabled: true,
        allowed: true,
        consent_required: false,
        quota: { period: "2026-10", limit: 0, used: 0, remaining: null },
      });
    }
    if (method === "GET" && path === "portal/ai/conversations") {
      const items = state.conversations
        ? [
            {
              uuid: F4.assistantConversation,
              title: "Garanti",
              message_count: 1,
              last_message_at: NOW,
              created_at: NOW,
              updated_at: NOW,
            },
          ]
        : [];
      return ok({ items, total: items.length, limit: 50, offset: 0 });
    }
    if (method === "POST" && path === "portal/ai/conversations") {
      state.conversations = true;
      return ok(
        {
          uuid: F4.assistantConversation,
          title: "",
          message_count: 0,
          last_message_at: null,
          created_at: NOW,
          updated_at: NOW,
        },
        201,
      );
    }
    if (
      method === "POST" &&
      path === `portal/ai/conversations/${F4.assistantConversation}/messages`
    ) {
      return stream(
        route,
        sse([
          [
            "message_start",
            {
              conversation_uuid: F4.assistantConversation,
              message_uuid: "pm1",
            },
          ],
          ["text_delta", { text: "Garantiniz aktif görünüyor. " }],
          ["text_delta", { text: "Detayları araç sayfanızda bulabilirsiniz." }],
          ["message_done", { message_uuid: "pm1", status: "complete" }],
        ]),
      );
    }
    return route.fulfill({
      status: 404,
      json: { success: false, error: { code: "NOT_FOUND", message: path } },
    });
  });
  return state;
}
