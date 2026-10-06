import { expect, test } from "@playwright/test";

import { signIn } from "./support/mock-api";
import {
  DEALER,
  mockTasks,
  OTHER_MEMBER,
  TASK_SLUG,
} from "./support/task-mock";

/**
 * TEC-221: center tasks against a mocked BFF — the list with its filters,
 * a new task assigned to a center member, the detail page's status change
 * and the comment stream.
 */

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("tasks: list filters", async ({ page }) => {
  const api = await mockTasks(page);

  await page.goto(`/t/${TASK_SLUG}/tasks`);
  await expect(page.getByRole("heading", { name: "Tasks" })).toBeVisible();
  const rows = page.getByTestId("task-row");
  // Default: open and in progress tasks only (status facet "active").
  await expect(rows).toHaveCount(2);
  await expect(page.getByText("Closed follow-up")).toHaveCount(0);
  expect(api.calls.some((c) => c.includes("status=active"))).toBe(true);
  await expect(page.locator("[data-overdue]")).toHaveCount(1);

  // Priority facet in the table toolbar → priority=urgent.
  await page
    .getByRole("button", { name: "Priority", exact: true })
    .first()
    .click();
  await page.getByRole("option", { name: "Urgent" }).click();
  await page.keyboard.press("Escape");
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText("Monthly visit");
  expect(api.calls.some((c) => c.includes("priority=urgent"))).toBe(true);

  // With a value the toolbar button also shows the chosen badge.
  await page
    .getByRole("button", { name: /^Priority/ })
    .first()
    .click();
  await page.getByRole("option", { name: "Urgent" }).click();
  await page.keyboard.press("Escape");
  await expect(rows).toHaveCount(2);

  // Due presets stay in the toolbar → due_to.
  await page.getByTestId("task-due-overdue").click();
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText("Overdue price review");
  expect(api.calls.some((c) => c.includes("due_to="))).toBe(true);
  await page.getByTestId("task-due-all").click();

  // Sorting is sent to the backend.
  await page
    .getByRole("table")
    .getByRole("button", { name: "Due date" })
    .click();
  await page.getByRole("menuitem", { name: "Sort ascending" }).click();
  await expect
    .poll(() => api.calls.some((c) => c.includes("sort=due_at")))
    .toBe(true);
});

test("tasks: create, change status, comment", async ({ page }) => {
  const api = await mockTasks(page);

  await page.goto(`/t/${TASK_SLUG}/tasks/new`);
  await page.getByTestId("task-title").fill("Call about the new film");
  await page.getByTestId("task-subject").selectOption(DEALER);
  await page.getByTestId("task-assignee").selectOption(OTHER_MEMBER);
  await page.getByTestId("task-priority").selectOption("high");
  await page.getByTestId("task-submit").click();

  await expect(page.getByTestId("task-detail")).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Call about the new film" }),
  ).toBeVisible();
  expect(api.bodies[0]).toEqual({
    subject_organization_uuid: DEALER,
    title: "Call about the new film",
    priority: "high",
    assignee_user_uuid: OTHER_MEMBER,
  });

  await page
    .getByTestId("task-status-actions")
    .locator('[data-status="in_progress"]')
    .click();
  await expect(
    page.getByTestId("task-status-actions").locator('[data-status="open"]'),
  ).toBeVisible();
  expect(api.bodies.at(-1)).toEqual({ status: "in_progress" });

  await page.getByTestId("task-comment-input").fill("Called, waiting.");
  await page.getByTestId("task-comment-submit").click();
  await expect(page.getByTestId("task-comment")).toContainText(
    "Called, waiting.",
  );
});
