// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, Fragment, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";

type AnyRow = Record<string, unknown> & { uuid: string };
type RowAction = { id: string; onSelect: () => void };

const captured = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<AnyRow>>[],
  bulkMenus: [] as Record<string, unknown>[],
  banners: [] as Record<string, unknown>[],
  rowActions: [] as RowAction[][],
  grants: new Set<string>(),
  push: vi.fn(),
  api: {
    list: vi.fn(),
    update: vi.fn(),
    assignees: vi.fn(),
    subjects: vi.fn(),
  },
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: captured.push }),
}));
vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: ReactNode }) =>
    createElement("a", { href }, children),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    format: { dateTime: (v: string) => v, number: (v: number) => String(v) },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => captured.grants.has(p) }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/features/tasks/services/tasks.service", async (orig) => ({
  ...(await orig<object>()),
  tasksService: captured.api,
}));
vi.mock("@/features/bulk-engine", async (orig) => ({
  ...(await orig<object>()),
  BulkActionMenu: (props: Record<string, unknown>) => {
    captured.bulkMenus.push(props);
    return null;
  },
  SelectionBanner: (props: Record<string, unknown>) => {
    captured.banners.push(props);
    return null;
  },
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityRowActions: ({ actions }: { actions: RowAction[] }) => {
    captured.rowActions.push(actions);
    return null;
  },
  // Renders the toolbar and the actions cell of every row.
  EntityTable: (props: Partial<DataTableProps<AnyRow>>) => {
    captured.tables.push(props);
    const actions = (props.columns ?? []).find((c) => c.id === "actions");
    return createElement(
      "div",
      null,
      (props as { toolbarExtra?: ReactNode }).toolbarExtra,
      (props.data ?? []).map((row) =>
        createElement(
          Fragment,
          { key: row.uuid },
          typeof actions?.cell === "function"
            ? (actions.cell as (ctx: unknown) => ReactNode)({
                row: { original: row },
              })
            : null,
        ),
      ),
    );
  },
}));

import { Permission } from "@/config/permissions";
import type { Task } from "@/features/tasks/services/tasks.service";

import {
  TASK_BULK_ACTIONS,
  TASKS_PERSIST_KEY,
  TasksListPage,
} from "./tasks-list-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

function task(patch: Partial<Task> = {}): Task {
  return {
    uuid: "t-1",
    title: "Aylık ziyaret",
    description: "",
    subject_organization: { uuid: "o-1", name: "Kuzey Oto", type: "dealer" },
    assignee: { uuid: "u-1", name: "Ayşe Merkez" },
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

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  captured.tables = [];
  captured.bulkMenus = [];
  captured.banners = [];
  captured.rowActions = [];
  captured.api.list.mockResolvedValue({
    items: [task(), task({ uuid: "t-2", status: "done" })],
    total: 45,
    limit: 20,
    offset: 0,
  });
  captured.api.update.mockImplementation((uuid: string) =>
    Promise.resolve(task({ uuid })),
  );
  captured.api.assignees.mockResolvedValue([
    { uuid: "u-1", name: "Ayşe Merkez" },
    { uuid: "u-2", name: "Can Merkez" },
  ]);
  captured.api.subjects.mockResolvedValue([
    { uuid: "o-1", name: "Kuzey Oto", type: "dealer" },
    { uuid: "o-2", name: "Ege Dağıtım", type: "distributor" },
  ]);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
  captured.grants = new Set();
});

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(TasksListPage, { slug: "olex" }),
      ),
    );
  });
  await flush();
}

const table = () => captured.tables.at(-1)!;
const lastList = () => captured.api.list.mock.calls.at(-1)![0];
const columnIds = () =>
  (table().columns ?? []).map(
    (c: ColumnDef<AnyRow, unknown>) =>
      c.id ?? (c as { accessorKey?: string }).accessorKey,
  );

async function setFilters(value: { id: string; value: unknown }[]) {
  await act(async () => {
    table().state!.onColumnFiltersChange!(value);
  });
  await flush();
}

