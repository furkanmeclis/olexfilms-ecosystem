import { expect, test, type Page } from "@playwright/test";

/**
 * TEC-628: the Linear revision-request widget (issue.technowide.tr) is loaded
 * once from the root layout with next/script (afterInteractive). The default
 * run stubs the embed script so CI needs no network: it checks a single
 * script tag per document and a single execution across a client-side
 * navigation. `E2E_WIDGET_LIVE=1` loads the real script instead and checks
 * the button, the iframe and the screenshot library (manual check; no issue
 * is created).
 */

const EMBED_URL = "https://issue.technowide.tr/embed.js";
const WIDGET_ORIGIN = "https://issue.technowide.tr";

async function stubEmbed(page: Page) {
  await page.route(EMBED_URL, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/javascript",
      body: "window.__lpiLoads = (window.__lpiLoads || 0) + 1;",
    }),
  );
}

test("embed script is injected once per document", async ({ page }) => {
  await stubEmbed(page);
  await page.goto("/portal/login");
  await expect.poll(() => page.evaluate(() => window.__lpiLoads ?? 0)).toBe(1);
  await expect(page.locator(`script[src="${EMBED_URL}"]`)).toHaveCount(1);
  await expect(page.locator(`script[src="${EMBED_URL}"]`)).toHaveAttribute(
    "data-nscript",
    "afterInteractive",
  );

  // A client-side navigation keeps the root layout: no second load.
  await page.goto("/platform/login");
  await expect(page.locator(`script[src="${EMBED_URL}"]`)).toHaveCount(1);
  expect(await page.evaluate(() => window.__lpiLoads)).toBe(1);
});

test("live widget: button once, iframe opens, screenshot library loads", async ({
  page,
}) => {
  test.skip(!process.env.E2E_WIDGET_LIVE, "set E2E_WIDGET_LIVE=1 to run");
  const widgetRequests: string[] = [];
  page.on("request", (request) => {
    if (request.url().startsWith(WIDGET_ORIGIN))
      widgetRequests.push(request.url());
  });

  await page.goto("/portal/login");
  const host = page.locator("#lpi-embed-host");
  await expect(host).toHaveCount(1);
  const button = host.locator(".lpi-button");
  await expect(button).toBeVisible();
  await expect(host.locator(".lpi-label")).toHaveText("Geri bildirim");

  await button.click();
  const frame = host.locator("iframe.lpi-frame");
  await expect(frame).toHaveAttribute(
    "src",
    new RegExp(`^${WIDGET_ORIGIN}/widget\\?.*origin=`),
  );
  await expect(host.locator(".lpi-frame-wrap")).toBeVisible();

  expect(widgetRequests.some((url) => url === EMBED_URL)).toBe(true);
  expect(
    widgetRequests.some((url) => url.startsWith(`${WIDGET_ORIGIN}/widget`)),
  ).toBe(true);
});

declare global {
  interface Window {
    __lpiLoads?: number;
  }
}
