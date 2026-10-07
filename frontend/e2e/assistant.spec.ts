import { expect, test, type Route } from "@playwright/test";

import { mockApi, signIn, SLUG } from "./support/mock-api";

/**
 * TEC-390: panel AI assistant against a mocked BFF — guidelines consent,
 * a streamed answer with a tool chip, the confirmation card and its
 * outcome, and the header sheet. The SSE bodies are served whole; the
 * client parses them the same way as a live stream.
 */

const CONV = "0b9c4c1e-0000-4000-8000-000000000390";
const ACTION = "0b9c4c1e-0000-4000-8000-000000000391";
const TASK = "0b9c4c1e-0000-4000-8000-000000000392";

function sse(events: [string, unknown][]) {
  return events
    .map(([e, d]) => `event: ${e}\ndata: ${JSON.stringify(d)}\n\n`)
    .join("");
}

function stream(route: Route, body: string) {
  return route.fulfill({
    status: 200,
    headers: {
      "content-type": "text/event-stream",
      "cache-control": "no-cache",
    },
    body,
  });
}

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("assistant: consent, streamed answer, confirm card", async ({ page }) => {
  const api = await mockApi(page);
  api.permissions.push("ai.use", "ai.actions.confirm");
  api.features.push("ai_assistant");
  let consented = false;
  const confirmBodies: unknown[] = [];
  const conversation = {
    uuid: CONV,
    title: "",
    message_count: 0,
    last_message_at: null,
    created_at: "2026-10-07T10:00:00Z",
    updated_at: "2026-10-07T10:00:00Z",
  };
  let created = false;

  api.extra.push(async ({ method, path, ok, route, body }) => {
    if (method === "GET" && path === "/v1/ai/status") {
      await ok({
        enabled: true,
        allowed: true,
        consent_required: !consented,
        consent: consented
          ? undefined
          : {
              uuid: "0b9c4c1e-0000-4000-8000-000000000393",
              kind: "ai_guidelines",
              locale: "en",
              version: 1,
              body: "## Guidelines\n\n- Do not share personal data.",
              created_at: "2026-10-01T00:00:00Z",
            },
        quota: { period: "2026-10", limit: 1000, used: 400, remaining: 600 },
      });
      return true;
    }
    if (method === "POST" && path === "/v1/consents") {
      consented = true;
      await ok({}, 201);
      return true;
    }
    if (method === "GET" && path === "/v1/ai/conversations") {
      const items = created ? [conversation] : [];
      await ok({ items, total: items.length, limit: 50, offset: 0 });
      return true;
    }
    if (method === "POST" && path === "/v1/ai/conversations") {
      created = true;
      await ok(conversation, 201);
      return true;
    }
    if (method === "POST" && path === `/v1/ai/conversations/${CONV}/messages`) {
      conversation.title = "Open tasks";
      await stream(
        route,
        sse([
          ["message_start", { conversation_uuid: CONV, message_uuid: "m1" }],
          ["tool_start", { id: "t1", name: "my_tasks" }],
          ["tool_result", { id: "t1", name: "my_tasks", ok: true }],
          ["text_delta", { text: "You have **no** open tasks. " }],
          ["text_delta", { text: "Shall I create one?" }],
          [
            "confirm",
            {
              action_uuid: ACTION,
              tool_use_id: "tu1",
              tool_name: "create_task",
              source: "panel",
              status: "pending",
              preview: {
                action: "create_task",
                summary: "Create task",
                fields: [{ key: "title", value: "Call the customer" }],
                edit: [
                  {
                    key: "title",
                    type: "text",
                    value: "Call the customer",
                    required: true,
                  },
                ],
              },
              expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
              created_at: new Date().toISOString(),
            },
          ],
          ["title", { conversation_uuid: CONV, title: "Open tasks" }],
        ]),
      );
      return true;
    }
    if (method === "POST" && path === `/v1/ai/actions/${ACTION}/confirm`) {
      confirmBodies.push(body);
      await stream(
        route,
        sse([
          ["message_start", { conversation_uuid: CONV, message_uuid: "m2" }],
          [
            "action",
            {
              action_uuid: ACTION,
              tool_use_id: "tu1",
              tool_name: "create_task",
              source: "panel",
              status: "confirmed",
              link: { kind: "task", uuid: TASK },
            },
          ],
          ["text_delta", { text: "The task is created." }],
          ["message_done", { message_uuid: "m2", status: "complete" }],
        ]),
      );
      return true;
    }
    return false;
  });

  await page.goto(`/t/${SLUG}/assistant`);
  await expect(
    page.getByRole("heading", { name: "AI assistant" }),
  ).toBeVisible();

  // K22: the box starts unchecked; Accept stays disabled until checked.
  const dialog = page.getByTestId("ai-assistant-consent");
  await expect(dialog).toContainText("Do not share personal data.");
  const accept = page.getByTestId("ai-assistant-consent-accept");
  await expect(accept).toBeDisabled();
  await expect(page.getByTestId("ai-input")).toBeDisabled();
  await dialog.getByRole("checkbox").click();
  await accept.click();
  await expect(dialog).toBeHidden();
  await expect(page.getByTestId("ai-input")).toBeEnabled();
  await expect(page.getByTestId("ai-quota")).toContainText("60%");

  await page.getByTestId("ai-input").fill("Do I have open tasks?");
  await page.getByTestId("ai-input").press("Enter");

  const answer = page.getByTestId("ai-message-assistant").first();
  await expect(answer).toContainText(
    "You have no open tasks. Shall I create one?",
  );
  await expect(answer.getByTestId("ai-text")).toHaveCount(1);
  await expect(answer.getByTestId("ai-tool-chip")).toContainText(
    "Searching tasks",
  );
  const card = page.getByTestId("ai-confirm-card");
  await expect(card).toContainText("Create task");
  await expect(page.getByTestId("ai-card-countdown")).toBeVisible();
  // The title event renames the conversation in the history column.
  await expect(page.locator("aside").getByText("Open tasks")).toBeVisible();

  await card.getByLabel(/Title/).fill("Call the customer tomorrow");
  await page.getByTestId("ai-card-confirm").click();
  const outcome = page.getByTestId("ai-action-outcome");
  await expect(outcome).toContainText("Done.");
  await expect(outcome.getByRole("link")).toHaveAttribute(
    "href",
    `/t/${SLUG}/tasks/${TASK}`,
  );
  expect(confirmBodies).toEqual([
    { edits: { title: "Call the customer tomorrow" } },
  ]);
  await expect(page.getByTestId("ai-card-confirm")).toHaveCount(0);
});

