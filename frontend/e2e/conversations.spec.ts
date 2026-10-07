import { expect, test, type Route } from "@playwright/test";

import { signIn } from "./support/mock-api";

/**
 * TEC-399: the platform WhatsApp inbox against a mocked BFF — list with
 * filters, split view, staff reply, 16 MB attachment limit, AI mode PATCH.
 * Per S2 only the platform admin sees conversations.
 */

const USER = "0b9c4c1e-0000-4000-8000-000000000002";
const CONV = "0b9c4c1e-0000-4000-8000-000000000399";
const NOW = "2026-10-07T09:00:00Z";
const PERMISSIONS = [
  "platform.settings.read",
  "conversations.read",
  "conversations.reply",
  "conversations.manage",
];

const envelope = (data: unknown) => ({ success: true, data, meta: {} });

function me() {
  return {
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
    permissions: PERMISSIONS,
    grants: Object.fromEntries(PERMISSIONS.map((p) => [p, "all"])),
    active_organization_uuid: null,
    organization_roles: [],
    organizations: [],
    links: {
      profile: "/v1/auth/profile",
      change_password: "/v1/auth/password/change",
      notification_preferences: "/v1/notification-preferences",
    },
    channels: { user: `user:${USER}` },
    realtime: { enabled: false, user_channel: `user:${USER}` },
  };
}

const conversation = {
  uuid: CONV,
  channel: "whatsapp",
  contact_e164: "+905551112233",
  contact_name: "Ayse Yilmaz",
  status: "open",
  ai_mode: "auto",
  ai_paused_until: null,
  assigned_user: null,
  assigned_org: null,
  identity_kind: "customer",
  identity_user: null,
  identity_org: null,
  locale: "tr",
  unread_count: 2,
  last_message_at: NOW,
  last_inbound_at: NOW,
  ai_consent_at: NOW,
  created_at: NOW,
  updated_at: NOW,
};

const inbound = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000003a1",
  conversation_uuid: CONV,
  direction: "in",
  sender_type: "contact",
  status: "received",
  body: "Hello, is my car ready?",
  has_stored_media: false,
  send_attempts: 0,
  created_at: NOW,
  sender_user: null,
};
const aiReply = {
  ...inbound,
  uuid: "0b9c4c1e-0000-4000-8000-0000000003a2",
  direction: "out",
  sender_type: "ai",
  status: "delivered",
  body: "Let me check that for you.",
  created_at: "2026-10-07T09:00:05Z",
};

const meta = {
  resource: "conversations",
  default_sort: "-last_message_at",
  bulk_actions: [
    {
      id: "close",
      label_key: "bulk.actions.conversations.close",
      permission: "conversations.manage",
    },
  ],
};

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("platform inbox: list, split view, reply, attachment limit, AI mode", async ({
  page,
}) => {
  const calls: string[] = [];
  const bodies: Record<string, unknown> = {};
  let current = { ...conversation };
  await page.route(
    (url) =>
      url.pathname.startsWith("/api/") &&
      !url.pathname.startsWith("/api/auth/"),
    async (route: Route) => {
      const req = route.request();
      const url = new URL(req.url());
      const path = url.pathname.replace(/^\/api/, "");
      const method = req.method();
      calls.push(`${method} ${path}${url.search}`);
      const ok = (data: unknown, status = 200) =>
        route.fulfill({ status, json: envelope(data) });
      if (method === "GET" && path === "/v1/auth/me") return ok(me());
      if (method === "GET" && path === "/v1/auth/step-up") {
        return ok({ valid: true, expires_at: null, methods: [] });
      }
      if (method === "GET" && path === "/v1/conversations/meta") {
        return ok(meta);
      }
      if (method === "GET" && path === "/v1/conversations") {
        return ok({ items: [current], total: 1, limit: 20, offset: 0 });
      }
      if (method === "GET" && path === `/v1/conversations/${CONV}`) {
        return ok(current);
      }
      if (method === "PATCH" && path === `/v1/conversations/${CONV}`) {
        bodies.patch = req.postDataJSON();
        current = { ...current, ...(bodies.patch as object) };
        return ok(current);
      }
      if (method === "POST" && path === `/v1/conversations/${CONV}/read`) {
        current = { ...current, unread_count: 0 };
        return ok(current);
      }
      if (method === "GET" && path === `/v1/conversations/${CONV}/messages`) {
        return ok({ items: [aiReply, inbound], next_cursor: null });
      }
      if (method === "POST" && path === `/v1/conversations/${CONV}/messages`) {
        bodies.reply = req.postDataJSON();
        current = { ...current, ai_mode: "paused" };
        return ok(
          {
            conversation: current,
            message: {
              ...inbound,
              uuid: "0b9c4c1e-0000-4000-8000-0000000003a3",
              direction: "out",
              sender_type: "staff",
              status: "queued",
              body: (bodies.reply as { body: string }).body,
              created_at: "2026-10-07T09:01:00Z",
              sender_user: { uuid: USER, name: "E2E Admin" },
            },
          },
          201,
        );
      }
      if (method === "GET" && path === "/v1/platform/users") {
        return ok({
          items: [
            {
              uuid: USER,
              email: "e2e@example.com",
              name: "E2E",
              surname: "Admin",
              status: "active",
              is_super_admin: true,
              email_verified: true,
            },
          ],
          total: 1,
          limit: 100,
          offset: 0,
        });
      }
      return route.fulfill({
        status: 404,
        json: { error: { code: "NOT_FOUND", message: "Not mocked" } },
      });
    },
  );

  await page.goto("/platform/conversations");
  await expect(
    page.getByRole("heading", { name: "Conversations", level: 1 }),
  ).toBeVisible();
  // Menu entry with the unread badge (platform panel only).
  await expect(
    page.getByRole("link", { name: /Conversations/ }).first(),
  ).toBeVisible();

  const row = page.getByRole("row", { name: /Ayse Yilmaz/ });
  await expect(row).toBeVisible();
  expect(
    calls.some(
      (c) =>
        c.startsWith("GET /v1/conversations?") &&
        c.includes("sort=-last_message_at"),
    ),
  ).toBe(true);

  await row.getByText("Ayse Yilmaz").click();
  const panel = page.getByTestId("conversation-panel");
  await expect(panel).toBeVisible();
  await expect(
    panel.locator('[data-testid="message"][data-sender="contact"]'),
  ).toContainText("Hello, is my car ready?");
  await expect(
    panel.locator('[data-testid="message"][data-sender="ai"]'),
  ).toContainText("Let me check that for you.");
  await expect
    .poll(() => calls)
    .toContain(`POST /v1/conversations/${CONV}/read`);

  // Over 16 MB is rejected on the client.
  await panel.getByTestId("attachment-input").setInputFiles({
    name: "big.pdf",
    mimeType: "application/pdf",
    buffer: Buffer.alloc(16 * 1024 * 1024 + 1),
  });
  await expect(panel.getByTestId("attachment-error")).toContainText("16 MB");

  await panel.getByTestId("composer-input").fill("Yes, it is ready.");
  await panel.getByTestId("composer-send").click();
  await expect(
    panel.locator('[data-testid="message"][data-sender="staff"]'),
  ).toContainText("Yes, it is ready.");
  await expect(panel.getByTestId("ai-paused-notice")).toBeVisible();
  expect(bodies.reply).toEqual({ body: "Yes, it is ready." });

  await panel.getByTestId("ai-mode-off").click();
  await expect.poll(() => bodies.patch).toEqual({ ai_mode: "off" });
});
