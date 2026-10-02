import { defineConfig, devices } from "@playwright/test";

import { E2E_AUTH_SECRET } from "./e2e/support/constants";

/**
 * Mocked end-to-end tests (TEC-183): the production build runs with no Go
 * backend; every browser call to the BFF (`/api/v1/**`) is answered by
 * `page.route` fixtures in e2e/support. Run `pnpm build` first, then
 * `pnpm test:e2e`. The real-stack run stays a manual check.
 */
const PORT = Number(process.env.E2E_PORT ?? 3183);
const baseURL = `http://127.0.0.1:${PORT}`;

export default defineConfig({
  testDir: "./e2e",
  testMatch: "**/*.spec.ts",
  fullyParallel: false,
  workers: 1,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI ? [["list"], ["github"]] : "list",
  use: {
    baseURL,
    locale: "en-US",
    timezoneId: "Europe/Istanbul",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: `next start -p ${PORT} -H 127.0.0.1`,
    url: `${baseURL}/platform/login`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
    env: {
      AUTH_SECRET: E2E_AUTH_SECRET,
      AUTH_TRUST_HOST: "true",
      AUTH_URL: baseURL,
      // No backend: server-side upstream calls fail fast (closed port).
      API_URL: "http://127.0.0.1:9/v1",
      NEXT_TELEMETRY_DISABLED: "1",
    },
  },
});
