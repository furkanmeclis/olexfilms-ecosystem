import { expect, test, type Route } from "@playwright/test";

import { signIn } from "./support/mock-api";

/**
 * TEC-353: platform "Review questions" against a mocked BFF — the
 * client-side list, a new question from the dialog (type, target,
 * required, Turkish + English tabs) saved as POST then one PUT per
 * language, the new row in the table.
 */

const USER = "0b9c4c1e-0000-4000-8000-000000000002";
const PERMISSIONS = ["platform.settings.read", "reviews.questions.manage"];
const NEW = "0b9c4c1e-0000-4000-8000-000000003539";
const AT = "2026-10-01T00:00:00Z";

type Question = Record<string, unknown> & {
  uuid: string;
  locales: { locale: string; text: string; updated_at: string }[];
};

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
    grants: Object.fromEntries(PERMISSIONS.map((p) => [p, "brand"])),
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

test("review questions: list, create with language tabs", async ({ page }) => {
  const questions: Question[] = [
    {
      uuid: "0b9c4c1e-0000-4000-8000-000000003538",
      question_key: "staff",
      question_type: "rating_1_5",
      target: "dealer",
      is_required: true,
      is_active: true,
      sort_order: 10,
      locales: [{ locale: "en", text: "How was the staff?", updated_at: AT }],
      created_at: AT,
      updated_at: AT,
    },
  ];
  const calls: string[] = [];
  const bodies: unknown[] = [];

  await page.route(
    (url) =>
      url.pathname.startsWith("/api/") &&
      !url.pathname.startsWith("/api/auth/"),
    async (route: Route) => {
      const req = route.request();
      const path = new URL(req.url()).pathname.replace(/^\/api/, "");
      const method = req.method();
      calls.push(`${method} ${path}`);
      const ok = (data: unknown, status = 200) =>
        route.fulfill({ status, json: envelope(data) });

      if (method === "GET" && path === "/v1/auth/me") return ok(me());
      if (method === "GET" && path === "/v1/auth/step-up") {
        return ok({ valid: true, expires_at: null, methods: [] });
      }
      if (path === "/v1/platform/review-questions") {
        if (method === "GET") return ok({ items: questions });
        if (method === "POST") {
          const body = req.postDataJSON() as Record<string, unknown>;
          bodies.push(body);
          const q: Question = {
            ...body,
            uuid: NEW,
            locales: [],
            created_at: AT,
            updated_at: AT,
          };
          questions.push(q);
          return ok(q, 201);
        }
      }
      const loc = path.match(
        /^\/v1\/platform\/review-questions\/([^/]+)\/locales\/([^/]+)$/,
      );
      if (method === "PUT" && loc) {
        const body = req.postDataJSON() as { text: string };
        bodies.push({ locale: loc[2], ...body });
        const q = questions.find((x) => x.uuid === loc[1]);
        q?.locales.push({ locale: loc[2], text: body.text, updated_at: AT });
        return ok({ locale: loc[2], text: body.text, updated_at: AT });
      }
      return route.fulfill({
        status: 404,
        json: { error: { code: "NOT_FOUND", message: "Not mocked" } },
      });
    },
  );

  await page.goto("/platform/review-questions");
  await expect(
    page.getByRole("heading", { name: "Review questions", level: 1 }),
  ).toBeVisible();
  await expect(page.getByText("How was the staff?")).toBeVisible();

  await page.getByRole("button", { name: "New question" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await dialog.getByLabel("Key").fill("film_quality");
  await dialog.getByRole("combobox", { name: "Target" }).click();
  await page.getByRole("option", { name: "Product (per product)" }).click();
  await dialog.getByLabel("Required").click();

  // Turkish is required: saving without it shows the error.
  await dialog.getByRole("button", { name: "Save" }).click();
  await expect(
    dialog.getByText("The Turkish question text is required."),
  ).toBeVisible();
  await dialog.locator("textarea[name=text_tr]").fill("Film kalitesi nasıldı?");
  await dialog.getByRole("tab", { name: "English" }).click();
  await dialog
    .locator("textarea[name=text_en]")
    .fill("How was the film quality?");
  await dialog.getByRole("tab", { name: "العربية" }).click();
  await expect(dialog.locator("textarea[name=text_ar]")).toHaveAttribute(
    "dir",
    "rtl",
  );
  await dialog.getByRole("button", { name: "Save" }).click();

  await expect(dialog).toBeHidden();
  await expect(page.getByText("How was the film quality?")).toBeVisible();
  expect(bodies).toEqual([
    {
      question_key: "film_quality",
      question_type: "rating_1_5",
      target: "product",
      is_required: true,
      is_active: true,
      sort_order: 20,
    },
    { locale: "tr", text: "Film kalitesi nasıldı?" },
    { locale: "en", text: "How was the film quality?" },
  ]);
});
