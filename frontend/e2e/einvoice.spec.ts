import { expect, test } from "@playwright/test";

import { mockApi, signIn, SLUG, type MockApi } from "./support/mock-api";

/**
 * TEC-504 (F5-08d): e-invoice screens against a mocked BFF — the menu under
 * Muhasebe, the invoice list → detail with the sandboxed preview and the
 * "nothing is sent, only archived" confirmation before the step-up, an
 * invalid invoice with Arşivle disabled, the billable records with the
 * bulk "Create draft" and the settings form's 3-character series check.
 */

const INV = "0b9c4c1e-0000-4000-8000-000000000504";
const BAD = "0b9c4c1e-0000-4000-8000-000000000505";
const SRC = "0b9c4c1e-0000-4000-8000-000000000506";
const page1 = <T>(items: T[]) => ({
  items,
  total: items.length,
  limit: 20,
  offset: 0,
});

function invoice(over: Record<string, unknown> = {}) {
  return {
    uuid: INV,
    number: null,
    profile: "EARSIVFATURA",
    invoice_type: "SATIS",
    status: "draft",
    validation_status: "valid",
    validation_messages: [],
    source_type: "order",
    source_uuid: SRC,
    buyer_organization: { uuid: "d1", name: "Ege Distribütör" },
    buyer: {
      name: "Ege Distribütör",
      vkn: "1234567890",
      einvoice_registered: false,
    },
    seller: { name: "Olex Merkez", einvoice_registered: true },
    lines: [],
    tax_breakdown: [],
    currency: "TRY",
    line_extension: "300.00",
    tax_exclusive: "300.00",
    tax_total: "60.00",
    payable: "360.00",
    issue_date: "2026-10-09",
    xml_sha256: null,
    has_xml: false,
    has_pdf: false,
    error: null,
    voided_at: null,
    void_reason: null,
    created_at: "2026-10-09T08:00:00Z",
    updated_at: "2026-10-09T08:00:00Z",
    ...over,
  };
}

const settings = {
  configured: true,
  vkn: "9000068418",
  tax_office: "Beşiktaş",
  legal_name: "Olexfilms Merkez A.Ş.",
  address: "Papatya Cad. No:21",
  city: "İstanbul",
  district: "Beşiktaş",
  country: "TR",
  iban: null,
  email: null,
  phone: null,
  website: null,
  trade_registry_no: null,
  mersis_no: null,
  default_note: null,
  earchive_series: "EAR",
  efatura_series: "EFN",
  pdf_enabled: true,
  custom_xslt: false,
  xslt_sha1: null,
  updated_at: "2026-10-09T08:00:00Z",
  counters: [{ series: "EAR", year: 2026, last_no: 42, updated_at: null }],
};

function asCenter(api: MockApi) {
  api.memberships = api.memberships.map((m) => ({
    ...m,
    type: "center",
    name: "Olex Merkez",
  }));
  api.permissions.push(
    "accounting.read",
    "einvoice.read",
    "einvoice.manage",
    "einvoice.settings",
  );
  api.features.push("accounting", "e_invoice");
  api.extra.push(async ({ method, path, ok, route }) => {
    if (method === "GET" && path === "/v1/einvoices") {
      await ok(
        page1([
          invoice(),
          invoice({
            uuid: BAD,
            status: "failed",
            validation_status: "invalid",
          }),
        ]),
      );
      return true;
    }
    if (method === "GET" && path === `/v1/einvoices/${INV}`) {
      await ok(invoice());
      return true;
    }
    if (method === "GET" && path === `/v1/einvoices/${BAD}`) {
      await ok(
        invoice({
          uuid: BAD,
          status: "failed",
          validation_status: "invalid",
          validation_messages: ["[BR-TR-01] Alıcı VKN zorunludur"],
          error: "schematron: 1 error",
        }),
      );
      return true;
    }
    if (method === "GET" && path.endsWith("/preview")) {
      await route.fulfill({
        status: 200,
        contentType: "text/html; charset=utf-8",
        body: "<html><body><h1>PREVIEW e-Arşiv</h1></body></html>",
      });
      return true;
    }
    if (method === "GET" && path === "/v1/einvoices/billable") {
      await ok(
        page1([
          {
            source_type: "order",
            source_uuid: SRC,
            source_no: "ORD-2026-0042",
            buyer_organization: { uuid: "d1", name: "Ege Distribütör" },
            currency: "TRY",
            line_extension: "300.00",
            tax_total: "60.00",
            payable: "360.00",
            billable_at: "2026-10-08T10:00:00Z",
            period_start: null,
            period_end: null,
          },
        ]),
      );
      return true;
    }
    if (method === "GET" && path === "/v1/einvoices/settings") {
      await ok(settings);
      return true;
    }
    return false;
  });
}

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("list → detail: preview, archive confirmation then step-up", async ({
  page,
}) => {
  const api = await mockApi(page);
  asCenter(api);

  await page.goto(`/t/${SLUG}/einvoices`);
  await expect(page.getByText("Ege Distribütör").first()).toBeVisible();
  await page.getByTestId("einvoice-row").first().click();

  await expect(page).toHaveURL(new RegExp(`/einvoices/${INV}$`));
  const frame = page.getByTestId("einvoice-preview");
  await expect(frame).toHaveAttribute("sandbox", "");
  await expect(
    page
      .frameLocator('[data-testid="einvoice-preview"]')
      .getByText("PREVIEW e-Arşiv"),
  ).toBeVisible();

  await page.getByTestId("einvoice-archive").click();
  await expect(
    page.getByText("Nothing is sent, the invoice is only archived.", {
      exact: false,
    }),
  ).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Archive" })
    .click();
  await expect(page.getByText("Confirm your identity")).toBeVisible();
  expect(api.calls.some((c) => c.startsWith("POST /v1/einvoices"))).toBe(false);
});

