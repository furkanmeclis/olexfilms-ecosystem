import { expect, test, type Route } from "@playwright/test";

import { signIn } from "./support/mock-api";

/**
 * TEC-222: the platform system settings hub against a mocked BFF — the
 * TEC-215 catalog grouped on the page, a client-side range check, a PUT
 * save, a DELETE reset, a 400 VALIDATION_ERROR shown on its field and the
 * masked SMTP password.
 */

const USER = "0b9c4c1e-0000-4000-8000-000000000002";
const PERMISSIONS = ["platform.settings.read", "platform.settings.write"];

type Setting = Record<string, unknown> & { key: string };

const envelope = (data: unknown) => ({ success: true, data, meta: {} });

function catalog(): Setting[] {
  return [
    {
      key: "contract_grace_days",
      group: "contracts",
      kind: "int",
      default: 0,
      description: "Read-only grace period after a contract expires",
      min: 0,
      max: 365,
      value: 0,
      is_default: true,
      schema_version: 1,
    },
    {
      key: "forecast_min_days",
      group: "forecast",
      kind: "int",
      default: 30,
      description: "Minimum forecast window",
      min: 1,
      max: 3650,
      value: 60,
      is_default: false,
      schema_version: 1,
      updated_at: "2026-10-01T09:00:00Z",
    },
    {
      key: "smtp.host",
      group: "smtp",
      kind: "string",
      default: "",
      description: "SMTP host",
      max_len: 253,
      value: "",
      is_default: true,
      schema_version: 1,
    },
    {
      key: "smtp.password",
      group: "smtp",
      kind: "string",
      default: "",
      description: "SMTP password",
      max_len: 255,
      secret: true,
      value: "********",
      is_default: false,
      schema_version: 1,
    },
  ];
}

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

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("system settings: list, validate, save, reset, masked secret", async ({
  page,
}) => {
  const settings = catalog();
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
      calls.push(`${method} ${path}`);
      const ok = (data: unknown) =>
        route.fulfill({ status: 200, json: envelope(data) });

      if (method === "GET" && path === "/v1/auth/me") return ok(me());
      if (method === "GET" && path === "/v1/auth/step-up") {
        return ok({ valid: true, expires_at: null, methods: [] });
      }
      if (method === "GET" && path === "/v1/platform/system-settings") {
        return ok({ items: settings });
      }
      const one = path.match(/^\/v1\/platform\/system-settings\/(.+)$/);
      if (one) {
        const key = decodeURIComponent(one[1]);
        const i = settings.findIndex((s) => s.key === key);
        if (method === "PUT") {
          const body = req.postDataJSON() as { value: unknown };
          bodies[key] = body;
          if (key === "smtp.host") {
            return route.fulfill({
              status: 400,
              json: {
                success: false,
                error: {
                  code: "VALIDATION_ERROR",
                  message: "Validation failed",
                  details: [{ field: "value", message: "host is not allowed" }],
                },
              },
            });
          }
          settings[i] = {
            ...settings[i],
            value: body.value,
            is_default: false,
            updated_at: "2026-10-03T09:00:00Z",
          };
          return ok(settings[i]);
        }
        if (method === "DELETE") {
          settings[i] = {
            ...settings[i],
            value: settings[i].default,
            is_default: true,
            updated_at: undefined,
          };
          return ok(settings[i]);
        }
      }
      return route.fulfill({
        status: 404,
        json: { error: { code: "NOT_FOUND", message: "Not mocked" } },
      });
    },
  );

  await page.goto("/platform/system-settings");
  await expect(
    page.getByRole("heading", { name: "System settings", level: 1 }),
  ).toBeVisible();
  await expect(page.getByTestId("group-contracts")).toBeVisible();
  await expect(page.getByTestId("group-smtp")).toBeVisible();

  const grace = page.getByTestId("setting-contract_grace_days");
  const graceInput = grace.locator("input");
  await expect(graceInput).toHaveValue("0");

  await graceInput.fill("500");
  await expect(grace.getByRole("alert")).toHaveText("Must be at most 365.");
  await expect(grace.getByRole("button", { name: "Save" })).toBeDisabled();

  await graceInput.fill("14");
  await grace.getByRole("button", { name: "Save" }).click();
  await expect(grace.getByText("Custom")).toBeVisible();
  expect(bodies["contract_grace_days"]).toEqual({ value: 14 });

  const forecast = page.getByTestId("setting-forecast_min_days");
  await forecast.getByRole("button", { name: "Reset to default" }).click();
  await expect(forecast.locator("input")).toHaveValue("30");
  expect(calls).toContain(
    "DELETE /v1/platform/system-settings/forecast_min_days",
  );

  const host = page.getByTestId("setting-smtp.host");
  await host.locator("input").fill("smtp.blocked.example");
  await host.getByRole("button", { name: "Save" }).click();
  await expect(host.getByRole("alert")).toHaveText("host is not allowed");

  const password = page.getByTestId("setting-smtp.password").locator("input");
  await expect(password).toHaveAttribute("type", "password");
  await expect(password).toHaveValue("");
  await expect(password).toHaveAttribute("placeholder", "********");
});
