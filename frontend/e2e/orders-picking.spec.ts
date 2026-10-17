import { expect, test } from "@playwright/test";

import { signIn } from "./support/mock-api";
import {
  DELIVERY_METHODS,
  FIXED_BARCODE,
  ITEM_UUID,
  ORDER_SLUG,
  ORDER_UUID,
  SHIPPING_NOTE_PDF,
  mockOrders,
} from "./support/order-mock";

/**
 * TEC-768 (Depo F4.3) against a mocked BFF: the center prepares an order
 * with "suggest and fill", marks it ready, ships it with a delivery method
 * and a tracking number, then downloads the shipping note.
 */

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("order: suggest and fill → ready → ship with tracking → shipping note", async ({
  page,
}) => {
  const api = await mockOrders(page);
  api.seed("preparing", 3);
  api.actAs("seller");
  await page.goto(`/t/${ORDER_SLUG}/orders/${ORDER_UUID}`);
  await expect(page.locator('[data-action="mark_ready"]')).toBeDisabled();
  await expect(page.getByTestId("order-pick-list")).toBeVisible();

  // Suggest and fill: the proposal is shown, nothing is assigned yet.
  await page.getByTestId("suggest-open").click();
  const dialog = page.getByTestId("suggest-dialog");
  await expect(dialog.getByTestId("suggestion")).toHaveCount(1);
  await expect(dialog.getByTestId("suggestion")).toContainText(FIXED_BARCODE);
  await expect(dialog.getByTestId("suggestion")).toContainText("D1-R1-A-01-01");
  await dialog.getByTestId("suggest-unlocated").click();
  await expect
    .poll(() =>
      api.calls.some((c) =>
        c.endsWith(`/items/${ITEM_UUID}/suggestions?include_unlocated=true`),
      ),
    )
    .toBe(true);
  await dialog.getByTestId("suggest-apply").click();
  await expect(dialog).toBeHidden();
  await expect(page.getByTestId("assigned-unit")).toContainText(FIXED_BARCODE);
  await expect(page.getByTestId("line-assigned")).toHaveText("3 / 3");
  expect(
    api.bodies[
      `POST /v1/orders/${ORDER_UUID}/items/${ITEM_UUID}/suggestions/apply`
    ]?.[0],
  ).toEqual({
    units: [{ unit_uuid: "0b9c4c1e-0000-4000-8000-0000000000a9", quantity: 3 }],
  });

  // Ready, then ship with the carrier and the tracking number.
  await page.locator('[data-action="mark_ready"]').click();
  await page.getByTestId("action-confirm").click();
  await page.locator('[data-action="ship"]').click();
  const confirm = page.getByTestId("action-confirm");
  await expect(confirm).toBeDisabled();
  await page.getByTestId("ship-method").click();
  await page.getByRole("option", { name: "Aras Kargo" }).click();
  await page.locator("#ship-tracking").fill("AR 123");
  await confirm.click();

  const moves = api.bodies[`POST /v1/orders/${ORDER_UUID}/transitions`] ?? [];
  await expect.poll(() => moves.length).toBe(2);
  expect(moves[1]).toEqual({
    status: "shipped",
    delivery_method_uuid: DELIVERY_METHODS[1].uuid,
    tracking_no: "AR 123",
  });
  await expect(page.getByTestId("shipping-method")).toHaveText("Aras Kargo");
  await expect(page.getByTestId("tracking-link")).toHaveAttribute(
    "href",
    "https://track.example/q?no=AR%20123",
  );

  // The shipping note downloads as a PDF.
  const [download] = await Promise.all([
    page.waitForEvent("download"),
    page.getByTestId("shipping-document-download").click(),
  ]);
  expect(download.suggestedFilename()).toBe("shipping-note-ORD-00000007.pdf");
  const path = await download.path();
  const { readFile } = await import("node:fs/promises");
  expect(await readFile(path, "utf8")).toBe(SHIPPING_NOTE_PDF);

  expect(
    api.unknown.filter((c) => /\/v1\/(orders|delivery-methods)/.test(c)),
  ).toEqual([]);
});
