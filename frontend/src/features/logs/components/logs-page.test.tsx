// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ComponentType } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type { ListLogsParams } from "@/features/logs/services/logs.service";

type AnyRow = { uuid: string };

const captured = vi.hoisted(() => ({
  tables: {} as Record<string, Partial<DataTableProps<AnyRow>>>,
  listParams: [] as ListLogsParams[],
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: { dateTime: (v: string) => v, date: (v: string) => v },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/providers/dialog-provider", () => ({
  useDialogs: () => ({ confirmDelete: vi.fn(async () => false) }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: unknown }) => children,
  EntityCreateButton: () => null,
  EntityToolbar: () => null,
  EntityTable: (props: Partial<DataTableProps<AnyRow>>) => {
    captured.tables[props.features?.persistKey ?? "?"] = props;
    return null;
  },
}));
vi.mock("@/features/logs/components/log-detail-drawer", () => ({
  LogDetailDrawer: () => null,
}));
vi.mock("@/features/logs/components/purge-rule-form-drawer", () => ({
  PurgeRuleFormDrawer: () => null,
}));
vi.mock("@/features/logs/hooks/use-log-mutations", () => {
  const mutation = () => ({ mutateAsync: vi.fn(), isPending: false });
  return {
    useDeleteLog: mutation,
    useCreatePurgeRule: mutation,
    useUpdatePurgeRule: mutation,
    useDeletePurgeRule: mutation,
    useRunPurgeRule: mutation,
  };
});
vi.mock("@/features/logs/services/logs.service", () => ({
  logsService: {
    list: vi.fn(async (params: ListLogsParams) => {
      captured.listParams.push(params);
      return { items: [], total: 120, limit: 20, offset: 0 };
    }),
    meta: vi.fn(async () => ({ default_sort: "-created_at" })),
    sources: vi.fn(async () => ({ items: ["api", "worker"] })),
    stats: vi.fn(async () => ({ error: 0, warn: 0, debug: 0 })),
    listRules: vi.fn(async () => ({
      items: [
        { uuid: "r1", name: "Debug", levels: ["debug"], enabled: true },
        { uuid: "r2", name: "Errors", levels: ["error"], enabled: false },
      ],
    })),
  },
}));

import { LogsPage } from "./logs-page";
import { PurgeRulesPanel } from "./purge-rules-panel";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = {};
  captured.listParams = [];
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function flush() {
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render(component: ComponentType) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(QueryClientProvider, { client }, createElement(component)),
    );
  });
  await flush();
}

const lastParams = () => captured.listParams[captured.listParams.length - 1];
const logsTable = () => captured.tables["platform-logs-v1"]!;
const column = (table: Partial<DataTableProps<AnyRow>>, id: string) =>
  (table.columns as ColumnDef<AnyRow, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("LogsPage list", () => {
  it("maps level (CSV), source and created range from column filters", async () => {
    await render(LogsPage);
    expect(lastParams()).toEqual({ limit: 20, offset: 0, sort: "-created_at" });
    expect(column(logsTable(), "source")?.meta?.filterOptions).toEqual([
      { value: "api", label: "api" },
      { value: "worker", label: "worker" },
    ]);

    await act(async () => {
      logsTable().state?.onColumnFiltersChange?.([
        { id: "level", value: ["warn", "error"] },
        { id: "source", value: "worker" },
        { id: "created_at", value: ["2026-10-01", "2026-10-05"] },
      ]);
    });
    await flush();

    expect(lastParams()).toMatchObject({
      level: "warn,error",
      source: "worker",
      created_from: "2026-10-01",
      created_to: "2026-10-05",
    });
    expect(logsTable().rowCount).toBe(120);
  });

  it("sorts on created_at/level/source but never on message", async () => {
    await render(LogsPage);
    expect(column(logsTable(), "message")?.enableSorting).toBe(false);
    for (const id of ["level", "source", "created_at"]) {
      expect(column(logsTable(), id)?.enableSorting, id).toBe(true);
    }
    await act(async () => {
      logsTable().state?.onSortingChange?.([{ id: "level", desc: true }]);
    });
    await flush();
    expect(lastParams().sort).toBe("-level");
  });
});

describe("PurgeRulesPanel", () => {
  it("is a client-side table over the full rule array", async () => {
    await render(PurgeRulesPanel);
    const table = captured.tables["platform-log-rules-v1"]!;
    expect(table.manual).toEqual({
      sorting: false,
      filtering: false,
      pagination: false,
    });
    expect(table.state).toBeUndefined();
    expect((table.data as AnyRow[]).map((rule) => rule.uuid)).toEqual([
      "r1",
      "r2",
    ]);
    expect(column(table, "enabled")?.meta?.filterVariant).toBe("boolean");
  });
});
