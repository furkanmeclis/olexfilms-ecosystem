import { expect, test } from "@playwright/test";

import {
  F5,
  asCenter,
  installF5,
  installStepUp,
  mockEinvoice,
  mockFleet,
  mockIntakePhotos,
  mockPerformance,
  mockRecommendedPrices,
  mockShowcase,
  mockStockForecast,
} from "./support/f5-mock";
import { ORG, SERVICE_UUID, SLUG, signIn } from "./support/mock-api";

/**
 * F5 gate (TEC-516, F5-10e): the add-on switch chain (platform admin →
 * distributor → dealer), the module request flow, every add-on's main
 * screen and the Özellikler page, against the mocked BFF of
 * e2e/support/f5-mock.ts. Modules are switched on only in these mocks.
 */

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("F5 gate: platform admin closes performance system wide → the distributor has no control for it", async ({
  page,
}) => {
  const world = await installF5(page);
  world.as("platform");

  await page.goto("/platform/modules");
  const row = page.getByRole("row", { name: /Performance and targets/ });
  const system = row.getByRole("switch", { name: "Available system wide" });
  await expect(system).toBeChecked();
  await system.click();
  await expect(system).not.toBeChecked();
  expect(world.api.bodies["PATCH /v1/platform/modules/performance"]).toEqual([
    { enabled: false },
  ]);

  world.as("distributor");
  await page.goto("/t/dist/features");
  const addon = page.getByTestId("module-level-addon");
  await expect(addon.getByTestId("module-fleet")).toBeVisible();
  // Closed system wide: GET /v1/features leaves it out, so there is no
  // row, no "Request" and no dealer standard switch for it.
  await expect(page.getByTestId("module-performance")).toHaveCount(0);
  await expect(page.getByTestId("module-request-performance")).toHaveCount(0);
  await page.getByRole("tab", { name: "Dealer standard" }).click();
  await expect(
    page.getByRole("switch", { name: "Fleet customers" }),
  ).toBeVisible();
  await expect(
    page.getByRole("switch", { name: "Performance and targets" }),
  ).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Performance" })).toHaveCount(0);
});

test("F5 gate: distributor turns stock_forecast on in the dealer standard → a new dealer has it on", async ({
  page,
}) => {
  const world = await installF5(page);
  // The center sold the add-on to the distributor.
  world.adminSet(F5.distributor, "stock_forecast", true);

  world.as("distributor");
  await page.goto("/t/dist/features");
  await page.getByRole("tab", { name: "Dealer standard" }).click();
  const toggle = page.getByRole("switch", {
    name: "Stock forecast and order suggestions",
  });
  await expect(toggle).not.toBeChecked();
  await toggle.click();
  await expect(toggle).toBeChecked();
  expect(
    world.api.bodies["PUT /v1/tenant/modules/dealer-standard/stock_forecast"],
  ).toEqual([{ enabled: true }]);

  world.as("newDealer");
  await page.goto("/t/yeni/features");
  const row = page.getByRole("row", {
    name: /Stock forecast and order suggestions/,
  });
  await expect(row.getByTestId("module-source")).toHaveText("Dealer standard");
  await expect(row).toContainText("Deniz Dağıtım");
  await expect(page.getByTestId("module-request-stock_forecast")).toHaveCount(
    0,
  );
  await expect(
    page.getByRole("link", { name: "Forecast and suggestions" }),
  ).toBeVisible();
});

test("F5 gate: dealer requests certificates, the distributor approves, the menu shows it", async ({
  page,
}) => {
  const world = await installF5(page);
  world.adminSet(F5.distributor, "certificates", true);

  await page.goto(`/t/${SLUG}/features`);
  await expect(page.getByTestId("module-level-addon")).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Staff certificates" }),
  ).toHaveCount(0);
  await page.getByTestId("module-request-certificates").click();
  await page
    .getByTestId("module-request-note")
    .fill("12 personelimiz sertifikalı");
  await page.getByTestId("module-request-submit").click();
  const row = page.getByRole("row", { name: /Certificates/ });
  await expect(row.getByTestId("module-request-pending")).toBeVisible();
  expect(world.api.bodies["POST /v1/features/certificates/request"]).toEqual([
    { note: "12 personelimiz sertifikalı" },
  ]);

  world.as("distributor");
  await page.goto("/t/dist/features");
  const tab = page.getByTestId("modules-requests-tab");
  await expect(tab).toContainText("1");
  await tab.click();
  const request = page.getByRole("row", { name: /Acme Bayi/ });
  await expect(request).toContainText("12 personelimiz sertifikalı");
  await request.getByRole("button", { name: "Approve" }).click();
  await page.getByTestId("module-request-submit").click();
  await expect(page.getByRole("row", { name: /Acme Bayi/ })).toHaveCount(0);
  expect(
    world.api.bodies[`POST /v1/tenant/modules/requests/${F5.request}/approve`],
  ).toHaveLength(1);

  world.as("dealer");
  await page.goto(`/t/${SLUG}/features`);
  await expect(
    page
      .getByRole("row", { name: /Certificates/ })
      .getByTestId("module-source"),
  ).toHaveText("Distributor");
  await expect(
    page.getByRole("link", { name: "Staff certificates" }),
  ).toBeVisible();
});

