import type {
  Task,
  TaskCreateInput,
  TaskPriority,
  TaskStatus,
  TaskUpdateInput,
} from "@/features/tasks/services/tasks.service";
import { isApiError } from "@/lib/api";

type T = (key: string, params?: Record<string, string | number>) => string;

/** Every task status in flow order (TaskStatus). */
export const TASK_STATUSES: TaskStatus[] = [
  "open",
  "in_progress",
  "done",
  "cancelled",
];

/** Status filter values: "active" is open + in_progress (CSV `status`). */
export const TASK_STATUS_FILTERS: (TaskStatus | "active")[] = [
  "active",
  ...TASK_STATUSES,
];

/** Priorities from low to urgent (TaskPriority). */
export const TASK_PRIORITIES: TaskPriority[] = [
  "low",
  "normal",
  "high",
  "urgent",
];

/** Due date filter presets of the list page. */
export type DueFilter = "overdue" | "today" | "week";
export const DUE_FILTERS: DueFilter[] = ["overdue", "today", "week"];

export const TITLE_MAX = 200;
export const DESCRIPTION_MAX = 10000;
export const COMMENT_MAX = 4000;

/** Status chip tone. */
export function taskStatusTone(
  status: TaskStatus,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "done":
      return "success";
    case "in_progress":
      return "warning";
    case "cancelled":
      return "danger";
    default:
      return "default";
  }
}

/** Priority chip tone. */
export function taskPriorityTone(
  priority: TaskPriority,
): "default" | "success" | "warning" | "danger" {
  switch (priority) {
    case "urgent":
      return "danger";
    case "high":
      return "warning";
    default:
      return "default";
  }
}

/** An open task past its due date. */
export function isOverdue(task: Task, now: Date = new Date()): boolean {
  if (!task.due_at || task.status === "done" || task.status === "cancelled") {
    return false;
  }
  return new Date(task.due_at).getTime() <= now.getTime();
}

/**
 * due_from / due_to (RFC3339) of a preset: overdue is "due before now"
 * (with the active status filter the page shows open tasks only), today
 * the local calendar day, week the next seven days from now.
 */
export function dueRange(
  filter: DueFilter,
  now: Date = new Date(),
): { due_from?: string; due_to?: string } {
  switch (filter) {
    case "overdue":
      return { due_to: now.toISOString() };
    case "today": {
      const start = new Date(now);
      start.setHours(0, 0, 0, 0);
      const end = new Date(start);
      end.setDate(end.getDate() + 1);
      return { due_from: start.toISOString(), due_to: end.toISOString() };
    }
    case "week": {
      const end = new Date(now.getTime() + 7 * 24 * 60 * 60 * 1000);
      return { due_from: now.toISOString(), due_to: end.toISOString() };
    }
  }
}

function pad(n: number): string {
  return String(n).padStart(2, "0");
}

/** ISO instant -> `datetime-local` input value (local time), "" for none. */
export function toLocalInput(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(
    d.getHours(),
  )}:${pad(d.getMinutes())}`;
}

/** `datetime-local` input value -> ISO instant, null for blank or invalid. */
export function fromLocalInput(value: string): string | null {
  const v = value.trim();
  if (v === "") return null;
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? null : d.toISOString();
}

/** Values of the task form (create and edit). */
export type TaskFormValues = {
  title: string;
  description: string;
  subjectUuid: string;
  assigneeUuid: string;
  priority: TaskPriority;
  /** `datetime-local` value; "" for no due date. */
  dueLocal: string;
};

export function emptyTaskForm(): TaskFormValues {
  return {
    title: "",
    description: "",
    subjectUuid: "",
    assigneeUuid: "",
    priority: "normal",
    dueLocal: "",
  };
}

export function taskFormOf(task: Task): TaskFormValues {
  return {
    title: task.title,
    description: task.description,
    subjectUuid: task.subject_organization.uuid,
    assigneeUuid: task.assignee?.uuid ?? "",
    priority: task.priority,
    dueLocal: toLocalInput(task.due_at),
  };
}

/** Field errors of the form (i18n keys), empty when valid. */
export function validateTaskForm(v: TaskFormValues): Record<string, string> {
  const errors: Record<string, string> = {};
  const title = v.title.trim();
  if (title === "") errors.title = "tasks.form.errors.title_required";
  else if ([...title].length > TITLE_MAX)
    errors.title = "tasks.form.errors.title_too_long";
  if ([...v.description.trim()].length > DESCRIPTION_MAX)
    errors.description = "tasks.form.errors.description_too_long";
  if (v.subjectUuid === "")
    errors.subject_organization_uuid = "tasks.form.errors.subject_required";
  if (v.dueLocal.trim() !== "" && fromLocalInput(v.dueLocal) === null)
    errors.due_at = "tasks.form.errors.due_invalid";
  return errors;
}

/** POST /v1/tasks body. */
export function buildCreateBody(v: TaskFormValues): TaskCreateInput {
  const body: TaskCreateInput = {
    subject_organization_uuid: v.subjectUuid,
    title: v.title.trim(),
    priority: v.priority,
  };
  const desc = v.description.trim();
  if (desc !== "") body.description = desc;
  if (v.assigneeUuid !== "") body.assignee_user_uuid = v.assigneeUuid;
  const due = fromLocalInput(v.dueLocal);
  if (due) body.due_at = due;
  return body;
}

/**
 * PATCH /v1/tasks/{uuid} body with the changed fields only; null clears the
 * assignee or the due date. Empty when nothing changed.
 */
export function buildUpdateBody(
  task: Task,
  v: TaskFormValues,
): TaskUpdateInput {
  const body: TaskUpdateInput = {};
  const title = v.title.trim();
  if (title !== task.title) body.title = title;
  const desc = v.description.trim();
  if (desc !== task.description) body.description = desc;
  if (v.subjectUuid !== task.subject_organization.uuid)
    body.subject_organization_uuid = v.subjectUuid;
  if (v.priority !== task.priority) body.priority = v.priority;
  const assignee = v.assigneeUuid === "" ? null : v.assigneeUuid;
  if (assignee !== (task.assignee?.uuid ?? null))
    body.assignee_user_uuid = assignee;
  const due = fromLocalInput(v.dueLocal);
  const was = task.due_at ? new Date(task.due_at).toISOString() : null;
  // The input has minute precision: compare at that precision.
  if (toLocalInput(due) !== toLocalInput(was)) body.due_at = due;
  return body;
}

/** Status buttons of the detail page: every status but the current one. */
export function statusTargets(task: Task): TaskStatus[] {
  return TASK_STATUSES.filter((s) => s !== task.status);
}

/** Validation detail field -> message key, for server side errors. */
export function serverFieldErrors(err: unknown): Record<string, string> {
  const out: Record<string, string> = {};
  if (!isApiError(err)) return out;
  for (const d of err.details ?? []) {
    if (d.field && d.message) out[d.field] = d.message;
  }
  return out;
}

/** Message for a failed task call: tasks.errors.<CODE> or the fallback. */
export function taskErrorMessage(err: unknown, t: T, fallback: string): string {
  if (!isApiError(err)) return fallback;
  const key = `tasks.errors.${err.code}`;
  const text = t(key);
  return text !== key ? text : fallback;
}
