import { expect, test, type Page } from "@playwright/test";

import {
  SERVICE_UUID,
  SLUG,
  mockApi,
  rollUnit,
  signIn,
  type MockApi,
} from "./support/mock-api";

/**
 * TEC-218: the stock picker of the service wizard (step 4, TEC-180/182):
 * listing, search, barcode scan (found / not found), stock limits for a
 * fixed barcode and a roll, the whole-roll option and removing an item.
 */

const kitUnit = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000f3",
  barcode: "OLX-KIT-010",
  unit_kind: "fixed",
  product: {
    uuid: "0b9c4c1e-0000-4000-8000-0000000000f4",
    sku: "KIT-CLEAN",
    name: "Olex Cleaning Kit",
    unit_type: "piece",
    available_parts: [],
  },
  quantity_on_hand: 3,
  initial_meters: null,
  remaining_meters: null,
};

const ceramicUnit = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000f5",
  barcode: "OLX-CER-001",
  unit_kind: "serial",
  product: {
    uuid: "0b9c4c1e-0000-4000-8000-0000000000f6",
    sku: "CER-9H",
    name: "Olex Ceramic 9H",
    unit_type: "piece",
    available_parts: [],
  },
  quantity_on_hand: 1,
  initial_meters: null,
  remaining_meters: null,
};

const stockCalls = (api: MockApi) =>
  api.calls.filter((c) =>
    c.startsWith(`GET /v1/services/${SERVICE_UUID}/stock-units?`),
  );

/** Opens the draft's wizard and walks to the stock step (hood picked). */
async function openStockStep(page: Page) {
  await page.goto(`/t/${SLUG}/services/${SERVICE_UUID}/wizard`);
  await expect(page.getByTestId("parts-step")).toBeVisible();
  await page.locator('path[data-part="body_kaput"]').click();
  await page.getByTestId("parts-next").click();
  await expect(page.getByTestId("measurement-step")).toBeVisible();
  await page.getByTestId("measurement-no").click();
  await page.getByTestId("measurement-save").click();
  await expect(page.getByTestId("stock-step")).toBeVisible();
}

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("stock picker: search, scan, limits, whole roll, remove", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.seedDraft(1000);
  api.stockUnits = [rollUnit, kitUnit, ceramicUnit];

  await openStockStep(page);
  const units = page.getByTestId("stock-units").locator("li");
  await expect(units).toHaveCount(3);
  await expect(page.locator('[data-unit="OLX-ROLL-001"]')).toContainText(
    "50.00",
  );
  await expect(page.locator('[data-unit="OLX-KIT-010"]')).toContainText("3");
  await expect(page.getByTestId("items-empty")).toBeVisible();

  // Search by product name / SKU (debounced, sent as q).
  const search = page.getByRole("searchbox");
  await search.fill("kit");
  await expect.poll(() => stockCalls(api).at(-1)).toContain("q=kit");
  await expect(units).toHaveCount(1);
  await expect(units.first()).toHaveAttribute("data-unit", "OLX-KIT-010");
  await search.fill("cer-9h");
  await expect(units).toHaveCount(1);
  await expect(units.first()).toHaveAttribute("data-unit", "OLX-CER-001");
  await search.fill("nothing-here");
  await expect(page.getByTestId("units-empty")).toBeVisible();
  await search.fill("");
  await expect(units).toHaveCount(3);

  // Scanner: an unknown barcode, then a known one (exact lookup).
  const scan = page.locator('input[name="barcode"]');
  await scan.fill("NOPE-404");
  await scan.press("Enter");
  await expect(page.getByTestId("scan-error")).toHaveText(
    "No available unit in your stock has this barcode.",
  );
  expect(stockCalls(api).at(-1)).toContain("barcode=NOPE-404");
  await scan.fill("OLX-KIT-010");
  await scan.press("Enter");
  await expect(page.getByTestId("scan-error")).toHaveCount(0);
  const form = page.getByTestId("add-unit-form");
  await expect(form).toContainText("Olex Cleaning Kit");
  await expect(scan).toHaveValue("");

  // Fixed barcode: at most what is on hand.
  const qty = form.locator('input[name="quantity"]');
  await qty.fill("5");
  await expect(page.getByTestId("quantity-error")).toHaveText(
    "Only 3 pcs are on hand.",
  );
  await expect(page.getByTestId("add-unit-submit")).toBeDisabled();
  await qty.fill("2");
  await page.getByTestId("add-unit-submit").click();
  await expect(page.getByTestId("service-item")).toHaveCount(1);
  expect(
    api.bodies[`POST /v1/services/${SERVICE_UUID}/items`]?.[0],
  ).toMatchObject({
    barcode: "OLX-KIT-010",
    kind: "full",
    quantity: 2,
    applied_parts: [],
  });

  // Roll: more than is left is refused, the whole roll is fine.
  await page
    .locator('[data-unit="OLX-ROLL-001"]')
    .getByTestId("pick-unit")
    .click();
  await form.locator('input[name="meters"]').fill("60");
  await expect(page.getByTestId("meters-error")).toHaveText(
    "Only 50.00 m are left on this roll.",
  );
  await expect(page.getByTestId("add-unit-submit")).toBeDisabled();
  await page.getByTestId("whole-roll").click();
  await expect(form.locator('input[name="meters"]')).toHaveCount(0);
  await page.getByTestId("add-unit-submit").click();
  await expect(page.getByTestId("service-item")).toHaveCount(2);
  const rollBody = api.bodies[`POST /v1/services/${SERVICE_UUID}/items`]?.[1];
  expect(rollBody).toMatchObject({
    barcode: "OLX-ROLL-001",
    kind: "full",
    applied_parts: ["body_kaput"],
  });
  expect(rollBody).not.toHaveProperty("meters");

  // Remove the kit line.
  await page
    .getByTestId("service-item")
    .filter({ hasText: "OLX-KIT-010" })
    .getByTestId("remove-item")
    .click();
  await expect(page.getByTestId("service-item")).toHaveCount(1);
  await expect(page.getByTestId("service-item")).toContainText("OLX-ROLL-001");
  expect(
    api.calls.some((c) =>
      c.startsWith(`DELETE /v1/services/${SERVICE_UUID}/items/`),
    ),
  ).toBe(true);
  await expect(page.getByTestId("complete-service")).toBeEnabled();
});
