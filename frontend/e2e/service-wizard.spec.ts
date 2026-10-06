import { expect, test } from "@playwright/test";

import {
  SERVICE_UUID,
  SLUG,
  customer,
  mockApi,
  signIn,
} from "./support/mock-api";

/**
 * TEC-183: the whole service wizard against a mocked BFF — customer and
 * vehicle, parts, VIN, stock, completion — ending on the service detail
 * page; then the list filters and the draft "continue in wizard" link.
 */

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("wizard: customer/vehicle → parts → VIN → stock → complete → detail", async ({
  page,
}) => {
  const api = await mockApi(page);

  await page.goto(`/t/${SLUG}/services/new`);
  await expect(
    page.getByRole("heading", { name: "New service" }),
  ).toBeVisible();

  // Step 1: customer and vehicle, then the draft.
  await page.getByLabel("Search by name or phone").fill("Ayşe");
  await page.getByTestId("customer-option").first().click();
  await expect(page.getByTestId("picked-customer")).toContainText(
    customer.name,
  );
  await page.getByTestId("vehicle-option").first().click();
  await page.locator("#service-km").fill("12000");
  await page.getByTestId("step1-continue").click();

  await expect(page).toHaveURL(
    new RegExp(`/t/${SLUG}/services/${SERVICE_UUID}/wizard$`),
  );
  expect(api.bodies["POST /v1/services"]?.[0]).toMatchObject({
    customer_uuid: customer.uuid,
    km: 12000,
    has_measurement: false,
  });

  // Step 2: parts on the SVG drawing.
  await expect(page.getByTestId("parts-step")).toBeVisible();
  await page.locator('path[data-part="body_kaput"]').click();
  await expect(page.getByTestId("parts-count")).toContainText("1");
  await page.getByTestId("parts-next").click();

  // Step 3: measurement needs the VIN.
  await expect(page.getByTestId("measurement-step")).toBeVisible();
  await page.getByTestId("measurement-yes").click();
  await page.locator('input[name="vin"]').fill("wvwzzz1jz3w386752");
  await page.getByTestId("measurement-save").click();
  await expect(page.getByTestId("stock-step")).toBeVisible();
  expect(api.bodies[`PATCH /v1/services/${SERVICE_UUID}`]?.at(-1)).toEqual({
    vin: "WVWZZZ1JZ3W386752",
    has_measurement: true,
  });

  // Step 4: 4.5 m from the roll, applied to the picked part.
  await page
    .locator('[data-unit="OLX-ROLL-001"]')
    .getByTestId("pick-unit")
    .click();
  await page.locator('input[name="meters"]').fill("4.5");
  await page.getByTestId("add-unit-submit").click();
  await expect(page.getByTestId("service-item")).toHaveCount(1);
  expect(
    api.bodies[`POST /v1/services/${SERVICE_UUID}/items`]?.[0],
  ).toMatchObject({
    barcode: "OLX-ROLL-001",
    kind: "partial",
    meters: 4.5,
    applied_parts: ["body_kaput"],
  });

  // Complete: the wizard lands on the detail page.
  await page.getByTestId("complete-service").click();
  await expect(page).toHaveURL(
    new RegExp(`/t/${SLUG}/services/${SERVICE_UUID}$`),
  );
  expect(
    api.bodies[`POST /v1/services/${SERVICE_UUID}/transitions`]?.[0],
  ).toEqual({
    status: "completed",
  });

  const detail = page.getByTestId("service-detail");
  await expect(detail).toBeVisible();
  await expect(page.getByRole("heading", { name: "DSE2E00001" })).toBeVisible();
  await expect(page.getByTestId("detail-status")).toHaveText("Completed");
  await expect(page.getByTestId("detail-vehicle")).toContainText(
    "BMW 320i 2022",
  );
  await expect(page.getByTestId("detail-vehicle")).toContainText(
    "WVWZZZ1JZ3W386752",
  );
  await expect(page.getByTestId("detail-customer")).toContainText(
    "Ayşe Yılmaz",
  );
  // TEC-378: items and warranties are nested DataTables (one row each).
  await expect(page.getByTestId("detail-item")).toHaveCount(1);
  const item = page
    .getByTestId("detail-items")
    .getByRole("row")
    .filter({ has: page.getByTestId("detail-item") });
  await expect(item).toContainText("Olex PPF 190");
  await expect(item).toContainText("OLX-ROLL-001");
  await expect(item).toContainText("4.50 m");
  await expect(item).toContainText("Hood");
  await expect(page.getByTestId("detail-warranty")).toContainText("WE2E0");
  await expect(page.getByTestId("detail-log")).toHaveCount(2);
  await expect(page.getByTestId("detail-log").first()).toHaveAttribute(
    "data-status",
    "completed",
  );
  await expect(page.getByTestId("detail-images-empty")).toBeVisible();
  await expect(page.getByTestId("continue-wizard")).toHaveCount(0);
  await expect(page.getByTestId("service-pdf")).toBeEnabled();
});

test("list: filters, then a draft continues in the wizard", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.seedDraft(5000);

  await page.goto(`/t/${SLUG}/services`);
  await expect(page.getByTestId("service-row")).toHaveCount(1);
  await expect(page.getByTestId("service-row")).toContainText("DSE2E00001");

  // TEC-378: a server DataTable; the default sort is sent, the status
  // facet and the toolbar search map to `status` / `q`.
  await expect
    .poll(() =>
      api.calls.filter((c) => c.startsWith("GET /v1/services?")).at(-1),
    )
    .toContain("sort=-created_at");
  await page.locator("button.border-dashed", { hasText: "Status" }).click();
  await page.getByRole("option", { name: "Draft" }).click();
  await expect
    .poll(() =>
      api.calls.filter((c) => c.startsWith("GET /v1/services?")).at(-1),
    )
    .toContain("status=draft");
  await page.keyboard.press("Escape");

  await page.getByPlaceholder("Search table…").fill("34ABC");
  await expect
    .poll(() =>
      api.calls.filter((c) => c.startsWith("GET /v1/services?")).at(-1),
    )
    .toContain("q=34ABC");

  await page.getByTestId("service-row").click();
  await expect(page).toHaveURL(
    new RegExp(`/t/${SLUG}/services/${SERVICE_UUID}$`),
  );
  await expect(page.getByTestId("detail-status")).toHaveText("Draft");
  await page.getByTestId("continue-wizard").click();
  await expect(page).toHaveURL(
    new RegExp(`/t/${SLUG}/services/${SERVICE_UUID}/wizard$`),
  );
  await expect(page.getByTestId("parts-step")).toBeVisible();
});
