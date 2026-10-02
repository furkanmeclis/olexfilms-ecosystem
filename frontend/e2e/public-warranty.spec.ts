import { expect, test, type APIRequestContext } from "@playwright/test";

import { E2E_UPSTREAM_PORT, E2E_WARRANTY } from "./support/constants";

/**
 * TEC-218: the public warranty page `/garanti/{public_code}` (TEC-189).
 * It renders on the server, so Go is the upstream mock (no session, no
 * browser call): a valid code, an unknown code, a malformed code (no
 * upstream call at all) and the per-IP rate limit (429 + Retry-After).
 */

async function upstreamCalls(request: APIRequestContext): Promise<string[]> {
  const res = await request.get(
    `http://127.0.0.1:${E2E_UPSTREAM_PORT}/__calls`,
  );
  return (await res.json()) as string[];
}

test("valid code shows the warranty without personal data", async ({
  page,
  request,
}) => {
  await page.goto(`/garanti/${E2E_WARRANTY.ok}`);

  const card = page.locator('[data-screen="warranty"]');
  await expect(card).toBeVisible();
  await expect(
    card.getByRole("heading", { level: 1, name: "Olex PPF Gloss" }),
  ).toBeVisible();
  await expect(card.locator('[data-status="active"]')).toHaveText("Active");
  await expect(card.locator('[data-slot="days-remaining"]')).toHaveText(
    "3,393",
  );
  await expect(card).toContainText("Olexfilms");
  await expect(card).toContainText("BMW M3");
  await expect(card).toContainText("34 *** 12");
  await expect(card).toContainText("6752");
  await expect(card).toContainText("Kadıköy Bayi");
  await expect(card).toContainText("İstanbul");
  await expect(page).toHaveTitle(/Warranty check/);

  expect(await upstreamCalls(request)).toContain(
    `GET /v1/public/warranties/${E2E_WARRANTY.ok}`,
  );
});

test("unknown code shows not found", async ({ page }) => {
  await page.goto(`/garanti/${E2E_WARRANTY.missing}`);

  const notice = page.locator('[data-screen="not-found"]');
  await expect(notice).toBeVisible();
  await expect(notice).toContainText("Warranty not found");
  await expect(page.locator('[data-screen="warranty"]')).toHaveCount(0);
});

test("malformed code is not found without asking the API", async ({
  page,
  request,
}) => {
  await page.goto("/garanti/abc");

  await expect(page.locator('[data-screen="not-found"]')).toBeVisible();
  const calls = await upstreamCalls(request);
  expect(calls.some((c) => c.endsWith("/public/warranties/abc"))).toBe(false);
});

test("rate limited (429) shows the retry notice", async ({ page }) => {
  await page.goto(`/garanti/${E2E_WARRANTY.limited}`);

  const notice = page.locator('[data-screen="rate-limited"]');
  await expect(notice).toBeVisible();
  await expect(notice).toContainText("Too many requests");
  await expect(notice).toContainText("Try again in 42 seconds.");
  await expect(page.locator('[data-screen="warranty"]')).toHaveCount(0);
});
