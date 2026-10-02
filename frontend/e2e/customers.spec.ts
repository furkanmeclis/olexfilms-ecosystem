import { expect, test } from "@playwright/test";

import {
  CUSTOMER_SLUG,
  CUSTOMER_UUID,
  mockCustomers,
} from "./support/customer-mock";
import { signIn } from "./support/mock-api";

/**
 * TEC-163: customers against a mocked BFF — a center admin creates a
 * customer, adds a vehicle (plate format checked in the browser), finds
 * the customer by search and anonymizes it.
 */

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("customer: create → vehicle → search → anonymize", async ({ page }) => {
  const api = await mockCustomers(page);
  const detailUrl = new RegExp(
    `/t/${CUSTOMER_SLUG}/customers/${CUSTOMER_UUID}$`,
  );

  await page.goto(`/t/${CUSTOMER_SLUG}/customers`);
  await expect(page.getByTestId("customers-empty")).toBeVisible();
  await page.getByTestId("new-customer").click();
  await expect(
    page.getByRole("heading", { name: "New customer" }),
  ).toBeVisible();

  // Client validation first, then a valid create.
  await page.getByTestId("customer-submit").click();
  await expect(page.locator('[data-error="name"]')).toBeVisible();
  await page.locator("#customer-phone").fill("0555 123 45 67");
  await page.locator("#customer-name").fill("Ayşe");
  await page.locator("#customer-surname").fill("Yılmaz");
  await page.getByTestId("customer-submit").click();
  await expect(page).toHaveURL(detailUrl);
  expect(api.bodies["POST /v1/customers"]?.[0]).toEqual({
    phone: "0555 123 45 67",
    name: "Ayşe",
    surname: "Yılmaz",
    type: "individual",
  });
  await expect(
    page.getByRole("heading", { name: "Ayşe Yılmaz" }),
  ).toBeVisible();
  await expect(page.locator('[data-action="anonymize"]')).toBeVisible();
  await expect(page.locator('[data-action="export"]')).toBeVisible();

  // Vehicle: a plate outside the TR format is refused in the browser.
  await page.locator('[data-action="add-vehicle"]').click();
  await page.locator("#vehicle-plate").fill("99 ABC 123");
  await page.getByTestId("vehicle-submit").click();
  await expect(page.locator('[data-error="plate"]')).toBeVisible();
  expect(api.bodies["POST /v1/vehicles"]).toBeUndefined();
  await page.locator("#vehicle-plate").fill("34 ABC 123");
  await page.getByTestId("vehicle-submit").click();
  await expect(page.getByTestId("vehicle-row")).toHaveCount(1);
  expect(api.bodies["POST /v1/vehicles"]?.[0]).toMatchObject({
    customer_uuid: CUSTOMER_UUID,
    plate: "34 ABC 123",
    plate_country: "TR",
  });

  // Search on the list.
  await page.goto(`/t/${CUSTOMER_SLUG}/customers`);
  await page.locator("#customer-search").fill("ayşe");
  await expect(page.getByTestId("customer-row")).toHaveCount(1);
  await expect
    .poll(() => api.calls.some((c) => /\/v1\/customers\?.*q=/.test(c)))
    .toBe(true);
  await page.getByTestId("customer-row").getByRole("link").click();
  await expect(page).toHaveURL(detailUrl);

  // Anonymize (typed confirmation).
  await page.locator('[data-action="anonymize"]').click();
  await page.locator("#anonymize-confirm").fill("ANONYMIZE");
  await page.getByTestId("anonymize-confirm-button").click();
  await expect(page.getByTestId("anonymized-notice")).toBeVisible();
  await expect(page.locator('[data-action="anonymize"]')).toHaveCount(0);
  expect(
    api.unknown.filter((c) => /\/v1\/(customers|vehicles)/.test(c)),
  ).toEqual([]);
});
