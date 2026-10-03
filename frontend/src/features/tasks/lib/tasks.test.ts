import { describe, expect, it } from "vitest";

import type { Task } from "@/features/tasks/services/tasks.service";

import {
  buildCreateBody,
  buildUpdateBody,
  dueRange,
  emptyTaskForm,
  fromLocalInput,
  isOverdue,
  listQuery,
  statusTargets,
  taskFormOf,
  toLocalInput,
  validateTaskForm,
} from "./tasks";

function task(patch: Partial<Task> = {}): Task {
  return {
    uuid: "t-1",
    title: "Ziyaret",
    description: "",
    subject_organization: { uuid: "o-1", name: "Kuzey Oto", type: "dealer" },
    assignee: null,
    created_by: null,
    priority: "normal",
    status: "open",
    source: "manual",
    due_at: null,
    closed_at: null,
    comment_count: 0,
    created_at: "2026-10-01T10:00:00Z",
    updated_at: "2026-10-01T10:00:00Z",
    ...patch,
  };
}

describe("tasks lib", () => {
  const now = new Date("2026-10-03T12:00:00Z");

  it("flags open tasks past their due date only", () => {
    expect(isOverdue(task({ due_at: "2026-10-03T11:00:00Z" }), now)).toBe(true);
    expect(isOverdue(task({ due_at: "2026-10-04T11:00:00Z" }), now)).toBe(
      false,
    );
    expect(
      isOverdue(task({ due_at: "2026-10-03T11:00:00Z", status: "done" }), now),
    ).toBe(false);
    expect(isOverdue(task(), now)).toBe(false);
  });

  it("resolves the due presets", () => {
    expect(dueRange("overdue", now)).toEqual({
      due_before: now.toISOString(),
    });
    const today = dueRange("today", now);
    expect(
      new Date(today.due_before!).getTime() -
        new Date(today.due_after!).getTime(),
    ).toBeGreaterThanOrEqual(23 * 3600 * 1000);
    expect(new Date(today.due_after!).getTime()).toBeLessThanOrEqual(
      now.getTime(),
    );
  });

  it("builds the list query from the filters", () => {
    expect(
      listQuery(
        { status: "", priority: "", assignee: "", subject: "", due: "" },
        2,
        now,
      ),
    ).toEqual({ limit: 20, offset: 40 });
  });

  it("round-trips the datetime-local value", () => {
    const iso = new Date("2026-10-31T17:00").toISOString();
    expect(toLocalInput(iso)).toBe("2026-10-31T17:00");
    expect(fromLocalInput("2026-10-31T17:00")).toBe(iso);
    expect(fromLocalInput("  ")).toBeNull();
    expect(toLocalInput(null)).toBe("");
  });

  it("validates the form", () => {
    expect(validateTaskForm(emptyTaskForm())).toEqual({
      title: "tasks.form.errors.title_required",
      subject_organization_uuid: "tasks.form.errors.subject_required",
    });
    expect(
      validateTaskForm({
        ...emptyTaskForm(),
        title: "x".repeat(201),
        subjectUuid: "o-1",
      }),
    ).toEqual({ title: "tasks.form.errors.title_too_long" });
  });

  it("creates without empty optional fields", () => {
    expect(
      buildCreateBody({ ...emptyTaskForm(), title: " A ", subjectUuid: "o-1" }),
    ).toEqual({
      subject_organization_uuid: "o-1",
      title: "A",
      priority: "normal",
    });
  });

  it("patches only what changed and clears with null", () => {
    const t = task({
      assignee: { uuid: "u-1", name: "Ayşe" },
      due_at: "2026-10-31T14:00:00Z",
    });
    expect(buildUpdateBody(t, taskFormOf(t))).toEqual({});
    expect(
      buildUpdateBody(t, { ...taskFormOf(t), assigneeUuid: "", dueLocal: "" }),
    ).toEqual({ assignee_user_uuid: null, due_at: null });
  });

  it("offers every other status", () => {
    expect(statusTargets(task({ status: "done" }))).toEqual([
      "open",
      "in_progress",
      "cancelled",
    ]);
  });
});
