import { expect, test, type Page } from "@playwright/test";

import { signIn } from "./support/mock-api";
import {
  FIXED_BARCODE,
  ORDER_SLUG,
  ORDER_UUID,
  PRODUCT,
  mockOrders,
} from "./support/order-mock";

/**
 * TEC-170: an order against a mocked BFF — the distributor creates and
 * submits it, the center approves, prepares (barcode), marks it ready and
 * ships, then the distributor receives it.
 */

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

async function act(page: Page, kind: string) {
  await page.locator(`[data-action="${kind}"]`).click();
  await page.getByTestId("action-confirm").click();
}

test("order: create → approve → ship → receive", async ({ page }) => {
  const api = await mockOrders(page);
  const detailUrl = new RegExp(`/t/${ORDER_SLUG}/orders/${ORDER_UUID}$`);

  // Buyer (distributor): new order with one product, then submit.
  api.actAs("buyer");
  await page.goto(`/t/${ORDER_SLUG}/orders/new`);
  await expect(page.getByRole("heading", { name: "New order" })).toBeVisible();
  await page.locator("#order-product-search").fill("bakım");
  await page.getByTestId("product-add").first().click();
  const line = page.getByTestId("order-line");
  await expect(line).toHaveCount(1);
  await expect(line.getByTestId("line-price-preview")).toContainText("80");
  await line.locator('input[name="amount"]').fill("2");
  await page.getByTestId("submit-order").click();

  await expect(page).toHaveURL(detailUrl);
  expect(api.bodies["POST /v1/orders"]?.[0]).toEqual({
    items: [{ product_uuid: PRODUCT.uuid, quantity: 2 }],
  });
  expect(api.bodies[`POST /v1/orders/${ORDER_UUID}/transitions`]?.[0]).toEqual({
    status: "submitted",
  });
  await expect(
    page.getByRole("heading", { name: "ORD-00000007" }),
  ).toBeVisible();
  await expect(page.getByTestId("order-total")).toContainText("160");
  // The buyer may only cancel a submitted order.
  await expect(page.locator("[data-action]")).toHaveCount(1);
  await expect(page.locator('[data-action="cancel"]')).toBeVisible();

  // Seller (center): incoming list, approve, prepare, scan, ready, ship.
  api.actAs("seller");
  await page.goto(`/t/${ORDER_SLUG}/orders`);
  await expect(page.getByTestId("order-row")).toHaveCount(1);
  await page.getByTestId("order-row").getByRole("link").click();
  await expect(page).toHaveURL(detailUrl);
  await expect(page.locator('[data-action="reject"]')).toBeVisible();
  await act(page, "approve");
  await expect(page.getByTestId("order-rate")).toContainText("35.1234");

  await act(page, "start_preparing");
  await expect(page.getByTestId("assign-form")).toBeVisible();
  await expect(page.locator('[data-action="mark_ready"]')).toBeDisabled();
  await page.locator('input[name="barcode"]').fill(FIXED_BARCODE);
  await page.locator('input[name="quantity"]').fill("2");
  await page.getByTestId("assign-submit").click();
  await expect(page.getByTestId("assigned-unit")).toContainText(FIXED_BARCODE);
  await expect(page.getByTestId("line-assigned")).toHaveText("2 / 2");
  expect(
    api.bodies[
      `POST /v1/orders/${ORDER_UUID}/items/0b9c4c1e-0000-4000-8000-0000000000a8/units`
    ]?.[0],
  ).toEqual({ barcode: FIXED_BARCODE, quantity: 2 });

  await act(page, "mark_ready");
  await expect(page.locator('[data-action="ship"]')).toBeVisible();
  await act(page, "ship");
  await expect(page.locator('[data-action="cancel"]')).toHaveCount(0);
  await expect(page.locator('[data-action="request_cancel"]')).toBeVisible();

  // Buyer: receive the shipment.
  api.actAs("buyer");
  await page.reload();
  await expect(page.locator('[data-action="receive"]')).toBeVisible();
  await act(page, "receive");
  await expect(page.locator("[data-action]")).toHaveCount(0);
  await expect(page.getByTestId("history-row")).toHaveCount(7);

  const moves = (api.bodies[`POST /v1/orders/${ORDER_UUID}/transitions`] ??
    []) as { status: string }[];
  expect(moves.map((m) => m.status)).toEqual([
    "submitted",
    "approved",
    "preparing",
    "ready",
    "shipped",
    "received",
  ]);
  // Shell calls (realtime token, search specs) are not part of the flow.
  expect(
    api.unknown.filter((c) => /\/v1\/(orders|catalog|tenant)/.test(c)),
  ).toEqual([]);
});
