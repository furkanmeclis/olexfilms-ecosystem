import { expect, test } from "@playwright/test";

import {
  F4,
  installF4,
  installF4Platform,
  installF4Portal,
} from "./support/f4-mock";
import { ORG, SLUG, signIn } from "./support/mock-api";

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("F4 gate: assistant consent streams an appointment confirmation card", async ({
  page,
}) => {
  const f4 = await installF4(page);

  await page.goto(`/t/${SLUG}/assistant`);
  const dialog = page.getByTestId("ai-assistant-consent");
  await expect(dialog).toContainText("Check before creating records.");
  await expect(page.getByTestId("ai-assistant-consent-accept")).toBeDisabled();
  await dialog.getByRole("checkbox").click();
  await page.getByTestId("ai-assistant-consent-accept").click();

  await page.getByTestId("ai-input").fill("Yarın randevu aç");
  await page.getByTestId("ai-input").press("Enter");
  const answer = page.getByTestId("ai-message-assistant").first();
  await expect(answer).toContainText("Randevu oluşturayım mı?");
  const card = page.getByTestId("ai-confirm-card");
  await expect(card).toContainText("Create appointment");
  await page.getByTestId("ai-card-confirm").click();
  await expect(page.getByTestId("ai-action-outcome")).toContainText("Done.");
  expect(
    f4.bodies[`POST /v1/ai/actions/${F4.assistantAction}/confirm`],
  ).toHaveLength(1);
});

test("F4 gate: quota exceeded org shows the assistant band", async ({
  page,
}) => {
  const f4 = await installF4(page);
  f4.consented = true;
  f4.quotaExceeded = true;

  await page.goto(`/t/${SLUG}/assistant`);
  await expect(page.getByTestId("ai-quota-band")).toBeVisible();
  await expect(page.getByTestId("ai-input")).toBeDisabled();
});

test("F4 gate: platform admin handles WhatsApp conversation and handoff", async ({
  page,
}) => {
  const state = await installF4Platform(page);

  await page.goto("/platform/conversations");
  await expect(
    page.getByRole("heading", { name: "Conversations" }),
  ).toBeVisible();
  await page
    .getByRole("row", { name: /Portal Ziyaretçisi/ })
    .getByText("Portal Ziyaretçisi")
    .click();
  const panel = page.getByTestId("conversation-panel");
  await expect(
    panel.locator('[data-testid="message"][data-sender="contact"]'),
  ).toContainText("Garantim");
  await expect(
    panel.locator('[data-testid="message"][data-sender="ai"]'),
  ).toContainText("Garanti bilgilerinizi");

  await panel.getByTestId("attachment-input").setInputFiles({
    name: "garanti.pdf",
    mimeType: "application/pdf",
    buffer: Buffer.from("%PDF-1.4\n"),
  });
  await expect(panel.getByTestId("attachment-name")).toContainText(
    "garanti.pdf",
  );
  await panel
    .getByTestId("composer-input")
    .fill("PDF'i ekledim, bayi sizi arayacak.");
  await panel.getByTestId("composer-send").click();
  await expect(
    panel.locator('[data-testid="message"][data-sender="staff"]'),
  ).toContainText("PDF'i ekledim");
  await expect(panel.getByTestId("ai-paused-notice")).toContainText("30");

  await panel.getByRole("button", { name: /Hand off|Bayiye devret/ }).click();
  await page.getByRole("combobox").fill("Acme");
  await page.getByRole("option", { name: "Acme Bayi" }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: /Hand off|Devret/ })
    .click();
  await expect(panel).toContainText("Acme Bayi");
  expect(state.bodies.patch).toEqual({ assigned_org_uuid: ORG });
});

test("F4 gate: OAuth consent redirects and MCP pending action is approved", async ({
  page,
}) => {
  await installF4(page);

  await page.goto(`/oauth/consent?request=${F4.mcpRequest}`);
  await expect(page.getByTestId("oauth-consent")).toBeVisible();
  await page.getByRole("radio", { name: /Acme Bayi/ }).click();
  await page.getByTestId("oauth-consent-approve").click();
  await page.waitForURL(/oauth-consent-done\?code=f4-code/);

  await page.goto(`/t/${SLUG}/assistant/approvals?action=${F4.mcpAction}`);
  const dialog = page.getByTestId("pending-action-dialog");
  await expect(dialog).toContainText("MCP Ayşe");
  await page.getByTestId("pending-action-confirm").click();
  await expect(dialog).toBeHidden();
});

test("F4 gate: dealer campaign localization blocks submit until distributor approval", async ({
  page,
}) => {
  const f4 = await installF4(page);

  await page.goto(`/t/${SLUG}/campaigns/new`);
  await page.locator("#campaign-name").fill("Sonbahar bakım kampanyası");
  await page.getByTestId("campaign-channel-whatsapp").click();
  await expect(page.getByTestId("campaign-preview-total")).toContainText("24");
  await expect(page.getByTestId("campaign-missing-locales")).toContainText(
    "tr",
  );
  await expect(page.getByTestId("campaign-missing-locales")).toContainText(
    "de",
  );
  await expect(page.getByTestId("campaign-submit")).toBeDisabled();

  await page.locator("#campaign-title-tr").fill("Sonbahar bakımı");
  await page.locator("#campaign-body-tr").fill("Aracınız için bakım zamanı.");
  await page.getByTestId("campaign-locale-tab-de").click();
  await expect(page.getByTestId("campaign-submit")).toBeDisabled();
  await page.locator("#campaign-title-de").fill("Herbstpflege");
  await page.locator("#campaign-body-de").fill("Zeit für die Fahrzeugpflege.");
  await expect(page.getByTestId("campaign-submit")).toBeEnabled();
  await page.getByTestId("campaign-submit").click();
  await expect(page).toHaveURL(
    new RegExp(`/t/${SLUG}/campaigns/${F4.campaign}$`),
  );

  f4.makeDistributorUser();
  await page.goto("/t/dist/campaigns/approvals");
  await page.getByRole("row", { name: /Sonbahar bakım kampanyası/ }).click();
  await page.getByTestId("campaign-decision-submit").click();
  await expect(page.getByTestId("campaign-approval-dialog")).toBeHidden();

  await page.goto(`/t/dist/campaigns/${F4.campaign}`);
  await expect(page.getByText("Approved")).toBeVisible();
  await expect(page.getByText(/Submitted|Gönderildi/)).toBeVisible();
  await expect(page.getByText(/Approved|Onaylandı/)).toBeVisible();
});

test("F4 gate: portal customer asks warranty and marketing opt-in is off", async ({
  page,
}) => {
  await installF4Portal(page);

  await page.goto("/portal/assistant");
  await page.getByTestId("ai-input").fill("garantim devam ediyor mu?");
  await page.getByTestId("ai-input").press("Enter");
  await expect(page.getByTestId("ai-message-assistant").first()).toContainText(
    "Garantiniz aktif",
  );

  await page.goto("/portal/preferences");
  await expect(
    page.getByTestId("portal-pref-campaign_marketing_enabled"),
  ).toHaveAttribute("aria-checked", "false");
});
