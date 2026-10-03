import { expect, test, type Route } from "@playwright/test";

import { signIn } from "./support/mock-api";

/**
 * TEC-274: the Glorian admin screen against a mocked BFF — masked API key,
 * the TEC-273 endpoints (connection, test, sync runs, held outbounds,
 * replay, reconcile) and the drift report.
 */

const USER = "0b9c4c1e-0000-4000-8000-000000000002";
// The platform panel needs one platform.* permission (hasPlatformPermission).
const PERMISSIONS = [
  "platform.settings.read",
  "integrations.glorian.view",
  "integrations.glorian.manage",
];
const NOW = "2026-10-03T09:00:00Z";
const OUTBOUND = "0b9c4c1e-0000-4000-8000-0000000000b1";
const RUN = "0b9c4c1e-0000-4000-8000-0000000000c1";
const BASE = "/v1/platform/integrations/glorian";

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
      is_super_admin: false,
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

const connection = {
  configured: true,
  uuid: "0b9c4c1e-0000-4000-8000-0000000000aa",
  key: "glorian",
  base_url: "https://hub.example.com",
  active: true,
  api_version: "1",
  default_warehouse_uuid: null,
  api_key_set: true,
  api_key_masked: "********",
  created_at: NOW,
  updated_at: NOW,
};

const reconcileRun = {
  uuid: RUN,
  kind: "reconcile",
  status: "succeeded",
  started_at: NOW,
  finished_at: NOW,
  watermark: null,
  counts: {
    only_remote: 2,
    only_local: 1,
    status_drift: 0,
    product_drift: 3,
    owner_drift: 4,
    remote: 10,
    local: 9,
    pages: 1,
    skipped: 0,
    details: {
      only_remote: [{ barcode: "GL-0001", remote_id: "r1" }],
      only_local: [{ barcode: "GL-0002", unit_id: "u2" }],
      status_drift: [],
      product_drift: [],
      owner_drift: [],
    },
  },
  error: null,
};

const held = {
  uuid: OUTBOUND,
  order_uuid: "0b9c4c1e-0000-4000-8000-0000000000d1",
  order_no: "ORD-0001",
  order_status: "approved",
  external_reference: "olex-ORD-0001",
  state: "held",
  held_reason: "missing_customer_link",
  attempts: 1,
  last_error: null,
  created_at: NOW,
  updated_at: NOW,
};

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("glorian admin: masked key, test, replay, reconcile drift", async ({
  page,
}) => {
  const calls: string[] = [];
  const bodies: Record<string, unknown> = {};

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
      if (method === "GET" && path === BASE) return ok(connection);
      if (method === "PUT" && path === BASE) {
        bodies.put = req.postDataJSON();
        return ok(connection);
      }
      if (method === "POST" && path === `${BASE}/test`) {
        return ok({
          ok: true,
          duration_ms: 42,
          http_status: null,
          code: null,
          message: null,
        });
      }
      if (method === "GET" && path === `${BASE}/sync-runs`) {
        return ok({ items: [reconcileRun] });
      }
      if (method === "POST" && path === `${BASE}/sync-runs`) {
        return ok({ kind: "pull", task: "glorian:pull_catalog" }, 202);
      }
      if (method === "GET" && path === `${BASE}/sync-runs/${RUN}`) {
        return ok(reconcileRun);
      }
      if (method === "GET" && path === `${BASE}/outbounds`) {
        return ok({ items: [held] });
      }
      if (
        method === "POST" &&
        path === `${BASE}/outbounds/${OUTBOUND}/replay`
      ) {
        return ok(held, 202);
      }
      if (method === "POST" && path === `${BASE}/reconcile`) {
        return ok({ ...reconcileRun, status: "running", counts: {} }, 202);
      }
      return route.fulfill({
        status: 404,
        json: { error: { code: "NOT_FOUND", message: "Not mocked" } },
      });
    },
  );

  await page.goto("/platform/integrations/glorian");
  await expect(
    page.getByRole("heading", { name: "Glorian integration", level: 1 }),
  ).toBeVisible();

  const key = page.getByTestId("glorian-api-key");
  await expect(key).toHaveAttribute("type", "password");
  await expect(key).toHaveValue("");
  await expect(key).toHaveAttribute("placeholder", "********");

  await page.getByTestId("glorian-test").click();
  await expect(page.getByTestId("glorian-test-result")).toContainText(
    "Connected (42 ms)",
  );

  const drift = page.getByTestId("drift-report").first();
  await expect(drift.getByTestId("drift-count-only_remote")).toContainText("2");
  await expect(drift.getByTestId("drift-count-owner_drift")).toContainText("4");

  await page.getByTestId(`replay-${OUTBOUND}`).click();
  await expect(page.getByText("Replay queued for ORD-0001")).toBeVisible();
  expect(calls).toContain(`POST ${BASE}/outbounds/${OUTBOUND}/replay`);

  await page.getByTestId("reconcile-start").click();
  await expect(
    page.getByText(
      "Reconcile is running; the report appears when it finishes.",
    ),
  ).toBeVisible();
  expect(calls).toContain(`POST ${BASE}/reconcile`);
});
