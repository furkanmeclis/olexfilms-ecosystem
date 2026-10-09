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

/** 1×1 PNG served for the example and photo urls. */
const PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=",
  "base64",
);

test("wizard photos step: camera cards, 12 MB check, unlock, 422 marks the missing angle (TEC-500)", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.seedDraft(5000);
  api.features.push("photo_standard", "intake_contracts");
  api.permissions.push("contracts.read", "contracts.write");

  const svc = `/v1/services/${SERVICE_UUID}`;
  const taken = new Set<string>();
  const angle = (key: string, name: string, example: boolean) => ({
    uuid: `00000000-0000-4000-8000-0000000005${key === "front" ? "01" : "02"}`,
    key,
    name: { en: name, tr: name },
    hint: { en: `Shoot the ${name.toLowerCase()} from 3 m` },
    ...(example
      ? { example_url: `/v1/photo-standard/angles/${key}/example` }
      : {}),
    required: true,
    sort_order: key === "front" ? 10 : 20,
    active: true,
  });
  const angles = [angle("front", "Front", true), angle("rear", "Rear", false)];
  const photo = (key: string) => ({
    uuid: `00000000-0000-4000-8000-0000000006${key === "front" ? "01" : "02"}`,
    angle_key: key,
    url: `${svc}/intake-photos/${key}/file`,
    mime: "image/png",
    size: PNG.length,
    sha256: "0".repeat(64),
    created_at: "2026-10-09T08:00:00Z",
  });
  api.extra.push(async ({ method, path, ok, route }) => {
    if (method === "GET" && path === `${svc}/intake-photos`) {
      const rows = angles.map((a) => ({
        angle: a,
        required: true,
        missing: !taken.has(a.key),
        ...(taken.has(a.key) ? { photo: photo(a.key) } : {}),
      }));
      await ok({
        service_uuid: SERVICE_UUID,
        angles: rows,
        missing: rows.filter((r) => r.missing).map((r) => r.angle.key),
      });
      return true;
    }
    const upload = path.match(/^\/v1\/services\/[^/]+\/intake-photos\/(\w+)$/);
    if (method === "POST" && upload) {
      taken.add(upload[1]);
      await ok(photo(upload[1]), 201);
      return true;
    }
    if (
      method === "GET" &&
      (path.endsWith("/example") || path.endsWith("/file"))
    ) {
      await route.fulfill({ status: 200, contentType: "image/png", body: PNG });
      return true;
    }
    if (method === "POST" && path === `${svc}/contract`) {
      // The rear photo went missing on the server meanwhile.
      taken.delete("rear");
      await route.fulfill({
        status: 422,
        json: {
          success: false,
          error: {
            code: "PHOTO_STANDARD_INCOMPLETE",
            message:
              "Every required intake photo angle must be photographed first",
            details: [
              {
                field: "intake_photos.rear",
                code: "missing",
                message: "required intake photo is missing",
              },
            ],
          },
          data: { missing_angles: ["rear"] },
        },
      });
      return true;
    }
    return false;
  });

  await page.goto(`/t/${SLUG}/services/${SERVICE_UUID}/wizard`);
  const step = page.getByTestId("photos-step");
  await expect(step).toBeVisible();
  const front = page.locator(
    '[data-testid="intake-angle"][data-angle="front"]',
  );
  const rear = page.locator('[data-testid="intake-angle"][data-angle="rear"]');
  await expect(front.getByTestId("intake-example")).toBeVisible();
  await expect(front).toContainText("Shoot the front from 3 m");
  await expect(front.getByTestId("intake-input")).toHaveAttribute(
    "capture",
    "environment",
  );
  await expect(page.getByTestId("intake-counter")).toBeVisible();
  await expect(page.getByTestId("photos-next")).toBeDisabled();
  await expect(page.locator('[data-step="contract"]')).toBeDisabled();
  await expect(page.locator('[data-step="contract"]')).toHaveAttribute(
    "data-blocked",
    "photos",
  );

  // Over 12 MB: refused in the browser, nothing is sent.
  await front.getByTestId("intake-input").setInputFiles({
    name: "big.jpg",
    mimeType: "image/jpeg",
    buffer: Buffer.alloc(12 * 1024 * 1024 + 1),
  });
  await expect(front.getByTestId("intake-error")).toBeVisible();
  expect(
    api.calls.filter(
      (c) => c.startsWith("POST") && c.includes("intake-photos"),
    ),
  ).toEqual([]);

  for (const card of [front, rear]) {
    await card.getByTestId("intake-input").setInputFiles({
      name: "shot.png",
      mimeType: "image/png",
      buffer: PNG,
    });
    await expect(card.getByTestId("intake-done")).toBeVisible();
  }
  await expect(page.getByTestId("intake-complete")).toBeVisible();
  await expect(page.getByTestId("photos-next")).toBeEnabled();
  expect(
    api.calls.filter((c) => c.startsWith(`POST ${svc}/intake-photos/`)),
  ).toEqual([
    `POST ${svc}/intake-photos/front`,
    `POST ${svc}/intake-photos/rear`,
  ]);

  // The contract is refused (422): the photos come back, rear marked.
  await page.locator('[data-step="contract"]').click();
  await page.getByTestId("contract-create").click();
  await expect(step).toBeVisible();
  await expect(rear).toHaveAttribute("data-missing", "true");
  await expect(rear.getByTestId("intake-missing")).toBeVisible();
  await expect(front).toHaveAttribute("data-missing", "false");
  await expect(page.getByTestId("photos-next")).toBeDisabled();
});