test("F5 gate: admin opens fleet for one dealer → fleet list, card and bulk plan preview", async ({
  page,
}) => {
  const world = await installF5(page);
  const previews = mockFleet(world.api);

  // The distributor has no fleet add-on: the dealer does not see it.
  await page.goto(`/t/${SLUG}/features`);
  await expect(page.getByTestId("module-level-core")).toBeVisible();
  await expect(page.getByTestId("module-fleet")).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Fleet list" })).toHaveCount(0);

  // Admin value for this dealer only (independent of the level above).
  world.adminSet(ORG, "fleet", true);
  await page.reload();
  const row = page.getByRole("row", { name: /Fleet customers/ });
  await expect(row.getByTestId("module-source")).toHaveText("Platform admin");

  await page.getByRole("link", { name: "Fleet list" }).click();
  await page.waitForURL(new RegExp(`/t/${SLUG}/fleets$`));
  await expect(page.getByTestId("fleet-row")).toContainText(
    "Ege Kiralama Filo",
  );
  await page.getByTestId("fleet-row").click();
  await page.waitForURL(new RegExp(`/fleets/${F5.fleet}$`));
  await expect(page.getByTestId("fleet-summary")).toBeVisible();
  await page.getByTestId("fleet-tab-vehicles").click();
  await page.getByTestId("fleet-plan-new").click();
  await page.waitForURL(new RegExp(`/fleets/${F5.fleet}/plans/new`));

  await page.locator("#plan-select-all").click();
  await expect(page.getByTestId("plan-selected-count")).toHaveText(
    "Selected: 2",
  );
  await page.getByTestId("plan-next-settings").click();
  await page.getByTestId("plan-service-type").fill("PPF");
  await page.getByTestId("plan-preview").click();
  await expect(page.getByTestId("plan-row")).toHaveCount(2);
  await expect(page.getByTestId("plan-confirm")).toBeEnabled();
  expect(previews).toHaveLength(1);
  expect(previews[0]).toMatchObject({
    vehicle_uuids: [...F5.vehicles],
    service_type: "PPF",
  });
});

test("F5 gate: module bundle subscription ends → efficiency off, menu hidden, URL shows the off screen", async ({
  page,
}) => {
  const world = await installF5(page);
  world.adminSet(F5.distributor, "efficiency", true);
  world.subscribe(ORG, "efficiency");

  await page.goto(`/t/${SLUG}/features`);
  const row = page.getByRole("row", { name: /Efficiency and waste analysis/ });
  await expect(row.getByTestId("module-via-subscription")).toHaveText(
    "On by subscription",
  );
  const menu = page.getByRole("link", { name: "Efficiency", exact: true });
  await expect(menu).toBeVisible();

  world.expire(ORG, "efficiency");
  await page.reload();
  await expect(row.getByTestId("module-via-subscription")).toHaveCount(0);
  await expect(page.getByTestId("module-request-efficiency")).toBeVisible();
  await expect(menu).toHaveCount(0);

  await page.goto(`/t/${SLUG}/efficiency`);
  const off = page.getByTestId("feature-disabled");
  await expect(off).toContainText("This module is off");
  await expect(off).toContainText("Efficiency and waste analysis");
  expect(world.api.calls.filter((c) => c.includes("/v1/efficiency"))).toEqual(
    [],
  );
});

