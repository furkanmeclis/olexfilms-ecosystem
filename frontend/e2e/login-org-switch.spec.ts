import { expect, test } from "@playwright/test";

import { E2E_LOGIN, E2E_UPSTREAM_PORT } from "./support/constants";
import { ORG, SLUG, membership, mockApi } from "./support/mock-api";

/**
 * TEC-218: tenant login through the real Auth.js credentials flow (Go's
 * auth/login is the upstream mock), then the header organization switcher
 * moves the session to a second membership through the real BFF (which
 * rewrites the session cookie). Other browser BFF calls are mocked.
 */

const BETA = {
  ...membership,
  uuid: "0b9c4c1e-0000-4000-8000-000000000003",
  slug: "beta",
  name: "Beta Distribütör",
  type: "distributor",
};

test("login on the tenant page, then switch organization", async ({
  page,
  request,
}) => {
  const api = await mockApi(page);
  api.memberships = [membership, BETA];
  // Before Auth.js sets the session cookie the BFF has no token: 401.
  await page.route("**/api/v1/auth/me", async (route) => {
    const cookie = (await route.request().allHeaders())["cookie"] ?? "";
    if (!cookie.includes("panel-session")) {
      return route.fulfill({
        status: 401,
        json: { error: { code: "UNAUTHORIZED", message: "No session" } },
      });
    }
    return route.fallback();
  });
  // The switch goes through the real BFF (it rewrites the session cookie)
  // to the upstream mock; the BFF mock only records it.
  const switches: string[] = [];
  await page.route("**/api/v1/auth/organization-context", async (route) => {
    const slug = String(
      (route.request().postDataJSON() as { organization_slug?: string })
        .organization_slug,
    );
    switches.push(slug);
    const target = api.memberships.find((m) => m.slug === slug);
    if (target) api.activeOrg = target.uuid as string;
    return route.continue();
  });

  await page.goto(`/t/${SLUG}/login`);
  await expect(page.getByText("Acme Bayi").first()).toBeVisible();

  // Wrong password: Go says 401, the form shows the generic error.
  await page.getByLabel("Email").fill(E2E_LOGIN.email);
  await page.getByRole("textbox", { name: "Password" }).fill("wrong-password");
  await page.getByRole("button", { name: "Login" }).click();
  await expect(page.getByText("Invalid email or password")).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/t/${SLUG}/login$`));

  // Right password: Auth.js signs the session in, the panel opens.
  await page
    .getByRole("textbox", { name: "Password" })
    .fill(E2E_LOGIN.password);
  await page.getByRole("button", { name: "Login" }).click();
  await expect(page).toHaveURL(new RegExp(`/t/${SLUG}$`));
  const switcher = page.getByTestId("organization-switcher");
  await expect(switcher).toContainText("Acme Bayi");

  const upstream = await request.get(
    `http://127.0.0.1:${E2E_UPSTREAM_PORT}/__calls`,
  );
  expect((await upstream.json()) as string[]).toEqual(
    expect.arrayContaining([
      "POST /v1/auth/login",
      "GET /v1/internal/auth/users/by-email",
    ]),
  );
  // The token carries the acme oid: no organization switch was needed.
  expect(api.activeOrg).toBe(ORG);
  expect(switches).toEqual([]);

  // Switch to the distributor membership.
  await switcher.click();
  await expect(
    page.getByTestId(`organization-switcher-item-${SLUG}`),
  ).toBeVisible();
  await page.getByTestId("organization-switcher-item-beta").click();

  await expect(page).toHaveURL(/\/t\/beta$/);
  await expect(page.getByTestId("organization-switcher")).toContainText(
    "Beta Distribütör",
  );
  // The switcher asks for beta first and the session ends on beta. (Today
  // the still-mounted acme context may bounce back to acme once before the
  // navigation lands; only the first and the last switch are pinned.)
  expect(switches[0]).toBe("beta");
  await expect.poll(() => switches.at(-1)).toBe("beta");
  // The session cookie (what the BFF sends to Go) is scoped to beta.
  await expect
    .poll(async () => {
      const res = await page.request.get("/api/auth/session");
      return ((await res.json()) as { organizationUuid?: string })
        .organizationUuid;
    })
    .toBe(BETA.uuid);
  expect(api.activeOrg).toBe(BETA.uuid);
  const after = await request.get(
    `http://127.0.0.1:${E2E_UPSTREAM_PORT}/__calls`,
  );
  expect((await after.json()) as string[]).toContain(
    "POST /v1/auth/organization-context",
  );
});