test("assistant: header sheet and module gate", async ({ page }) => {
  const api = await mockApi(page);
  api.permissions.push("ai.use");
  api.extra.push(async ({ method, path, ok }) => {
    if (method === "GET" && path === "/v1/ai/status") {
      await ok({
        enabled: true,
        allowed: true,
        consent_required: false,
        quota: { period: "2026-10", limit: 0, used: 0, remaining: null },
      });
      return true;
    }
    if (method === "GET" && path === "/v1/ai/conversations") {
      await ok({ items: [], total: 0, limit: 50, offset: 0 });
      return true;
    }
    return false;
  });

  // Module off: no header button and no menu entry.
  await page.goto(`/t/${SLUG}/profile`);
  await expect(page.getByRole("link", { name: "Overview" })).toBeVisible();
  await expect(page.getByTestId("ai-header-button")).toHaveCount(0);
  await expect(page.getByRole("link", { name: "AI assistant" })).toHaveCount(0);

  api.features.push("ai_assistant");
  await page.reload();
  await expect(page.getByRole("link", { name: "AI assistant" })).toBeVisible();
  await page.getByTestId("ai-header-button").click();
  await expect(page.getByRole("dialog")).toContainText(
    "Ask a question to start a conversation.",
  );
  await expect(page.getByRole("dialog").getByTestId("ai-input")).toBeEnabled();
});
