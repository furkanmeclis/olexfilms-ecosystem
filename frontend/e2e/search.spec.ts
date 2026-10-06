import { expect, test } from "@playwright/test";

import { signIn } from "./support/mock-api";
import { KIT, mockStock, ROLL, STOCK_SLUG } from "./support/stock-mock";

/**
 * TEC-213: Cmd+K global search against a mocked BFF — a barcode typed into
 * the palette lists the stock unit group, the keyboard picks the hit and
 * Enter opens My stock filtered on that barcode.
 */

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("cmd+k: barcode search opens the unit in My stock", async ({ page }) => {
  const api = await mockStock(page);

  await page.goto(`/t/${STOCK_SLUG}`);
  await expect(page.getByTestId("stock-summary-widget")).toBeVisible();
  await page.keyboard.press("ControlOrMeta+k");
  const input = page.locator("[cmdk-input]");
  await expect(input).toBeVisible();
  await input.fill(ROLL.barcode);

  const unit = page.locator("[cmdk-item]", { hasText: ROLL.barcode });
  await expect(unit).toBeVisible();
  await expect(
    page.locator("[cmdk-item]", { hasText: KIT.barcode }),
  ).toHaveCount(0);
  expect(
    api.calls.some(
      (c) =>
        c.startsWith("GET /v1/search/global?") && c.includes("OLX-ROLL-001"),
    ),
  ).toBe(true);

  // The barcode group heads the list; Enter opens the selected hit.
  await expect(page.locator("[cmdk-group-heading]").first()).toHaveText(
    "Stock units",
  );
  await expect(unit).toHaveAttribute("aria-selected", "true");
  await input.press("Enter");

  await expect(page).toHaveURL(
    new RegExp(`/t/${STOCK_SLUG}/stock\\?barcode=OLX-ROLL-001$`),
  );
  await expect(page.locator("#stock-barcode")).toHaveValue(ROLL.barcode);
  const rows = page
    .getByRole("row")
    .filter({ has: page.getByTestId("stock-row") });
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText(ROLL.barcode);
});
