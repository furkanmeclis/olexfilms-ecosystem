// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  comments: vi.fn(),
  addComment: vi.fn(),
  assignees: vi.fn(),
  subjects: vi.fn(),
}));
const state = vi.hoisted(() => ({ grants: new Set<string>() }));
const router = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string;
    children: unknown;
  } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));
vi.mock("next/navigation", () => ({ useRouter: () => router }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/features/tasks/services/tasks.service", async (orig) => ({
  ...(await orig<object>()),
  tasksService: api,
}));

import { Permission } from "@/config/permissions";
import type { Task } from "@/features/tasks/services/tasks.service";

import { TaskDetailPage } from "./task-detail-page";
import { TaskFormPage } from "./task-form-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  api.assignees.mockResolvedValue([
    { uuid: "u-1", name: "Ayşe Merkez" },
    { uuid: "u-2", name: "Can Merkez" },
  ]);
  api.subjects.mockResolvedValue([
    { uuid: "o-1", name: "Kuzey Oto", type: "dealer" },
    { uuid: "o-2", name: "Ege Dağıtım", type: "distributor" },
  ]);
  api.comments.mockResolvedValue({
    items: [],
    total: 0,
    limit: 200,
    offset: 0,
  });
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
  state.grants = new Set();
});

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render(node: ReturnType<typeof createElement>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await flush();
}

function task(patch: Partial<Task> = {}): Task {
  return {
    uuid: "t-1",
    title: "Aylık ziyaret",
    description: "Stok ve fiyatları konuş",
    subject_organization: { uuid: "o-1", name: "Kuzey Oto", type: "dealer" },
    assignee: { uuid: "u-1", name: "Ayşe Merkez" },
    created_by: { uuid: "u-2", name: "Can Merkez" },
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

const q = <T extends Element>(sel: string) =>
  container.querySelector(sel) as T | null;

async function choose(sel: string, value: string) {
  const el = q<HTMLSelectElement>(sel)!;
  await act(async () => {
    el.value = value;
    el.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await flush();
}

async function type(sel: string, value: string) {
  const el = q<HTMLInputElement | HTMLTextAreaElement>(sel)!;
  const proto =
    el instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value")!.set!;
  await act(async () => {
    setter.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await flush();
}

async function click(el: Element | null) {
  await act(async () => {
    (el as HTMLElement).click();
  });
  await flush();
}

describe("TaskFormPage", () => {
  it("validates, then creates the task and opens it", async () => {
    state.grants = new Set([Permission.TasksRead, Permission.TasksWrite]);
    api.create.mockResolvedValue(task({ uuid: "t-9" }));
    await render(createElement(TaskFormPage, { slug: "olex" }));

    await click(q("[data-testid=task-submit]"));
    expect(api.create).not.toHaveBeenCalled();
    expect(container.textContent).toContain("tasks.form.errors.title_required");
    expect(container.textContent).toContain(
      "tasks.form.errors.subject_required",
    );

    await type("[data-testid=task-title]", "  Ziyaret  ");
    await choose("[data-testid=task-subject]", "o-2");
    await choose("[data-testid=task-assignee]", "u-1");
    await choose("[data-testid=task-priority]", "urgent");
    await type("[data-testid=task-due]", "2026-10-31T17:00");
    await click(q("[data-testid=task-submit]"));

    expect(api.create).toHaveBeenCalledWith({
      subject_organization_uuid: "o-2",
      title: "Ziyaret",
      priority: "urgent",
      assignee_user_uuid: "u-1",
      due_at: new Date("2026-10-31T17:00").toISOString(),
    });
    expect(router.push).toHaveBeenCalledWith("/t/olex/tasks/t-9");
  });
});

describe("TaskDetailPage", () => {
  it("is read-only without tasks.write", async () => {
    state.grants = new Set([Permission.TasksRead]);
    api.get.mockResolvedValue(task());
    await render(createElement(TaskDetailPage, { slug: "olex", uuid: "t-1" }));
    expect(q("[data-testid=task-detail]")).not.toBeNull();
    expect(q("[data-testid=task-edit-form]")).toBeNull();
    expect(q("[data-testid=task-status-actions]")).toBeNull();
    expect(q("[data-testid=task-comment-input]")).toBeNull();
  });

  it("saves only the changed fields and clears the assignee", async () => {
    state.grants = new Set([Permission.TasksRead, Permission.TasksWrite]);
    api.get.mockResolvedValue(task());
    api.update.mockImplementation((_uuid: string, body: object) =>
      Promise.resolve(task({ ...body, assignee: null })),
    );
    await render(createElement(TaskDetailPage, { slug: "olex", uuid: "t-1" }));

    const save = q<HTMLButtonElement>("[data-testid=task-save]")!;
    expect(save.disabled).toBe(true);
    expect(q<HTMLInputElement>("[data-testid=task-title]")!.value).toBe(
      "Aylık ziyaret",
    );

    await type("[data-testid=task-title]", "Aylık ziyaret (Ekim)");
    await choose("[data-testid=task-assignee]", "");
    await choose("[data-testid=task-priority]", "high");
    await click(q("[data-testid=task-save]"));

    expect(api.update).toHaveBeenCalledWith("t-1", {
      title: "Aylık ziyaret (Ekim)",
      priority: "high",
      assignee_user_uuid: null,
    });
  });

  it("changes the status and adds a comment", async () => {
    state.grants = new Set([Permission.TasksRead, Permission.TasksWrite]);
    api.get.mockResolvedValue(task());
    api.update.mockResolvedValue(task({ status: "in_progress" }));
    api.addComment.mockResolvedValue({
      uuid: "c-1",
      body: "Arandı",
      author: { uuid: "u-1", name: "Ayşe Merkez" },
      created_at: "2026-10-02T10:00:00Z",
    });
    await render(createElement(TaskDetailPage, { slug: "olex", uuid: "t-1" }));

    const statuses = [
      ...container.querySelectorAll("[data-testid=task-status-actions] button"),
    ].map((b) => b.getAttribute("data-status"));
    expect(statuses).toEqual(["in_progress", "done", "cancelled"]);
    await click(q('[data-status="in_progress"]'));
    expect(api.update).toHaveBeenCalledWith("t-1", { status: "in_progress" });

    expect(q("[data-testid=task-comments-empty]")).not.toBeNull();
    await type("[data-testid=task-comment-input]", "  Arandı ");
    await click(q("[data-testid=task-comment-submit]"));
    expect(api.addComment).toHaveBeenCalledWith("t-1", "Arandı");
  });
});
