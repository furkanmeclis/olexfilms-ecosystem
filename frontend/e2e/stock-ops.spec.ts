import { expect, test } from "@playwright/test";

import { signIn } from "./support/mock-api";
import { chooseOption } from "./support/pickers";
import {
  APPROVER,
  mockStockOps,
  NEW_BARCODE,
  OPS_SLUG,
  ROLL_BARCODE,
  TARGET,
} from "./support/stock-ops-mock";

/**
 * TEC-763: the stock operation screens against a mocked BFF — a center
 * warehouse user scans a roll, opens its movement history, splits meters
 * off as a new unit and prints its label; then requests a product fix,
 * cannot approve it, and a second center user approves it after a
 * password step-up.
 */

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("scan → unit history → split roll → label", async ({ page }) => {
  const api = await mockStockOps(page);

  await page.goto(`/t/${OPS_SLUG}/warehouse/scan`);
  await page.getByTestId("scan-input").fill(`OFW:UNIT:${ROLL_BARCODE}`);
  await page.getByTestId("scan-input").press("Enter");
  await expect(page.getByTestId("scan-result")).toHaveAttribute(
    "data-type",
    "unit",
  );
  await page.getByTestId("unit-history-link").click();

  await expect(page).toHaveURL(
    new RegExp(`/t/${OPS_SLUG}/stock/units/${ROLL_BARCODE}$`),
  );
  await expect(page.getByTestId("unit-meters")).toHaveText("50.00 / 50.00 m");
  const movements = page.getByTestId("unit-movement");
  await expect(movements).toHaveCount(2);
  await expect(movements.first()).toContainText("Stock entry");
  await expect(movements.first()).toContainText("by Deniz Depo");
  await expect(
    movements.first().getByTestId("unit-movement-document"),
  ).toHaveAttribute("href", /\/warehouse\/entries\//);

  await page.getByTestId("unit-split").click();
  const dialog = page.getByTestId("split-dialog");
  await dialog.locator("#split-meters").fill("60");
  await dialog.getByTestId("split-submit").click();
  await expect(dialog.getByTestId("split-error")).toContainText(
    "The roll has only 50.00 m",
  );
  expect(api.bodies.split).toBeUndefined();

  await dialog.locator("#split-meters").fill("12.5");
  await expect(dialog.getByTestId("split-preview")).toHaveText(
    "New unit: 12.50 m · stays on the roll: 37.50 m",
  );
  await dialog.getByTestId("split-submit").click();
  await expect(dialog.getByTestId("split-done")).toContainText(NEW_BARCODE);
  expect(api.bodies.split).toEqual([
    expect.objectContaining({
      barcode: ROLL_BARCODE,
      meters: "12.50",
      idempotency_key: expect.stringMatching(/^split:/),
    }),
  ]);

  const download = page.waitForEvent("download");
  await dialog.getByTestId("split-label").click();
  expect((await download).suggestedFilename()).toBe(`${NEW_BARCODE}.pdf`);
  expect(api.bodies.labels).toEqual([[NEW_BARCODE]]);

  // The history refreshes with the split.
  await page.keyboard.press("Escape");
  await expect(movements).toHaveCount(3);
  await expect(page.getByTestId("unit-meters")).toHaveText("37.50 / 50.00 m");
  expect(api.unknown.filter((c) => c.includes("/v1/stock/"))).toEqual([]);
});

test("product fix request → another center user approves with step-up", async ({
  page,
}) => {
  const api = await mockStockOps(page);

  await page.goto(`/t/${OPS_SLUG}/stock/units/${ROLL_BARCODE}`);
  await page.getByTestId("unit-reclassify").click();
  const dialog = page.getByTestId("reclass-dialog");
  await dialog.getByTestId("reclass-submit").click();
  await expect(dialog.getByText("Pick the correct product.")).toBeVisible();
  expect(api.bodies.reclass).toBeUndefined();

  await chooseOption(dialog.locator("#reclass-target"), TARGET.uuid);
  await dialog.locator("#reclass-reason").fill("Label printed on PPF 190");
  await dialog.getByTestId("reclass-submit").click();
  await expect(dialog.getByTestId("reclass-created")).toBeVisible();
  expect(api.bodies.reclass).toEqual([
    {
      barcode: ROLL_BARCODE,
      to_product_uuid: TARGET.uuid,
      reason: "Label printed on PPF 190",
    },
  ]);

  // The requester sees why it cannot approve its own request.
  await dialog.getByRole("link", { name: "Open request" }).click();
  await expect(page.getByTestId("reclass-detail")).toHaveAttribute(
    "data-status",
    "pending",
  );
  await expect(page.getByTestId("reclass-self-hint")).toBeVisible();
  await expect(page.getByTestId("reclass-approve")).toHaveCount(0);

  // A second center user approves it after the password step-up.
  api.actAs("approver");
  await page.reload();
  await expect(page.getByTestId("reclass-self-hint")).toHaveCount(0);
  await page.getByTestId("reclass-approve").click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Approve" })
    .click();
  await expect(page.getByText("Confirm your identity")).toBeVisible();
  await page.getByRole("textbox", { name: "Password" }).fill("e2e-password-1");
  await page.getByRole("button", { name: "Verify" }).click();

  await expect(page.getByTestId("reclass-detail")).toHaveAttribute(
    "data-status",
    "approved",
  );
  expect(api.bodies.approve).toEqual([{ user: APPROVER, stepUp: true }]);

  // The request list shows it approved.
  await page.goto(`/t/${OPS_SLUG}/stock/reclassifications`);
  await expect(page.getByTestId("reclass-row")).toHaveText(ROLL_BARCODE);
  await expect(page.getByText("Approved").first()).toBeVisible();
  expect(api.unknown.filter((c) => c.includes("/v1/stock/"))).toEqual([]);
});
