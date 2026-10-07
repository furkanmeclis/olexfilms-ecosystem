import { defineConfig, devices } from "@playwright/test";

import {
  E2E_ADAPTER_SECRET,
  E2E_AUTH_SECRET,
  E2E_UPSTREAM_PORT,
} from "./e2e/support/constants";

/**
 * Mocked end-to-end tests (TEC-183): the production build runs with no Go
 * backend; every browser call to the BFF (`/api/v1/**`) is answered by
 * `page.route` fixtures in e2e/support. The few calls Next.js makes to Go
 * from the server (credentials login, the public warranty page) go to the
 * tiny upstream mock in e2e/support/upstream-mock.mjs (TEC-218), which
 * drops every other request. Run `pnpm build` first, then `pnpm test:e2e`.
 * The real-stack run stays a manual check.
 */
const PORT = Number(process.env.E2E_PORT ?? 3183);
const baseURL = `http://127.0.0.1:${PORT}`;
const upstreamURL = `http://127.0.0.1:${E2E_UPSTREAM_PORT}`;
// e2e-screenshots workflow: a screenshot (and trace) for every test plus an
// HTML report, uploaded as run artifacts. Off for the regular CI run.
const capture = Boolean(process.env.E2E_CAPTURE);

export default defineConfig({
  testDir: "./e2e",
  testMatch: "**/*.spec.ts",
  fullyParallel: false,
  workers: 1,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: capture
    ? [["list"], ["html", { open: "never" }]]
    : process.env.CI
      ? [["list"], ["github"]]
      : "list",
  use: {
    baseURL,
    locale: "en-US",
    timezoneId: "Europe/Istanbul",
    trace: capture ? "on" : "retain-on-failure",
    screenshot: capture ? "on" : "off",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    {
      command: "node e2e/support/upstream-mock.mjs",
      url: `${upstreamURL}/health`,
      reuseExistingServer: !process.env.CI,
      timeout: 30_000,
      env: {
        E2E_UPSTREAM_PORT: String(E2E_UPSTREAM_PORT),
        AUTH_ADAPTER_SECRET: E2E_ADAPTER_SECRET,
      },
    },
    {
      command: `next start -p ${PORT} -H 127.0.0.1`,
      url: `${baseURL}/platform/login`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        AUTH_SECRET: E2E_AUTH_SECRET,
        AUTH_TRUST_HOST: "true",
        AUTH_URL: baseURL,
        AUTH_ADAPTER_SECRET: E2E_ADAPTER_SECRET,
        // Mocked Go API: login + public warranty only, the rest fails fast.
        API_URL: `${upstreamURL}/v1`,
        NEXT_TELEMETRY_DISABLED: "1",
      },
    },
  ],
});
