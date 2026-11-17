import { expect, test } from "@playwright/test";

import {
  customer,
  mockApi,
  SERVICE_UUID,
  signIn,
  SLUG,
} from "./support/mock-api";

/**
 * TEC-1217: cross-record links. A center admin opens a service and follows
 * the customer link (EntityLink) to the customer card.
 */

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("service → customer through EntityLink", async ({ page }) => {
  const api = await mockApi(page);
  api.seedDraft();
  api.features.push("customers");
  const customerPath = `/v1/customers/${customer.uuid}`;
  api.extra.push(({ method, path, ok }) => {
    if (method === "GET" && path === customerPath) {
      void ok({
        ...customer,
        tax_office: null,
        national_id_last4: null,
        tax_no_last4: null,
        address: {},
        notification_prefs: {},
        editable: false,
        identity_editable: false,
        organizations: [],
      });
      return true;
    }
    return false;
  });

  await page.goto(`/t/${SLUG}/services/${SERVICE_UUID}`);
  const section = page.getByTestId("detail-customer");
  const link = section.getByRole("link", { name: "Ayşe Yılmaz" });
  await expect(link).toBeVisible();
  await link.click();
  await expect(page).toHaveURL(
    new RegExp(`/t/${SLUG}/customers/${customer.uuid}$`),
  );
  await expect(
    page.getByRole("heading", { name: "Ayşe Yılmaz" }),
  ).toBeVisible();
});
