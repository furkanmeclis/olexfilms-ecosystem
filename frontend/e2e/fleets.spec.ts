import { expect, test } from "@playwright/test";

import { mockApi, signIn, SLUG, type MockApi } from "./support/mock-api";

/**
 * TEC-477 (F5-02f): panel fleet screens against a mocked BFF — the fleet
 * list and the "new fleet" VKN lookup (a registered VKN gets a link
 * request, not the form), the bulk service plan wizard (capacity warnings,
 * 409 → preview again) and the row-wise intake of the plan detail.
 */

const FLEET = "0b9c4c1e-0000-4000-8000-000000000477";
const PLAN = "0b9c4c1e-0000-4000-8000-000000004771";
const V = [1, 2, 3].map((i) => `0b9c4c1e-0000-4000-8000-00000000477${i + 1}`);
const A = [
  "0b9c4c1e-0000-4000-8000-0000000047a1",
  "0b9c4c1e-0000-4000-8000-0000000047a2",
];
const NOW = "2026-10-08T09:00:00Z";

const fail = (code: string) => ({
  success: false,
  error: { code, message: code },
  meta: {},
});

const link = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000047b1",
  status: "active",
  dealer_uuid: "0b9c4c1e-0000-4000-8000-000000000001",
  dealer_name: "Acme Bayi",
  started_at: NOW,
  ended_at: null,
  created_at: NOW,
};

const card = {
  uuid: FLEET,
  name: "Acme Filo",
  status: "active",
  profile: {
    tax_number: "1234567890",
    tax_office: null,
    legal_name: "Acme Lojistik A.Ş.",
    contact_name: null,
    contact_phone: null,
    billing_email: null,
    report_frequency: "monthly",
    report_locale: "tr",
  },
  has_primary_user: true,
  vehicle_count: 3,
  active_warranty_count: 0,
  service_count: 0,
  recent_services: [],
  links: [link],
  cari: null,
  created_at: NOW,
};

const vehicle = (uuid: string, i: number) => ({
  uuid,
  plate: `34 FLT 0${i + 1}`,
  plate_country: "TR",
  vin: null,
  model_year: 2024,
  car_brand: { uuid: "b", name: "Renault" },
  car_model: { uuid: "m", name: "Clio" },
  last_service_at: null,
  active_warranty_count: 0,
  created_at: NOW,
});

function grantFleet(api: MockApi) {
  api.permissions.push("fleets.read", "fleets.manage", "fleets.plan");
  api.features.push("fleet", "appointments");
}

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("fleets: list, and a registered VKN gets a link request instead of the form", async ({
  page,
}) => {
  const api = await mockApi(page);
  grantFleet(api);
  const linked: string[] = [];
  api.extra.push(async ({ method, path, url, ok }) => {
    if (method === "GET" && path === "/v1/fleets") {
      await ok({
        items: [
          {
            uuid: FLEET,
            name: "Acme Filo",
            legal_name: "Acme Lojistik A.Ş.",
            tax_number: "1234567890",
            status: "active",
            vehicle_count: 3,
            last_service_at: null,
            link,
          },
        ],
        total: 1,
        limit: 20,
        offset: 0,
      });
      return true;
    }
    if (method === "GET" && path === "/v1/fleets/lookup") {
      expect(url.searchParams.get("tax_number")).toBe("9876543210");
      await ok({
        fleet_uuid: "0b9c4c1e-0000-4000-8000-0000000047c1",
        name: "Beta Filo",
        legal_name: "Beta Taşımacılık A.Ş.",
        link_status: "",
      });
      return true;
    }
    if (method === "POST" && /^\/v1\/fleets\/[^/]+\/links$/.test(path)) {
      linked.push(path);
      await ok({ uuid: "0b9c4c1e-0000-4000-8000-0000000047c1" }, 201);
      return true;
    }
    return false;
  });

  await page.goto(`/t/${SLUG}/fleets`);
  await expect(page.getByTestId("fleet-row")).toContainText("Acme Filo");

  await page.getByRole("button", { name: "New fleet" }).click();
  await page.getByTestId("fleet-tax-number").fill("9876543210");
  await page.getByTestId("fleet-lookup").click();
  await expect(page.getByTestId("fleet-found")).toContainText(
    "Beta Taşımacılık A.Ş.",
  );
  await expect(page.getByTestId("fleet-open-form")).toHaveCount(0);
  await page.getByTestId("fleet-request-link").click();
  await expect
    .poll(() => linked)
    .toEqual(["/v1/fleets/0b9c4c1e-0000-4000-8000-0000000047c1/links"]);
});

