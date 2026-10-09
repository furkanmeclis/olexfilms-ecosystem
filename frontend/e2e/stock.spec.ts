import { expect, test } from "@playwright/test";

import { signIn } from "./support/mock-api";
import { openOptions } from "./support/pickers";
import {
  DEALER_UUID,
  KIT,
  mockStock,
  ROLL,
  STOCK_SLUG,
} from "./support/stock-mock";

/**
 * TEC-224: the dealer "My stock" page against a mocked BFF — the on-hand
 * list without a purchase price column, the consumed tab, the barcode
 * filter, the home widget totals and the distributor's dealer picker.
 */

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("my stock: list, consumed tab, barcode filter, widget", async ({
  page,
}) => {
  const api = await mockStock(page);

  await page.goto(`/t/${STOCK_SLUG}/stock`);
  await expect(page.getByRole("heading", { name: "My stock" })).toBeVisible();
  const rows = page
    .getByRole("row")
    .filter({ has: page.getByTestId("stock-row") });
  await expect(rows).toHaveCount(2);
  await expect(rows.first()).toContainText("OLX-ROLL-001");
  await expect(rows.first()).toContainText("37.50 / 50.00 m");
  // No price in the answer: the purchase price column stays hidden.
  await expect(
    page.getByRole("table").getByText("Purchase price", { exact: true }),
  ).toHaveCount(0);
  await expect(page.getByTestId("stock-dealer")).toHaveCount(0);

  await page.getByTestId("stock-tab-consumed").click();
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText("OLX-ROLL-002");
  expect(
    api.calls.some((c) => c.includes("/units?") && c.includes("status=used")),
  ).toBe(true);

  await page.getByTestId("stock-tab-stock").click();
  await page.locator("#stock-barcode").fill(KIT.barcode);
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText("Olex Cleaning Kit");
  await page.getByRole("button", { name: "Clear filter" }).click();
  await expect(page.locator("#stock-barcode")).toHaveValue("");
  await expect(rows).toHaveCount(2);

  await page.goto(`/t/${STOCK_SLUG}`);
  const widget = page.getByTestId("stock-summary-widget");
  await expect(widget).toBeVisible();
  await expect(widget.getByTestId("stock-widget-products")).toHaveText("2");
  await expect(widget.getByTestId("stock-widget-quantity")).toHaveText("3");
  await expect(widget.getByTestId("stock-widget-meters")).toHaveText("37.5");
});

test("my stock: a distributor picks a dealer of its subtree", async ({
  page,
}) => {
  const api = await mockStock(page);
  api.actAs("distributor");
  api.units[DEALER_UUID] = [
    {
      ...ROLL,
      uuid: "0b9c4c1e-0000-4000-8000-0000000000f9",
      barcode: "OLX-ROLL-DEALER",
      purchase_price: { amount: "120.00", currency: "TRY", source: "list" },
    },
  ];

  await page.goto(`/t/${STOCK_SLUG}/stock`);
  const picker = page.getByTestId("stock-dealer");
  await expect(picker).toBeVisible();
  // Own stock plus the one dealer of the subtree.
  await expect(await openOptions(picker)).toHaveCount(2);
  await page.locator(`[role="option"][data-value="${DEALER_UUID}"]`).click();
  const rows = page
    .getByRole("row")
    .filter({ has: page.getByTestId("stock-row") });
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText("OLX-ROLL-DEALER");
  // The distributor sees the dealer's purchase price (K8).
  await expect(
    page.getByRole("table").getByText("Purchase price", { exact: true }),
  ).toBeVisible();
  await expect(page.getByTestId("stock-price").first()).toContainText("120");
  await expect(page.getByText("read-only")).toBeVisible();
  expect(
    api.calls.some((c) =>
      c.startsWith(`GET /v1/stock/organizations/${DEALER_UUID}/units?`),
    ),
  ).toBe(true);
});
