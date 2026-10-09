import { expect, test } from "@playwright/test";

import { mockApi, signIn, SLUG, type MockApi } from "./support/mock-api";

/**
 * TEC-507 (F5-09c): recommended price screens against a mocked BFF — the
 * center's price list (inline edit → basket → "Publish" with a past day
 * disabled and the step-up dialog), the price discipline report and the
 * dealer's sale prices with the recommended column and deviation badge.
 */

const P1 = "0b9c4c1e-0000-4000-8000-000000000507";
const page1 = <T>(items: T[]) => ({
  items,
  total: items.length,
  limit: 20,
  offset: 0,
});

function asCenter(api: MockApi) {
  api.memberships = api.memberships.map((m) => ({
    ...m,
    type: "center",
    name: "Olex Merkez",
  }));
  api.permissions.push(
    "pricing.recommended.read",
    "pricing.recommended.write",
    "pricing.discipline.read",
  );
  api.features.push("catalog");
}

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("center drafts a recommended price and publish asks for step-up", async ({
  page,
}) => {
  const api = await mockApi(page);
  asCenter(api);
  api.extra.push(async ({ method, path, ok }) => {
    if (method === "GET" && path === "/v1/catalog/products") {
      await ok(
        page1([
          {
            uuid: P1,
            sku: "PPF-190",
            name: "Olex PPF 190",
            active: true,
            category: { uuid: "c1", name: "PPF" },
          },
        ]),
      );
      return true;
    }
    if (method === "GET" && path === "/v1/tenant/pricing/recommended/current") {
      await ok(
        page1([
          {
            product_uuid: P1,
            product_sku: "PPF-190",
            product_name: "Olex PPF 190",
            price: "1000.00",
            currency: "TRY",
            country_iso2: "",
            scope: "currency",
            effective_from: "2026-10-01",
            version_uuid: "v1",
            source: "publish",
            batch_id: null,
          },
        ]),
      );
      return true;
    }
    if (
      method === "GET" &&
      path === "/v1/tenant/pricing/recommended/versions"
    ) {
      await ok(page1([]));
      return true;
    }
    if (
      method === "GET" &&
      path === "/v1/tenant/pricing/recommended/settings"
    ) {
      await ok({ deviation_warning_pct: 15 });
      return true;
    }
    if (method === "GET" && path === "/v1/geo/countries") {
      await ok({
        items: [
          {
            id: 1,
            iso2: "TR",
            iso3: "TUR",
            name_en: "Turkey",
            name_tr: "Türkiye",
            default_currency: "TRY",
          },
        ],
      });
      return true;
    }
    return false;
  });

  await page.goto(`/t/${SLUG}/catalog/recommended-prices`);
  await expect(page.getByText("Olex PPF 190")).toBeVisible();
  await expect(page.getByTestId("open-publish")).toBeDisabled();

  await page.getByText("Double-click to set").dblclick();
  const editor = page.locator("input:focus");
  await editor.fill("1100");
  await editor.press("Enter");
  await expect(page.getByTestId("publish-basket")).toBeVisible();
  await expect(page.getByTestId("open-publish")).toBeEnabled();

  await page.getByTestId("open-publish").click();
  const dialog = page.getByTestId("publish-dialog");
  await expect(dialog).toBeVisible();
  await dialog.locator("#recommended-effective-from").click();
  // Days before today are disabled in the calendar.
  await expect(
    page.locator('[data-disabled="true"][data-day]').first(),
  ).toBeVisible();
  await page.keyboard.press("Escape");

  await dialog.getByTestId("publish-submit").click();
  await expect(page.getByText("Confirm your identity")).toBeVisible();
  expect(
    api.calls.some((c) => c.startsWith("POST /v1/tenant/pricing/recommended")),
  ).toBe(false);
});

test("price discipline shows country cards and over-threshold rows", async ({
  page,
}) => {
  const api = await mockApi(page);
  asCenter(api);
  api.extra.push(async ({ method, path, ok }) => {
    if (method === "GET" && path === "/v1/pricing/discipline/summary") {
      await ok({
        snapshot_date: "2026-10-09",
        threshold_pct: 15,
        countries: [
          {
            country_iso2: "TR",
            country_name_en: "Turkey",
            country_name_tr: "Türkiye",
            currency: "TRY",
            org_count: 2,
            row_count: 2,
            avg_deviation_pct: "11.00",
            median_deviation_pct: "11.00",
            over_threshold_org_count: 1,
          },
        ],
        products: [],
      });
      return true;
    }
    if (method === "GET" && path === "/v1/pricing/discipline") {
      await ok(
        page1([
          {
            snapshot_date: "2026-10-09",
            organization_uuid: "o2",
            organization_name: "Kadıköy Bayi",
            organization_type: "dealer",
            product_uuid: P1,
            product_sku: "PPF-190",
            product_name: "Olex PPF 190",
            country_iso2: "TR",
            currency: "TRY",
            recommended_price: "1000.00",
            list_price: "1200.00",
            deviation_pct: "20.00",
            avg_sale_price: "1180.00",
            sales_quantity: "3",
            over_threshold: true,
          },
        ]),
      );
      return true;
    }
    if (method === "GET" && path === "/v1/tenant/organizations") {
      await ok(page1([{ uuid: "d1", name: "Ege Distribütör" }]));
      return true;
    }
    return false;
  });

  await page.goto(`/t/${SLUG}/catalog/price-discipline`);
  await expect(page.getByTestId("discipline-country-card")).toContainText(
    "Turkey",
  );
  await expect(page.getByText("Kadıköy Bayi")).toBeVisible();
  await expect(
    page.locator('[data-testid="deviation-badge"][data-over-threshold="true"]'),
  ).toBeVisible();
});
