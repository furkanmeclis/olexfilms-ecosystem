import { expect, test, type Route } from "@playwright/test";

import { E2E_QUOTE } from "./support/constants";

/**
 * TEC-320: the public quote page `/teklif/{token}` (server rendered, Go is
 * the upstream mock) and the dealer application form `/bayi-basvuru`
 * (config from the upstream mock; the browser's geo pickers and the submit
 * go through the BFF and are answered with `page.route`).
 */

const envelope = (data: unknown) => ({ success: true, data, meta: {} });

test("quote link shows the quote without contact details", async ({ page }) => {
  await page.goto(`/teklif/${E2E_QUOTE.ok}?lang=en`);

  const card = page.locator('[data-screen="quote"]');
  await expect(card).toBeVisible();
  await expect(
    card.getByRole("heading", { level: 1, name: "Olex Kadıköy" }),
  ).toBeVisible();
  await expect(card).toContainText("Q-000042");
  await expect(card.locator('[data-slot="quote-line"]')).toHaveCount(2);
  await expect(card.locator('[data-slot="grand-total"]')).toContainText(
    "11,500.00",
  );
  await expect(card.locator('[data-slot="pdf-download"]')).toHaveAttribute(
    "href",
    `/teklif/${E2E_QUOTE.ok}/pdf?lang=en`,
  );
  await expect(page.locator("a[href^='tel:'], a[href^='mailto:']")).toHaveCount(
    0,
  );
});

test("Go's /portal/quotes/{token} link opens the public page", async ({
  page,
}) => {
  await page.goto(`/portal/quotes/${E2E_QUOTE.ok}`);
  await expect(page).toHaveURL(new RegExp(`/teklif/${E2E_QUOTE.ok}$`));
  await expect(page.locator('[data-screen="quote"]')).toBeVisible();
});

test("unknown quote token is a 404 page", async ({ page }) => {
  const res = await page.goto(`/teklif/${E2E_QUOTE.missing}`);
  expect(res?.status()).toBe(404);
  await expect(page.locator('[data-screen="not-found"]')).toBeVisible();
  await expect(page.locator('[data-screen="quote"]')).toHaveCount(0);
});

test("dealer application: country loads provinces, KVKK gates submit, success", async ({
  page,
}) => {
  let posted: Record<string, unknown> | null = null;
  await page.route("**/api/v1/public/geo/countries", (route: Route) =>
    route.fulfill({
      json: envelope({
        items: [
          {
            id: 1,
            iso2: "TR",
            iso3: "TUR",
            name_en: "Turkey",
            name_tr: "Türkiye",
            is_active: true,
            has_provinces: true,
          },
        ],
      }),
    }),
  );
  await page.route("**/api/v1/public/geo/countries/TR/provinces", (route) =>
    route.fulfill({
      json: envelope({
        items: [
          {
            id: 34,
            country_id: 1,
            code: "34",
            name: "İstanbul",
            has_districts: false,
          },
        ],
      }),
    }),
  );
  await page.route("**/api/v1/public/dealer-applications", (route) => {
    posted = route.request().postDataJSON() as Record<string, unknown>;
    return route.fulfill({ status: 202, json: envelope({ received: true }) });
  });

  await page.goto("/bayi-basvuru?lang=en");
  const form = page.locator('[data-slot="dealer-application-form"]');
  await expect(form).toBeVisible();

  const province = form.locator('select[name="province_id"]');
  await expect(province).toBeDisabled();
  await form.locator('select[name="country_id"]').selectOption("1");
  await expect(province).toBeEnabled();
  await province.selectOption("34");

  await form.locator('input[name="company_name"]').fill("Kadıköy Kaplama");
  await form.locator('input[name="contact_name"]').fill("Ayşe Yılmaz");
  await form.locator('input[name="phone"]').fill("12");

  const submit = form.getByRole("button", { name: "Send application" });
  await expect(submit).toBeDisabled();
  await form.getByRole("checkbox").click();
  await expect(submit).toBeEnabled();

  await submit.click();
  await expect(form).toContainText("Enter a valid phone number.");
  expect(posted).toBeNull();

  await form.locator('input[name="phone"]').fill("0532 123 45 67");
  await submit.click();
  await expect(page.locator('[data-screen="success"]')).toContainText(
    "Application received",
  );
  expect(posted).toMatchObject({
    phone: "+905321234567",
    country_id: 1,
    province_id: 34,
    kvkk_consent: true,
    language: "en",
    website: "",
  });
});
