import { expect, test } from "@playwright/test";

import { mockApi, signIn, SLUG, type MockApi } from "./support/mock-api";

/**
 * TEC-509 (F5-10b): the dealer's Özellikler page against a mocked BFF —
 * level groups with counts, price lines, a module off at the level above
 * without a request button, and the request flow (note over 1000
 * characters is refused, the request is sent, pending badge, withdraw).
 */

const NOW = "2026-10-09T09:00:00Z";

type Item = Record<string, unknown>;

function item(key: string, level: string, extra: Item = {}): Item {
  return {
    key,
    level,
    enabled: level !== "addon",
    visible: true,
    paid: level === "addon",
    default_enabled: level !== "addon",
    source: level === "core" ? "core" : "default",
    upstream_enabled: true,
    admin_override: false,
    description: `${key} description`,
    free_default: level !== "addon",
    price: null,
    contact_for_price: false,
    request: null,
    ...extra,
  };
}

function setup(api: MockApi) {
  api.permissions.push("modules.read");
  const items: Item[] = [
    item("organizations", "core"),
    item("tasks", "core"),
    item("leads", "standard"),
    item("fleet", "addon", {
      price: {
        amount: "250.00",
        currency: "TRY",
        recurrence: "monthly",
        item_uuid: "0b9c4c1e-0000-4000-8000-000000000509",
        item_name: "Fleet bundle",
      },
    }),
    item("efficiency", "addon", {
      upstream_enabled: false,
      contact_for_price: true,
    }),
  ];
  api.featureItems = items;
  const fleet = items[3];
  api.extra.push(async ({ method, path, body, ok }) => {
    if (path !== "/v1/features/fleet/request") return false;
    if (method === "POST") {
      fleet.request = {
        uuid: "0b9c4c1e-0000-4000-8000-0000000005a1",
        status: "pending",
        note: (body as { note?: string } | undefined)?.note ?? "",
        decision_note: "",
        created_at: NOW,
        decided_at: null,
      };
      await ok(
        { status: "requested", recipients: 1, request: fleet.request },
        202,
      );
      return true;
    }
    if (method === "DELETE") {
      fleet.request = {
        ...(fleet.request as Item),
        status: "cancelled",
      };
      await ok(fleet.request);
      return true;
    }
    return false;
  });
}

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("Özellikler: level groups, prices and the request flow", async ({
  page,
}) => {
  const api = await mockApi(page);
  setup(api);
  await page.goto(`/t/${SLUG}/features`);

  const addon = page.getByTestId("module-level-addon");
  await expect(page.getByTestId("module-level-core")).toContainText("Core");
  await expect(page.getByTestId("module-level-core")).toContainText("2");
  await expect(addon).toContainText("Add-on");
  await expect(addon).toContainText("fleet description");

  await expect(page.getByTestId("module-price-leads")).toHaveText(
    "Free — on by default, no service record needed",
  );
  await expect(page.getByTestId("module-price-fleet")).toContainText("Paid: ");
  await expect(page.getByTestId("module-price-fleet")).toContainText("/month");
  await expect(page.getByTestId("module-price-efficiency")).toHaveText(
    "Contact us for the price",
  );

  await expect(addon.getByTestId("module-upstream-closed")).toHaveText(
    "Off at the level above",
  );
  await expect(page.getByTestId("module-request-efficiency")).toHaveCount(0);
  await expect(page.getByTestId("module-request-organizations")).toHaveCount(0);

  await page.getByTestId("module-request-fleet").click();
  const note = page.getByTestId("module-request-note");
  await note.fill("x".repeat(1001));
  await expect(page.getByTestId("module-request-note-error")).toHaveText(
    "The note can be at most 1000 characters.",
  );
  await expect(page.getByTestId("module-request-submit")).toBeDisabled();

  await note.fill("We have 12 fleet customers");
  await page.getByTestId("module-request-submit").click();
  await expect(addon.getByTestId("module-request-pending")).toBeVisible();
  expect(api.bodies["POST /v1/features/fleet/request"]).toEqual([
    { note: "We have 12 fleet customers" },
  ]);

  await page.getByTestId("module-request-cancel-fleet").click();
  await expect(page.getByTestId("module-request-fleet")).toBeVisible();
  expect(api.unknown).toEqual([]);
});