test("an invalid invoice keeps Arşivle disabled", async ({ page }) => {
  const api = await mockApi(page);
  asCenter(api);
  await page.goto(`/t/${SLUG}/einvoices/${BAD}`);
  await expect(page.getByText("[BR-TR-01] Alıcı VKN zorunludur")).toBeVisible();
  await expect(page.getByTestId("einvoice-archive")).toBeDisabled();
  await expect(page.getByTestId("einvoice-invalid-hint")).toBeVisible();
});

test("billable records offer the bulk create draft", async ({ page }) => {
  const api = await mockApi(page);
  asCenter(api);
  await page.goto(`/t/${SLUG}/einvoices/billable`);
  await expect(page.getByText("ORD-2026-0042")).toBeVisible();
  api.extra.push(async ({ method, path, ok }) => {
    if (method === "POST" && path === "/v1/einvoices/billable/bulk") {
      await ok({
        sync: true,
        summary: { total: 1, succeeded: 1, failed: 0 },
        operation: null,
      });
      return true;
    }
    return false;
  });
  await page.getByRole("checkbox").nth(1).check();
  await page.getByRole("button", { name: "Create draft" }).first().click();
  await expect(
    page.getByText("Create e-invoice drafts for the selected records?", {
      exact: false,
    }),
  ).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Confirm" })
    .click();
  await expect
    .poll(() => api.bodies["POST /v1/einvoices/billable/bulk"]?.length ?? 0)
    .toBe(1);
  const body = api.bodies["POST /v1/einvoices/billable/bulk"]![0] as {
    action: string;
    target: { scope: string; ids: string[] };
  };
  expect(body.action).toBe("create_draft");
  expect(body.target).toMatchObject({ scope: "ids", ids: [SRC] });
});

test("settings reject a series outside 3 characters", async ({ page }) => {
  const api = await mockApi(page);
  asCenter(api);
  await page.goto(`/t/${SLUG}/einvoices/settings`);
  await expect(page.getByTestId("einvoice-counters")).toContainText(
    "EAR2026000000042",
  );
  await page.getByTestId("einvoice-earchive-series").fill("AB");
  await page.getByTestId("einvoice-settings-save").click();
  await expect(
    page.getByText("The series must be exactly 3 characters."),
  ).toBeVisible();
  expect(api.calls.some((c) => c.startsWith("PUT /v1/einvoices"))).toBe(false);
});

test("the e-invoice menu sits under Muhasebe only with the module", async ({
  page,
}) => {
  const api = await mockApi(page);
  asCenter(api);
  await page.goto(`/t/${SLUG}/einvoices/settings`);
  await expect(
    page.getByRole("link", { name: "e-Invoice (e-Fatura)" }),
  ).toBeVisible();

  const off = await mockApi(page);
  asCenter(off);
  off.features = off.features.filter((f) => f !== "e_invoice");
  await page.unrouteAll({ behavior: "ignoreErrors" });
  await page.route(
    (url) =>
      url.pathname.startsWith("/api/") &&
      !url.pathname.startsWith("/api/auth/"),
    (route) => off.handle(route),
  );
  await page.reload();
  await expect(page.getByText("The e-invoice add-on is off")).toBeVisible();
  await expect(
    page.getByRole("link", { name: "e-Invoice (e-Fatura)" }),
  ).toHaveCount(0);
});