describe("TasksListPage (TEC-380)", () => {
  it("is forbidden without tasks.read", async () => {
    await render();
    expect(captured.api.list).not.toHaveBeenCalled();
    expect(container.textContent).toContain("tasks.list.forbidden");
  });

  it("opens on active tasks, newest first, as a server table", async () => {
    captured.grants = new Set([Permission.TasksRead]);
    await render();
    expect(lastList()).toEqual({
      status: "active",
      sort: "-created_at",
      limit: 20,
      offset: 0,
    });
    expect(table().rowCount).toBe(45);
    expect(table().features).toMatchObject({
      persistKey: TASKS_PERSIST_KEY,
      rowSelection: false,
      inlineEdit: false,
    });
    // Read-only: no select column, no bulk menu.
    expect(columnIds()).not.toContain("select");
    expect(captured.bulkMenus).toHaveLength(0);
  });

  it("sorts on the backend fields only", async () => {
    captured.grants = new Set([Permission.TasksRead]);
    await render();
    const sortable = (table().columns ?? [])
      .filter((c) => c.enableSorting)
      .map((c) => c.id ?? (c as { accessorKey?: string }).accessorKey);
    expect(sortable).toEqual([
      "title",
      "status",
      "priority",
      "subject",
      "due_at",
      "created_at",
      "updated_at",
    ]);
    for (const [id, sort] of [
      ["due_at", "due_at"],
      ["subject", "-subject"],
      ["priority", "-priority"],
    ] as const) {
      await act(async () => {
        table().state!.onSortingChange!([{ id, desc: sort.startsWith("-") }]);
      });
      await flush();
      expect(lastList().sort).toBe(sort);
    }
  });

  it("maps search and every column filter to the list params", async () => {
    captured.grants = new Set([Permission.TasksRead]);
    await render();
    await act(async () => {
      table().state!.onGlobalFilterChange!("fiyat");
    });
    await setFilters([
      { id: "status", value: ["open", "in_progress"] },
      { id: "priority", value: ["high", "urgent"] },
      { id: "subject", value: ["o-1", "o-2"] },
      { id: "assignee", value: "u-2" },
      { id: "due_at", value: ["2026-10-01", "2026-10-31"] },
      { id: "created_at", value: ["2026-09-01", undefined] },
    ]);
    expect(lastList()).toEqual({
      q: "fiyat",
      status: "open,in_progress",
      priority: "high,urgent",
      subject_organization_uuid: "o-1,o-2",
      assignee_user_uuid: "u-2",
      due_from: "2026-10-01",
      due_to: "2026-10-31",
      created_from: "2026-09-01",
      sort: "-created_at",
      limit: 20,
      offset: 0,
    });
  });

  it("resolves the due presets to due_from / due_to", async () => {
    captured.grants = new Set([Permission.TasksRead]);
    await render();
    await setFilters([
      { id: "status", value: ["active"] },
      { id: "due_at", value: ["2026-10-01", "2026-10-31"] },
    ]);
    await act(async () => {
      (
        container.querySelector("[data-testid=task-due-overdue]") as HTMLElement
      ).click();
    });
    await flush();
    const overdue = lastList();
    expect(overdue.due_to).toEqual(expect.any(String));
    expect(overdue.due_from).toBeUndefined();
    expect(overdue.due_before).toBeUndefined();
    // The preset replaced the typed due range.
    expect(
      table().state!.columnFilters!.find((f) => f.id === "due_at"),
    ).toBeUndefined();

    await act(async () => {
      (
        container.querySelector("[data-testid=task-due-week]") as HTMLElement
      ).click();
    });
    await flush();
    const week = lastList();
    expect(
      new Date(String(week.due_to)).getTime() -
        new Date(String(week.due_from)).getTime(),
    ).toBe(7 * 24 * 60 * 60 * 1000);

    await act(async () => {
      (
        container.querySelector("[data-testid=task-due-all]") as HTMLElement
      ).click();
    });
    await flush();
    expect(lastList().due_to).toBeUndefined();
  });

  it("offers the bulk actions with every matching task", async () => {
    captured.grants = new Set([Permission.TasksRead, Permission.TasksWrite]);
    await render();
    expect(columnIds()[0]).toBe("__select");
    expect(table().features).toMatchObject({
      rowSelection: true,
      inlineEdit: true,
    });
    const menu = captured.bulkMenus.at(-1)!;
    expect(menu.resource).toBe("tasks");
    expect((menu.actions as { id: string }[]).map((a) => a.id)).toEqual([
      "assign",
      "set_status",
      "set_priority",
    ]);
    expect(menu.paramOptions).toEqual({
      assignee_uuid: [
        { value: "u-1", label: "Ayşe Merkez" },
        { value: "u-2", label: "Can Merkez" },
      ],
    });
    expect(
      TASK_BULK_ACTIONS.find((a) => a.id === "set_status")?.params?.[0]
        ?.options,
    ).toEqual(["open", "in_progress", "done", "cancelled"]);

    await setFilters([
      { id: "status", value: ["active"] },
      { id: "priority", value: ["urgent"] },
    ]);
    await act(async () => {
      table().state!.onRowSelectionChange!({ "t-1": true });
    });
    await flush();
    await act(async () => {
      (captured.banners.at(-1)!.onSelectAllMatching as () => void)();
    });
    await flush();
    expect(captured.bulkMenus.at(-1)!.scope).toEqual({
      mode: "all",
      query: { status: "active", priority: "urgent", sort: "-created_at" },
      total: 45,
    });
  });

  it("changes status from the row menu and priority inline", async () => {
    captured.grants = new Set([Permission.TasksRead, Permission.TasksWrite]);
    await render();
    const [openRow, doneRow] = captured.rowActions.slice(-2);
    expect(openRow.map((a) => a.id)).toEqual(["open", "in_progress", "done"]);
    expect(doneRow.map((a) => a.id)).toEqual(["open", "reopen"]);

    await act(async () => {
      openRow.find((a) => a.id === "in_progress")!.onSelect();
    });
    await flush();
    expect(captured.api.update).toHaveBeenCalledWith("t-1", {
      status: "in_progress",
    });

    await act(async () => {
      table().onCellEdit!({
        rowId: "t-2",
        columnId: "priority",
        value: "urgent",
        row: task({ uuid: "t-2" }) as unknown as AnyRow,
      });
    });
    await flush();
    expect(captured.api.update).toHaveBeenLastCalledWith("t-2", {
      priority: "urgent",
    });

    openRow.find((a) => a.id === "open")!.onSelect();
    expect(captured.push).toHaveBeenCalledWith("/t/olex/tasks/t-1");
  });
});
