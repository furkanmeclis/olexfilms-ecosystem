import { expect, test } from "@playwright/test";

import { mockApi, signIn, SLUG } from "./support/mock-api";

/**
 * TEC-403 (F4-03d): MCP screens against a mocked BFF — the OAuth consent
 * screen (organization choice before "Allow"), the pending AI actions
 * screen with its confirmation card, and "Connected apps" on the profile.
 */

const REQ = "0b9c4c1e-0000-4000-8000-000000000403";
const ACTION = "0b9c4c1e-0000-4000-8000-000000000404";
const GRANT = "0b9c4c1e-0000-4000-8000-000000000405";

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("consent: choose an organization, then allow", async ({ page }) => {
  const api = await mockApi(page);
  const decisions: unknown[] = [];
  api.extra.push(async ({ method, path, ok, body }) => {
    if (method === "GET" && path === `/v1/oauth/requests/${REQ}`) {
      await ok({
        request_uuid: REQ,
        client_id: "claude-client",
        client_name: "Claude",
        redirect_host: "claude.ai",
        resource: "/mcp/dealer",
        resource_url: "https://olexfilms.test/mcp/dealer",
        realm: "dealer",
        organizations: [
          {
            uuid: "0b9c4c1e-0000-4000-8000-0000000004a1",
            name: "Acme Bayi",
            type: "dealer",
            tool_count: 21,
          },
          {
            uuid: "0b9c4c1e-0000-4000-8000-0000000004a2",
            name: "Beta Bayi",
            type: "dealer",
            tool_count: 9,
          },
        ],
        expires_at: new Date(Date.now() + 10 * 60_000).toISOString(),
      });
      return true;
    }
    if (method === "POST" && path === `/v1/oauth/requests/${REQ}/decide`) {
      decisions.push(body);
      await ok({ redirect_url: "/oauth-consent-done?code=abc" });
      return true;
    }
    return false;
  });

  await page.goto(`/oauth/consent?request=${REQ}`);
  await expect(page.getByTestId("oauth-consent")).toBeVisible();
  await expect(page.getByTestId("oauth-consent-redirect")).toContainText(
    "claude.ai",
  );
  const allow = page.getByTestId("oauth-consent-approve");
  await expect(allow).toBeDisabled();

  await page.getByRole("radio", { name: /Beta Bayi/ }).click();
  await expect(page.getByTestId("oauth-consent-summary")).toContainText("9");
  await expect(allow).toBeEnabled();
  await allow.click();
  await page.waitForURL(/oauth-consent-done\?code=abc/);
  expect(decisions).toEqual([
    {
      decision: "approve",
      organization_uuid: "0b9c4c1e-0000-4000-8000-0000000004a2",
    },
  ]);
});

test("pending AI actions: open the card from the link and approve", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.permissions.push("ai.actions.confirm");
  api.features.push("mcp");
  let done = false;
  const listQueries: string[] = [];
  const decided: string[] = [];
  api.extra.push(async ({ method, path, url, ok }) => {
    if (method === "GET" && path === "/v1/ai/pending-actions") {
      listQueries.push(url.search);
      const items = done
        ? []
        : [
            {
              action_uuid: ACTION,
              tool_use_id: "mcp-1",
              tool_name: "create_lead",
              source: "mcp",
              status: "pending",
              preview: {
                action: "create_lead",
                summary: "Lead for Ayse",
                fields: [{ key: "contact_name", value: "Ayse" }],
              },
              expires_at: new Date(Date.now() + 20 * 60_000).toISOString(),
              created_at: new Date().toISOString(),
            },
          ];
      await ok({ items, total: items.length, limit: 20, offset: 0 });
      return true;
    }
    if (method === "POST" && path.startsWith("/v1/ai/pending-actions/")) {
      decided.push(path);
      done = true;
      await ok({
        action_uuid: ACTION,
        tool_use_id: "mcp-1",
        tool_name: "create_lead",
        source: "mcp",
        status: "confirmed",
      });
      return true;
    }
    return false;
  });

  await page.goto(`/t/${SLUG}/assistant/approvals?action=${ACTION}`);
  const dialog = page.getByTestId("pending-action-dialog");
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("Ayse");
  await expect(page.getByTestId("pending-action-countdown")).toBeVisible();
  expect(listQueries[0]).toContain("sort=-created_at");

  await page.getByTestId("pending-action-confirm").click();
  await expect(dialog).toBeHidden();
  expect(decided).toEqual([`/v1/ai/pending-actions/${ACTION}/confirm`]);
});

test("profile: disconnect a connected app after confirming", async ({
  page,
}) => {
  const api = await mockApi(page);
  let revoked = false;
  const deletes: string[] = [];
  api.extra.push(async ({ method, path, ok, route }) => {
    if (method === "GET" && path === "/v1/oauth/grants") {
      const items = revoked
        ? []
        : [
            {
              uuid: GRANT,
              client_id: "claude-client",
              client_name: "Claude",
              resource: "/mcp/dealer",
              realm: "dealer",
              scopes: ["mcp"],
              organization_uuid: "0b9c4c1e-0000-4000-8000-0000000004a1",
              organization_name: "Acme Bayi",
              organization_type: "dealer",
              created_at: "2026-10-01T09:00:00Z",
              last_used_at: null,
            },
          ];
      await ok({ items, total: items.length, limit: 10, offset: 0 });
      return true;
    }
    if (method === "DELETE" && path === `/v1/oauth/grants/${GRANT}`) {
      deletes.push(path);
      revoked = true;
      await route.fulfill({ status: 204, body: "" });
      return true;
    }
    return false;
  });

  await page.goto(`/t/${SLUG}/profile`);
  const card = page.getByTestId("connected-apps-card");
  await expect(card.getByText("Claude").first()).toBeVisible();
  await card.getByRole("button", { name: "Disconnect" }).first().click();
  expect(deletes).toEqual([]);
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Disconnect" })
    .click();
  await expect(card.getByText("No connected apps")).toBeVisible();
  expect(deletes).toEqual([`/v1/oauth/grants/${GRANT}`]);
});