test("F5 gate tour: showcase is published → public dealer page takes a quote", async ({
  page,
}) => {
  const world = await installF5(page);
  world.adminSet(ORG, "dealer_showcase", true);
  world.api.permissions.push("showcase.read", "showcase.write");
  const showcase = mockShowcase(world.api);

  await page.goto(`/t/${SLUG}/showcase`);
  await expect(page.getByText("Draft").first()).toBeVisible();
  await page.getByRole("button", { name: "Publish" }).click();
  await expect(page.getByText("Showcase published.")).toBeVisible();
  expect(
    world.api.calls.some((c) => c.startsWith("POST /v1/showcase/submit")),
  ).toBe(true);

  await page.goto(`/bayi/${F5.dealerCode}?lang=en`);
  await expect(
    page.getByRole("heading", { level: 1, name: "Olex Kadıköy" }),
  ).toBeVisible();
  const form = page.locator('[data-slot="showcase-lead-form"]');
  await expect(form).toBeVisible();
  await form.locator('input[name="name"]').fill("Ayşe Yılmaz");
  await form.locator('input[name="phone"]').fill("0532 123 45 67");
  await form.getByRole("checkbox", { name: "KVKK" }).click();
  await form.getByRole("button", { name: "Send request" }).click();
  await expect(
    page.locator('[data-screen="showcase-lead-success"]'),
  ).toContainText("Request received");
  expect(showcase.leads).toHaveLength(1);
  expect(showcase.leads[0]).toMatchObject({
    name: "Ayşe Yılmaz",
    phone: "+905321234567",
    kvkk_consent: true,
  });
});

test("F5 gate tour: stock forecast list → bulk order draft", async ({
  page,
}) => {
  const world = await installF5(page);
  world.adminSet(ORG, "stock_forecast", true);
  world.api.permissions.push("stock_forecast.manage");
  const drafts = mockStockForecast(world.api);

  await page.goto(`/t/${SLUG}/stock-forecast`);
  await expect(page.getByTestId("stock-forecast-row").first()).toBeVisible();
  const critical = page.getByRole("row", { name: /Olex PPF 190/ });
  await critical.getByRole("checkbox", { name: "Select row" }).click();
  await page.getByRole("button", { name: "Bulk" }).click();
  await page.getByRole("button", { name: "Create draft" }).click();
  await expect(page.getByTestId("stock-forecast-draft-link")).toHaveText(
    "ORD-2026-0516",
  );
  expect(drafts).toEqual([
    { lines: [{ product_uuid: F5.product, quantity: 10, meters: null }] },
  ]);
});

test("F5 gate tour: performance ranking and region map", async ({ page }) => {
  const world = await installF5(page);
  world.adminSet(ORG, "performance", true);
  asCenter(world.api);
  await mockPerformance(page, world.api);

  await page.goto(`/t/${SLUG}/performance`);
  await page.getByTestId("performance-tab-ranking").click();
  const ranking = page.getByTestId("performance-ranking");
  await expect(ranking).toContainText("Kadıköy Bayi");
  await expect(ranking).toContainText("Bursa Bayi");

  await page.getByTestId("performance-tab-map").click();
  const map = page.getByTestId("performance-leaflet");
  await expect(map).toHaveClass(/leaflet-container/);
  await expect(map.locator("path.leaflet-interactive")).toHaveCount(1);
  await expect(page.getByTestId("performance-region-map")).toContainText(
    "Ankara",
  );
});

test("F5 gate tour: photo step warns about a missing angle", async ({
  page,
}) => {
  const world = await installF5(page);
  world.adminSet(ORG, "photo_standard", true);
  world.api.permissions.push("contracts.read", "contracts.write");
  world.api.seedDraft(5000);
  mockIntakePhotos(world.api);

  await page.goto(`/t/${SLUG}/services/${SERVICE_UUID}/wizard`);
  await expect(page.getByTestId("photos-step")).toBeVisible();
  await expect(page.getByTestId("intake-counter")).toContainText(
    "1 required angle",
  );
  await expect(page.getByTestId("photos-next")).toBeDisabled();

  const rear = page.locator('[data-testid="intake-angle"][data-angle="rear"]');
  await rear.getByTestId("intake-input").setInputFiles({
    name: "rear.png",
    mimeType: "image/png",
    buffer: Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=",
      "base64",
    ),
  });
  await expect(page.getByTestId("intake-complete")).toBeVisible();

  // The server lost the rear photo meanwhile: the contract is refused.
  await page.locator('[data-step="contract"]').click();
  await page.getByTestId("contract-create").click();
  await expect(rear).toHaveAttribute("data-missing", "true");
  await expect(rear.getByTestId("intake-missing")).toBeVisible();
  await expect(page.getByTestId("photos-next")).toBeDisabled();
});

