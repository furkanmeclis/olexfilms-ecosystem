import { expect, test } from "@playwright/test";

import {
  customer,
  mockApi,
  ORG,
  SERVICE_UUID,
  signIn,
  SLUG,
} from "./support/mock-api";

/**
 * TEC-1217: cross-record links. A panel user opens a service, follows the
 * customer link (EntityLink) to the customer card, opens the customer's
 * services tab and from a service row goes to the dealer's organization
 * page and its tabs.
 */

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

const NOW = "2026-10-01T09:00:00Z";

test("service → customer → customer's services → dealer page", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.seedDraft();
  api.features.push("customers");
  api.permissions.push("organizations.read");
  const customerPath = `/v1/customers/${customer.uuid}`;
  const dealer = {
    uuid: ORG,
    slug: SLUG,
    name: "Acme Bayi",
    city: "İstanbul",
    district: "Kadıköy",
    phone: "+902161234567",
    address: "Bağdat Cd. 1",
    status: "active",
    access_starts_at: NOW,
    type: "dealer",
    parent: null,
    created_at: NOW,
    updated_at: NOW,
  };
  api.extra.push(({ method, path, ok }) => {
    if (method !== "GET") return false;
    if (path === customerPath) {
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
    if (path === `/v1/tenant/organizations/${ORG}`) {
      void ok(dealer);
      return true;
    }
    if (path === "/v1/tenant/organizations") {
      void ok({ items: [dealer], scope: "brand" });
      return true;
    }
    return false;
  });

  // Service detail → customer.
  await page.goto(`/t/${SLUG}/services/${SERVICE_UUID}`);
  await page
    .getByTestId("detail-customer")
    .getByRole("link", { name: "Ayşe Yılmaz" })
    .click();
  await expect(page).toHaveURL(
    new RegExp(`/t/${SLUG}/customers/${customer.uuid}$`),
  );
  await expect(
    page.getByRole("heading", { name: "Ayşe Yılmaz" }),
  ).toBeVisible();

  // Customer → its services (reverse filter) → the dealer of a service.
  const related = page.getByTestId("customer-related");
  await expect(related.getByRole("link", { name: "DSE2E00001" })).toBeVisible();
  await expect
    .poll(() =>
      api.calls.some(
        (c) =>
          c.startsWith(`GET /v1/services?`) &&
          c.includes(`customer_uuid=${customer.uuid}`),
      ),
    )
    .toBe(true);
  await related.getByRole("link", { name: "Acme Bayi" }).click();
  await expect(page).toHaveURL(new RegExp(`/t/${SLUG}/organizations/${ORG}$`));
  await expect(page.getByRole("heading", { name: "Acme Bayi" })).toBeVisible();
  await expect(page.getByText("Bağdat Cd. 1")).toBeVisible();

  // Organization tabs: services with the organization filter.
  await page.getByTestId("organization-tab-services").click();
  await expect(page).toHaveURL(/tab=services/);
  await expect(page.getByRole("link", { name: "DSE2E00001" })).toBeVisible();
  expect(
    api.calls.some(
      (c) =>
        c.startsWith("GET /v1/services?") &&
        c.includes(`organization_uuid=${ORG}`),
    ),
  ).toBe(true);
  // A dealer staff without accounting / price permissions has no cari tab.
  await expect(page.getByTestId("organization-tab-cari")).toHaveCount(0);
});
