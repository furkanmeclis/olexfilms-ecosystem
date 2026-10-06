// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";

type AnyRow = { uuid: string };
type Params = Record<string, unknown>;

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<AnyRow>>,
  activity: [] as Params[],
  exports: [] as { params: Params; scope: string }[],
  imports: [] as { params: Params; scope: string }[],
  ioQuery: undefined as undefined | Record<string, string | undefined>,
  actorFilter: undefined as
    undefined | { value: string; onValueChange: (uuid: string) => void },
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
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
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
// No `importOriginal` spread: the real entity barrel pulls in the io pages
// before this mock is ready, so they would keep the real EntityTable.
vi.mock("@/components/entity", async () => ({
  useServerListState: (
    await import("@/components/entity/use-server-list-state")
  ).useServerListState,
  EntityPage: ({ children }: { children: unknown }) => children,
  EntityToolbar: () => null,
  EntityTable: (
    props: Partial<DataTableProps<AnyRow>> & { toolbarExtra?: unknown },
  ) => {
    captured.table = props;
    return props.toolbarExtra ?? null;
  },
}));
vi.mock("@/features/io/components/resource-io-toolbar", () => ({
  ResourceIOToolbar: (props: {
    query?: Record<string, string | undefined>;
  }) => {
    captured.ioQuery = props.query;
    return null;
  },
}));
vi.mock("@/features/users/components/user-filter-combobox", () => ({
  UserFilterCombobox: (props: {
    value: string;
    onValueChange: (uuid: string) => void;
  }) => {
    captured.actorFilter = props;
    return null;
  },
}));
vi.mock("@/features/io/services/activity.service", () => ({
  activityService: {
    meta: vi.fn(async () => ({
      default_sort: "-created_at",
      capabilities: { export: true },
    })),
    list: vi.fn(async (params: Params) => {
      captured.activity.push(params);
      return { items: [], total: 7, limit: 20, offset: 0 };
    }),
  },
}));
vi.mock("@/features/io/services/exports.service", () => ({
  exportsService: {
    list: vi.fn(async (params: Params, scope: string) => {
      captured.exports.push({ params, scope });
      return { items: [], total: 3, limit: 20, offset: 0 };
    }),
    download: vi.fn(),
  },
}));
vi.mock("@/features/io/services/imports.service", () => ({
  importsService: {
    list: vi.fn(async (params: Params, scope: string) => {
      captured.imports.push({ params, scope });
      return { items: [], total: 4, limit: 20, offset: 0 };
    }),
    rollback: vi.fn(),
  },
}));

import { ActivityPage } from "./activity-page";
import { ExportsPage } from "./exports-page";
import { ImportsPage } from "./imports-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  captured.activity = [];
  captured.exports = [];
  captured.imports = [];
  captured.ioQuery = undefined;
  captured.actorFilter = undefined;
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

async function render(node: ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await flush();
}

async function act2(fn: () => void) {
  await act(async () => fn());
  await flush();
}

const last = <T,>(list: T[]) => list[list.length - 1]!;
const column = (id: string) =>
  (captured.table?.columns as ColumnDef<AnyRow, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("ActivityPage", () => {
  it("maps resource/action (CSV), actor and created range; export gets them", async () => {
    await render(createElement(ActivityPage));
    expect(last(captured.activity)).toEqual({
      limit: 20,
      offset: 0,
      sort: "-created_at",
    });
    expect(
      column("resource")?.meta?.filterOptions?.some(
        (option) => option.value === "platform.users",
      ),
    ).toBe(true);

    await act2(() =>
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "resource", value: ["platform.users", "platform.roles"] },
        { id: "action", value: ["export.requested"] },
        { id: "created_at", value: ["2026-09-01", undefined] },
      ]),
    );
    await act2(() => captured.actorFilter?.onValueChange("user-uuid"));

    const expected = {
      resource: "platform.users,platform.roles",
      action: "export.requested",
      actor: "user-uuid",
      created_from: "2026-09-01",
    };
    expect(last(captured.activity)).toMatchObject(expected);
    expect(captured.ioQuery).toMatchObject({
      ...expected,
      sort: "-created_at",
    });
    expect(captured.actorFilter?.value).toBe("user-uuid");

    await act2(() => captured.actorFilter?.onValueChange(""));
    expect(last(captured.activity).actor).toBeUndefined();
  });

  it("sorts on created_at/action/resource only", async () => {
    await render(createElement(ActivityPage));
    expect(column("actor")?.enableSorting).toBe(false);
    await act2(() =>
      captured.table?.state?.onSortingChange?.([{ id: "action", desc: false }]),
    );
    expect(last(captured.activity).sort).toBe("action");
    expect(captured.table?.rowCount).toBe(7);
  });
});

describe("ExportsPage / ImportsPage", () => {
  it("export jobs: sort, q and status/resource/format/date filters", async () => {
    await render(createElement(ExportsPage, { scope: "tenant", slug: "acme" }));
    expect(last(captured.exports)).toEqual({
      params: { limit: 20, offset: 0, sort: "-created_at" },
      scope: "tenant",
    });
    // Tenant scope only offers tenant resources.
    expect(
      column("resource")
        ?.meta?.filterOptions?.map((option) => option.value)
        .every((value) => value.startsWith("tenant.")),
    ).toBe(true);

    await act2(() => {
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "status", value: ["failed", "expired"] },
        { id: "format", value: ["xlsx"] },
        { id: "resource", value: ["tenant.sales"] },
        { id: "created_at", value: [undefined, "2026-10-01"] },
      ]);
      captured.table?.state?.onGlobalFilterChange?.("sales");
      captured.table?.state?.onSortingChange?.([{ id: "status", desc: true }]);
    });

    expect(last(captured.exports).params).toMatchObject({
      status: "failed,expired",
      format: "xlsx",
      resource: "tenant.sales",
      created_to: "2026-10-01",
      q: "sales",
      sort: "-status",
    });
    expect(captured.table?.rowCount).toBe(3);
    expect(captured.table?.features?.persistKey).toBe("tenant-exports-v1-acme");
  });

  it("import jobs: same params on the platform list", async () => {
    await render(createElement(ImportsPage));
    await act2(() => {
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "status", value: ["applied", "rolled_back"] },
        { id: "format", value: ["csv", "tsv"] },
      ]);
      captured.table?.state?.onSortingChange?.([
        { id: "resource", desc: false },
      ]);
    });
    expect(last(captured.imports)).toEqual({
      scope: "platform",
      params: {
        limit: 20,
        offset: 0,
        sort: "resource",
        status: "applied,rolled_back",
        format: "csv,tsv",
      },
    });
    expect(column("source_filename")?.enableSorting).toBe(false);
  });
});
