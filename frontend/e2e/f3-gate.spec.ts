import { expect, test, type Locator, type Page } from "@playwright/test";

import { E2E_PORTAL } from "./support/constants";
import {
  CONTRACT_NO,
  CONTRACT_OTP,
  CONTRACT_UUID,
  MEASUREMENTS,
  VIN,
  installF3,
} from "./support/f3-mock";
import {
  SERVICE_UUID,
  SLUG,
  customer,
  mockApi,
  signIn,
} from "./support/mock-api";
import { PortalMock, routePortal } from "./support/portal-mock";

/**
 * TEC-304 (F3-01k): the F3 gate scenario against a mocked BFF. The dealer
 * picks a customer and a vehicle with a VIN, the parts, the "before"
 * measurement, then the required intake contract keeps "Next" closed until
 * the customer (WhatsApp OTP + canvas) and the staff sign; the stock step
 * completes the service and its detail shows the contract PDF and the
 * measurement section (before/after, part diff, "check required" band).
 * The customer then finds the contract in the portal's "My contracts".
 * Only this spec turns the measurements and intake_contracts modules and
 * contracts.intake_required on (installF3).
 */

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

/** One stroke across the signature canvas inside `slot`. */
async function drawSignature(page: Page, slot: Locator) {
  const canvas = slot.getByTestId("signature-canvas");
  await canvas.scrollIntoViewIfNeeded();
  const box = await canvas.boundingBox();
  if (!box) throw new Error("signature canvas not visible");
  const y = box.y + box.height / 2;
  await page.mouse.move(box.x + 20, y);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 3, y - 20, { steps: 5 });
  await page.mouse.move(box.x + (box.width * 2) / 3, y + 20, { steps: 5 });
  await page.mouse.up();
  await expect(canvas).toHaveAttribute("data-empty", "false");
}