test("F5 gate tour: e-invoice draft → archive with step-up → XML download", async ({
  page,
}) => {
  const world = await installF5(page);
  world.adminSet(ORG, "e_invoice", true);
  asCenter(world.api);
  world.api.permissions.push(
    "accounting.read",
    "einvoice.read",
    "einvoice.manage",
  );
  const einvoice = mockEinvoice(world.api);
  const stepUp = await installStepUp(page, world.api);

  await page.goto(`/t/${SLUG}/einvoices/billable`);
  await page.getByRole("button", { name: "Create draft" }).click();
  await page.waitForURL(new RegExp(`/einvoices/${F5.invoice}$`));
  expect(einvoice.created).toEqual([
    { source_type: "order", source_uuid: F5.source },
  ]);

  await page.getByTestId("einvoice-archive").click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Archive" })
    .click();
  await expect(page.getByText("Confirm your identity")).toBeVisible();
  await page.getByRole("textbox", { name: "Password" }).fill("e2e-password-1");
  await page.getByRole("button", { name: "Verify" }).click();
  expect(stepUp.passwords).toEqual(["e2e-password-1"]);

  const xml = page.getByTestId("einvoice-download-xml");
  await expect(xml).toBeVisible();
  expect(world.api.calls).toContain(`POST /v1/einvoices/${F5.invoice}/archive`);
  const download = page.waitForEvent("download");
  await xml.click();
  expect((await download).suggestedFilename()).toBe("EAR2026000000043.xml");
});

test("F5 gate tour: recommended price is published → dealer price screen shows the deviation", async ({
  page,
}) => {
  const world = await installF5(page);
  asCenter(world.api);
  world.api.permissions.push(
    "pricing.recommended.read",
    "pricing.recommended.write",
  );
  const prices = mockRecommendedPrices(world.api);
  await installStepUp(page, world.api);

  await page.goto(`/t/${SLUG}/catalog/recommended-prices`);
  await page.getByText("Double-click to set").dblclick();
  const editor = page.locator("input:focus");
  await editor.fill("1100");
  await editor.press("Enter");
  await page.getByTestId("open-publish").click();
  await page
    .getByTestId("publish-dialog")
    .getByTestId("publish-submit")
    .click();
  await page.getByRole("textbox", { name: "Password" }).fill("e2e-password-1");
  await page.getByRole("button", { name: "Verify" }).click();
  await expect.poll(() => prices.published.length).toBe(1);
  expect(prices.published[0]).toMatchObject({
    rows: [{ product_uuid: F5.product, currency: "TRY", price: "1100.00" }],
  });

  // The dealer (Acme Bayi) sells at 900: 18.18% under the new price.
  world.as("dealer");
  world.api.permissions.push(
    "dealer_pricing.write",
    "pricing.recommended.read",
  );
  await page.goto(`/t/${SLUG}/dealer-sales/prices`);
  const badge = page.getByTestId("deviation-badge");
  await expect(badge).toHaveText("-18.18%");
  await expect(badge).toHaveAttribute("data-over-threshold", "true");
});

test("F5 gate: Özellikler shows level counts and price / free lines", async ({
  page,
}) => {
  const world = await installF5(page);
  // The distributor lost fleet, the admin keeps it off for this dealer:
  // visible (admin override), but "off at the level above", no request.
  world.adminSet(ORG, "fleet", false);
  world.adminSet(F5.distributor, "efficiency", true);
  world.adminSet(F5.distributor, "certificates", true);
  world.adminSet(F5.distributor, "photo_standard", true);

  await page.goto(`/t/${SLUG}/features`);
  const level = (key: string) =>
    page.getByTestId(`module-level-${key}`).getByRole("heading");
  await expect(level("core")).toHaveText(/^Core\s*14$/);
  await expect(level("standard")).toHaveText(/^Standard\s*10$/);
  // Add-ons off at the distributor are left out (13 in the catalog): fleet
  // (admin override), efficiency, certificates and photo_standard stay.
  await expect(level("addon")).toHaveText(/^Add-on\s*4$/);

  await expect(page.getByTestId("module-price-leads")).toHaveText(
    "Free — on by default, no service record needed",
  );
  await expect(page.getByTestId("module-price-fleet")).toContainText("Paid: ");
  await expect(page.getByTestId("module-price-fleet")).toContainText("/month");
  await expect(page.getByTestId("module-price-efficiency")).toHaveText(
    "Contact us for the price",
  );
  await expect(page.getByTestId("module-price-photo_standard")).toHaveText(
    "Free",
  );
  const fleet = page.getByRole("row", { name: /Fleet customers/ });
  await expect(fleet.getByTestId("module-upstream-closed")).toHaveText(
    "Off at the level above",
  );
  await expect(page.getByTestId("module-request-fleet")).toHaveCount(0);
  await expect(page.getByTestId("module-request-efficiency")).toBeVisible();
});
