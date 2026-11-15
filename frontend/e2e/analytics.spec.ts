import { expect, test, type Page } from "@playwright/test";

import {
  membership,
  mockApi,
  signIn,
  SLUG,
  type MockApi,
} from "./support/mock-api";
import { mockEmptyLists, mockReports } from "./support/reports-mock";

/**
 * TEC-1212: role overviews (center, dealer, distributor, warehouse,
 * accounting) on the analytics kit and the reports center with a report
 * detail page, against the mocked report envelope v2 (TEC-1210).
 * E2E_ANALYTICS_SHOTS=<dir> also writes light / dark screenshots of every
 * role for the PR.
 */

type Role = {
  name: string;
  orgType: "center" | "distributor" | "dealer";
  permissions: string[];
  features: string[];
  testId: string;
};

const ROLES: Role[] = [
  {
    name: "center",
    orgType: "center",
    permissions: [
      "services.read",
      "services.write",
      "orders.read",
      "customers.read",
      "warranties.read",
      "measurements.read",
      "stock.read",
      "organizations.read",
      "accounting.read",
      "warehouse.read",
    ],
    features: [
      "services",
      "orders",
      "stock",
      "organizations",
      "customers",
      "accounting",
    ],
    testId: "center-overview",
  },
  {
    name: "dealer",
    orgType: "dealer",
    permissions: [
      "services.read",
      "services.write",
      "orders.read",
      "customers.read",
      "warranties.read",
      "measurements.read",
    ],
    features: ["services", "orders", "customers", "measurements"],
    testId: "dealer-overview",
  },
  {
    name: "distributor",
    orgType: "distributor",
    permissions: [
      "services.read",
      "services.write",
      "orders.read",
      "organizations.read",
      "warranties.read",
    ],
    features: ["services", "orders", "organizations"],
    testId: "distributor-overview",
  },
  {
    name: "warehouse",
    orgType: "center",
    permissions: ["warehouse.read", "orders.read", "stock.read"],
    features: ["orders", "stock"],
    testId: "warehouse-overview",
  },
  {
    name: "accounting",
    orgType: "center",
    permissions: ["accounting.read", "orders.read"],
    features: ["accounting", "orders"],
    testId: "accounting-overview",
  },
];

async function setup(
  page: Page,
  role: Role,
  broken?: string,
): Promise<MockApi> {
  const api = await mockApi(page);
  api.memberships = [
    { ...membership, type: role.orgType, name: `Acme ${role.name}` },
  ];
  api.permissions = role.permissions;
  api.features = role.features;
  if (broken) {
    api.extra.push(async (call) => {
      if (call.method === "GET" && call.path === `/v1/reports/${broken}`) {
        await call.route.fulfill({
          status: 500,
          json: { error: { code: "INTERNAL", message: "boom" } },
        });
        return true;
      }
      return false;
    });
  }
  mockReports(api);
  mockEmptyLists(api);
  return api;
}

const shots = process.env.E2E_ANALYTICS_SHOTS;

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

for (const role of ROLES) {
  test(`overview: ${role.name}`, async ({ page }) => {
    const api = await setup(page, role);
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto(`/t/${SLUG}`);
    const overview = page.getByTestId(role.testId);
    await expect(overview).toBeVisible();
    await expect(overview.getByTestId("overview-period")).toBeVisible();
    // One period drives the panels: compare=previous by default (the
    // accounting P&L asks the period and the compared window by date).
    const periodCalls = () =>
      role.name === "accounting"
        ? api.calls.filter((c) =>
            c.startsWith("GET /v1/accounting/reports/pnl"),
          )
        : api.calls.filter((c) => c.includes("compare=previous"));
    await expect
      .poll(() => periodCalls().length)
      .toBeGreaterThan(role.name === "accounting" ? 1 : 0);
    const before = periodCalls().length;
    // Picking another period refetches with it.
    await overview
      .getByTestId("overview-period")
      .locator('[data-preset="90d"]')
      .click();
    await expect
      .poll(() =>
        role.name === "accounting"
          ? periodCalls().length > before
          : api.calls.some((c) => c.includes("period=90d")),
      )
      .toBe(true);
    if (shots) {
      for (const scheme of ["light", "dark"] as const) {
        await page.emulateMedia({ colorScheme: scheme });
        await page.evaluate((s) => {
          document.documentElement.classList.toggle("dark", s === "dark");
        }, scheme);
        await page.waitForTimeout(600);
        await page.screenshot({
          path: `${shots}/overview-${role.name}-${scheme}.png`,
          fullPage: true,
        });
      }
    }
  });
}

test("overview: a failing panel shows a retry instead of vanishing", async ({
  page,
}) => {
  await setup(page, ROLES[0]!, "services.trend");
  await page.goto(`/t/${SLUG}`);
  const panel = page.getByTestId("trend-services.trend");
  await expect(panel).toBeVisible();
  await expect(panel.getByRole("alert")).toBeVisible();
  await expect(panel.getByRole("button")).toBeVisible();
});

test("reports center: categories, detail page and export", async ({ page }) => {
  const api = await setup(page, ROLES[0]!);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto(`/t/${SLUG}/reports`);
  const center = page.getByTestId("reports-center");
  await expect(center).toBeVisible();
  await expect(center.getByTestId("reports-category-services")).toBeVisible();
  await expect(center.getByTestId("reports-category-network")).toBeVisible();
  await expect(center.getByTestId("reports-category-warehouse")).toBeVisible();
  // the dedicated accounting reports page sits in its category
  await expect(
    center
      .getByTestId("reports-category-accounting")
      .locator('[data-report="accounting.reports"]'),
  ).toBeVisible();
  if (shots) {
    await page.screenshot({
      path: `${shots}/reports-center-light.png`,
      fullPage: true,
    });
  }

  await center.locator('[data-report="network.summary"]').click();
  const detail = page.getByTestId("report-detail");
  await expect(detail).toBeVisible();
  await expect(detail.getByTestId("report-kpis")).toBeVisible();
  await expect(detail.getByTestId("report-trend")).toBeVisible();
  await expect(detail.getByTestId("report-grid")).toBeVisible();
  // row drill-down: an organization row opens its services
  await expect(
    detail
      .getByTestId("report-grid")
      .locator('a[href*="/services?organization_uuid="]')
      .first(),
  ).toBeVisible();
  if (shots) {
    for (const scheme of ["light", "dark"] as const) {
      await page.emulateMedia({ colorScheme: scheme });
      await page.evaluate((s) => {
        document.documentElement.classList.toggle("dark", s === "dark");
      }, scheme);
      await page.waitForTimeout(600);
      await page.screenshot({
        path: `${shots}/report-detail-${scheme}.png`,
        fullPage: true,
      });
    }
  }
  await page.getByTestId("report-export").click();
  await page.getByRole("menuitem", { name: "CSV" }).click();
  await expect
    .poll(() =>
      api.calls.some(
        (c) =>
          c.startsWith("GET /v1/reports/network.summary/export") &&
          c.includes("format=csv"),
      ),
    )
    .toBe(true);
});