test("fleet plan: capacity warning, 409 → preview again, then row-wise intake", async ({
  page,
}) => {
  const api = await mockApi(page);
  grantFleet(api);
  let creates = 0;
  let previews = 0;
  api.extra.push(async ({ method, path, ok, route }) => {
    if (method === "GET" && path === `/v1/fleets/${FLEET}`) {
      await ok(card);
      return true;
    }
    if (method === "GET" && path === `/v1/fleets/${FLEET}/vehicles`) {
      await ok({ items: V.map(vehicle), total: 3, limit: 100, offset: 0 });
      return true;
    }
    if (
      method === "POST" &&
      path === `/v1/fleets/${FLEET}/service-plans/preview`
    ) {
      previews++;
      await ok({
        fleet_uuid: FLEET,
        dealer_uuid: link.dealer_uuid,
        service_type: "PPF",
        note: "",
        appointments: V.map((uuid, i) => ({
          vehicle_uuid: uuid,
          starts_at: `2026-10-12T0${6 + i}:00:00Z`,
        })),
        warnings: [],
      });
      return true;
    }
    if (method === "POST" && path === `/v1/fleets/${FLEET}/service-plans`) {
      creates++;
      if (creates === 1) {
        await route.fulfill({
          status: 409,
          json: fail("FLEET_SERVICE_PLAN_STALE"),
        });
        return true;
      }
      await ok({ uuid: PLAN }, 201);
      return true;
    }
    if (
      method === "GET" &&
      path === `/v1/fleets/${FLEET}/service-plans/${PLAN}`
    ) {
      await ok({
        uuid: PLAN,
        fleet_uuid: FLEET,
        dealer_uuid: link.dealer_uuid,
        status: "scheduled",
        service_type: "PPF",
        note: "",
        created_at: NOW,
        appointments: A.map((uuid, i) => ({
          uuid,
          organization_id: 1,
          customer_user_id: 1,
          starts_at: `2026-10-12T0${6 + i}:00:00Z`,
          ends_at: `2026-10-12T0${7 + i}:00:00Z`,
          estimated_minutes: 60,
          source: "fleet_plan",
          status: "scheduled",
          note: "",
          vehicle_uuid: V[i],
          vehicle_plate: `34 FLT 0${i + 1}`,
          service_uuid: null,
        })),
      });
      return true;
    }
    if (
      method === "POST" &&
      path === `/v1/fleets/${FLEET}/service-plans/${PLAN}/start-intake`
    ) {
      await ok({
        plan_uuid: PLAN,
        results: [
          { appointment_uuid: A[0], ok: true, service_uuid: "svc-1" },
          {
            appointment_uuid: A[1],
            ok: false,
            code: "APPOINTMENT_INVALID_TRANSITION",
            message: "appointment status transition is not allowed",
          },
        ],
      });
      return true;
    }
    return false;
  });

  await page.goto(
    `/t/${SLUG}/fleets/${FLEET}/plans/new?vehicles=${V.join(",")}`,
  );
  await page.getByTestId("plan-service-type").fill("PPF");
  await page.getByTestId("plan-daily-max").fill("2");
  await page.getByTestId("plan-preview").click();

  await expect(page.getByTestId("plan-warnings")).toBeVisible();
  await expect(page.getByTestId("plan-row-warning-capacity")).toHaveCount(3);

  await page.getByTestId("plan-confirm").click();
  await expect(page.getByTestId("plan-stale")).toBeVisible();
  await expect(page.getByTestId("plan-confirm")).toBeDisabled();
  await page.getByTestId("plan-repreview").click();
  await expect(page.getByTestId("plan-stale")).toHaveCount(0);
  expect(previews).toBe(2);

  await page.getByTestId("plan-confirm").click();
  await page.waitForURL(new RegExp(`/fleets/${FLEET}/plans/${PLAN}$`));

  await page.getByRole("checkbox", { name: "Select all" }).first().click();
  await page.getByTestId("intake-selected").click();
  await expect(page.getByTestId("intake-result-ok")).toHaveAttribute(
    "data-appointment",
    A[0],
  );
  await expect(page.getByTestId("intake-result-error")).toHaveAttribute(
    "data-appointment",
    A[1],
  );
  await expect(page.getByTestId("intake-summary")).toBeVisible();
});