test("F3 gate: VIN vehicle → parts → before measurement → required contract → stock → detail → portal", async ({
  page,
  browser,
  baseURL,
}) => {
  const api = await mockApi(page);
  const f3 = installF3(api);
  const svc = `/v1/services/${SERVICE_UUID}`;

  await page.goto(`/t/${SLUG}/services/new`);
  await expect(
    page.getByRole("heading", { name: "New service" }),
  ).toBeVisible();

  // 1. Customer and the vehicle with a VIN; the draft snapshots the VIN.
  await page.getByLabel("Search by name or phone").fill("Ayşe");
  await page.getByTestId("customer-option").first().click();
  await expect(page.getByTestId("picked-customer")).toContainText(
    customer.name,
  );
  await page.getByTestId("vehicle-option").first().click();
  await page.locator("#service-km").fill("8000");
  await page.getByTestId("step1-continue").click();
  await expect(page).toHaveURL(
    new RegExp(`/t/${SLUG}/services/${SERVICE_UUID}/wizard$`),
  );
  // The module adds the contract step between measurement and stock.
  await expect(
    page.getByTestId("wizard-stepper").locator("button[data-step]"),
  ).toHaveCount(5);

  // Parts.
  await expect(page.getByTestId("parts-step")).toBeVisible();
  await page.locator('path[data-part="body_kaput"]').click();
  await page.locator('path[data-part="body_on_tampon"]').click();
  await expect(page.getByTestId("parts-count")).toContainText("2");
  await page.getByTestId("parts-next").click();

  // 2. Measurement: "yes" lists the VIN's reports; pick the suggested
  // "before" one.
  await expect(page.getByTestId("measurement-step")).toBeVisible();
  await expect(page.locator('input[name="vin"]')).toHaveValue(VIN);
  await page.getByTestId("measurement-yes").click();
  const reports = page.getByTestId("measurement-reports");
  await expect(reports.getByTestId("measurement-option")).toHaveCount(2);
  await expect(
    reports.getByTestId("measurement-reports-warning"),
  ).toBeVisible();
  const before = reports.locator(
    `[data-testid="measurement-option"][data-uuid="${MEASUREMENTS.before}"]`,
  );
  await expect(before).toContainText("Suggested");
  await before.click();
  await expect(before).toHaveAttribute("aria-checked", "true");
  await expect(reports.getByTestId("measurement-reports-warning")).toHaveCount(
    0,
  );
  await page.getByTestId("measurement-save").click();
  expect(api.bodies[`PATCH ${svc}`]?.at(-1)).toEqual({
    vin: VIN,
    has_measurement: true,
  });

  // 3. Contract: required, so "Next" and the stock step stay closed.
  const step = page.getByTestId("contract-step");
  await expect(step).toBeVisible();
  expect(api.bodies[`POST ${svc}/measurements`]?.[0]).toEqual({
    measurement_uuid: MEASUREMENTS.before,
    phase: "before",
  });
  await expect(step.getByTestId("contract-required")).toBeVisible();
  await expect(step.getByTestId("contract-next")).toBeDisabled();
  await expect(
    page.getByTestId("wizard-stepper").locator('button[data-step="stock"]'),
  ).toBeDisabled();

  await step.getByTestId("contract-create").click();
  await expect(step.getByTestId("contract-no")).toContainText(
    `#${CONTRACT_NO}`,
  );
  await expect(step.getByTestId("contract-status")).toHaveText(
    "Awaiting signatures",
  );
  await expect(step.getByTestId("contract-next")).toBeDisabled();

  // Customer: KVKK notice, WhatsApp OTP, the code and a canvas signature.
  const customerSlot = step.getByTestId("customer-sign");
  await expect(customerSlot.getByTestId("kvkk-notice")).toBeVisible();
  await expect(customerSlot.getByTestId("customer-sign-submit")).toBeDisabled();
  await customerSlot.getByTestId("otp-send").click();
  await expect(customerSlot.getByTestId("otp-countdown")).toBeVisible();
  const code = f3.whatsapp.findLast(
    (m) => m.to === E2E_PORTAL.owner.phone,
  )?.code;
  expect(code).toBe(CONTRACT_OTP);
  await customerSlot.locator('input[name="code"]').fill(code ?? "");
  await expect(customerSlot.getByTestId("customer-sign-submit")).toBeDisabled();
  await drawSignature(page, customerSlot.getByTestId("customer-canvas"));
  await customerSlot.getByTestId("customer-sign-submit").click();
  await expect(step.getByTestId("customer-signed")).toBeVisible();
  const customerSign = api.bodies[
    `POST /v1/contracts/${CONTRACT_UUID}/signers/customer/sign`
  ]?.[0] as { code: string; signature_png: string };
  expect(customerSign.code).toBe(CONTRACT_OTP);
  // Raw base64 PNG, no data URL prefix.
  expect(customerSign.signature_png).toMatch(/^iVBORw0KGgo/);
  await expect(step.getByTestId("contract-next")).toBeDisabled();

  // Staff signature executes the contract and opens "Next".
  const staffSlot = step.getByTestId("staff-sign");
  await drawSignature(page, staffSlot.getByTestId("staff-canvas"));
  await staffSlot.getByTestId("staff-sign-submit").click();
  await expect(step.getByTestId("staff-signed")).toBeVisible();
  await expect(step.getByTestId("contract-status")).toHaveText("Signed");
  await expect(step.getByTestId("contract-executed")).toBeVisible();
  await expect(step.getByTestId("contract-next")).toBeEnabled();
  await step.getByTestId("contract-next").click();

  // 4. Stock: 6 m from the roll on the two parts, then complete.
  await expect(page.getByTestId("stock-step")).toBeVisible();
  await page
    .locator('[data-unit="OLX-ROLL-001"]')
    .getByTestId("pick-unit")
    .click();
  await page.locator('input[name="meters"]').fill("6");
  await page.getByTestId("add-unit-submit").click();
  await expect(page.getByTestId("service-item")).toHaveCount(1);
  expect(api.bodies[`POST ${svc}/items`]?.[0]).toMatchObject({
    barcode: "OLX-ROLL-001",
    kind: "partial",
    meters: 6,
    applied_parts: ["body_kaput", "body_on_tampon"],
  });
  await page.getByTestId("complete-service").click();
  await expect(page).toHaveURL(
    new RegExp(`/t/${SLUG}/services/${SERVICE_UUID}$`),
  );
  expect(api.bodies[`POST ${svc}/transitions`]?.[0]).toEqual({
    status: "completed",
  });

  // 5. Detail: the contract card with its PDF.
  await expect(page.getByTestId("detail-status")).toHaveText("Completed");
  const card = page.getByTestId("service-contract");
  await expect(card.getByTestId("service-contract-no")).toHaveText(
    `#${CONTRACT_NO}`,
  );
  await expect(card.getByTestId("service-contract-status")).toHaveText(
    "Signed",
  );
  const pdf = card.getByTestId("service-contract-pdf");
  await expect(pdf).toBeEnabled();
  const download = page.waitForEvent("download");
  await pdf.click();
  expect((await download).suggestedFilename()).toBe(
    `contract-${CONTRACT_NO}.pdf`,
  );

  // Measurement section: the manual "before" is confirmed, the "after"
  // auto match of the completion waits for the dealer.
  const section = page.getByTestId("detail-measurements");
  const beforeCard = section.getByTestId("measurement-phase-before");
  const afterCard = section.getByTestId("measurement-phase-after");
  await expect(beforeCard).toContainText("Selected manually");
  await expect(beforeCard).toContainText("Confirmed");
  await expect(beforeCard).toContainText("NX-304-A");
  await expect(afterCard).toContainText("Automatic match");
  await expect(afterCard.getByTestId("measurement-pending")).toBeVisible();
  await afterCard.getByTestId("measurement-confirm-after").click();
  await expect(afterCard.getByTestId("measurement-pending")).toHaveCount(0);
  await expect(afterCard).toContainText("Confirmed");
  expect(api.bodies[`POST ${svc}/measurements`]?.at(-1)).toEqual({
    measurement_uuid: MEASUREMENTS.after,
    phase: "after",
  });

  // The diff table: hood within tolerance, roof deviates → check band.
  const diff = section.getByTestId("measurement-diff");
  await expect(diff.getByTestId("diff-row")).toHaveCount(2);
  await expect(diff).toContainText("Tolerance: ±20 µm");
  const roof = diff.getByRole("row").filter({
    has: page.locator('[data-testid="diff-row"][data-part="ROOF"]'),
  });
  await expect(roof.getByTestId("diff-deviation")).toHaveText("Deviates");
  await expect(roof).toContainText("105");
  await expect(roof).toContainText("120");
  await expect(diff.getByTestId("diff-deviation")).toHaveCount(1);
  const band = section.getByTestId("measurement-check-band");
  await expect(band).toBeVisible();
  await band.getByTestId("measurement-check-open").click();
  await page
    .getByTestId("measurement-check-note")
    .fill("Roof re-measured, film is fine.");
  await page.getByTestId("measurement-check-save").click();
  await expect(band).toHaveCount(0);
  await expect(section.getByTestId("measurement-checked")).toBeVisible();
  expect(api.bodies[`POST ${svc}/measurements/checked`]?.[0]).toEqual({
    note: "Roof re-measured, film is fine.",
  });
  // Every F3 call is mocked (the realtime token is not under test).
  expect(api.unknown.filter((c) => !c.includes("/v1/realtime/"))).toEqual([]);

  // Portal: the customer finds the executed contract in "My contracts".
  const portal = new PortalMock();
  portal.contracts = [f3.portalContract()];
  const { owner } = E2E_PORTAL;
  const portalContext = await browser.newContext({
    baseURL,
    locale: "en-US",
    timezoneId: "Europe/Istanbul",
  });
  try {
    await routePortal(portalContext, portal, owner);
    const p = await portalContext.newPage();
    await p.goto("/portal/login");
    await p.locator("#portal-phone").fill(owner.typed);
    await p.getByRole("button", { name: "Send code via WhatsApp" }).click();
    await p
      .locator("#portal-otp")
      .fill(portal.lastCode(owner.phone, "customer_login") ?? "");
    await p.getByRole("button", { name: "Sign in" }).click();
    await expect(p).toHaveURL(/\/portal$/);
    await p
      .getByTestId("portal-nav")
      .getByRole("link", { name: "My contracts" })
      .click();
    await expect(p).toHaveURL(/\/portal\/contracts$/);
    const contracts = p.getByTestId("portal-contracts");
    await expect(contracts.getByTestId("portal-contract-no")).toHaveText(
      `#${CONTRACT_NO}`,
    );
    await expect(contracts).toContainText("Acme Bayi");
    await expect(contracts).toContainText("34ABC123");
    await expect(contracts.getByTestId("portal-contract-service")).toHaveText(
      "DSE2E00001",
    );
    await expect(contracts.getByTestId("portal-contract-pdf")).toHaveAttribute(
      "href",
      new RegExp(`/portal/contracts/${CONTRACT_UUID}/pdf$`),
    );
    expect(portal.unknown).toEqual([]);
  } finally {
    await portalContext.close();
  }
});
