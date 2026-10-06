// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type {
  ListRolesParams,
  RoleSummary,
} from "@/features/roles/services/roles.service";

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<RoleSummary>>,
  listParams: [] as ListRolesParams[],
  ioQuery: undefined as undefined | Record<string, string | undefined>,
  meta: undefined as undefined | { default_sort?: string },
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
  EntityTable: (
    props: Partial<DataTableProps<RoleSummary>> & { toolbarExtra?: unknown },
  ) => {
    captured.table = props;
    return props.toolbarExtra ?? null;
  },
}));
vi.mock("@/features/bulk-engine", () => ({
  BulkActionMenu: () => null,
  SelectionBanner: () => null,
  resolveBulkActionsWithIcons: () => [],
  useBulkSelection: () => ({
    rowSelection: {},
    onRowSelectionChange: () => {},
    selectedCount: 0,
    showSelectAllBanner: false,
    scope: { mode: "none" },
    selectAllMatching: () => {},
    clearSelection: () => {},
  }),
}));
vi.mock("@/features/io", () => ({
  ResourceIOToolbar: (props: {
    query?: Record<string, string | undefined>;
  }) => {
    captured.ioQuery = props.query;
    return null;
  },
}));
vi.mock("@/features/roles/hooks/use-role-mutations", () => ({
  useDeleteRole: () => ({ mutateAsync: vi.fn() }),
}));
vi.mock("@/features/roles/services/roles.service", async (orig) => ({
  ...(await orig<object>()),
  rolesService: {
    list: vi.fn(async (params: ListRolesParams) => {
      captured.listParams.push(params);
      return { items: [], total: 42, limit: 20, offset: 0 };
    }),
    meta: vi.fn(async () => captured.meta ?? {}),
  },
}));

import { RolesPage } from "./roles-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  captured.listParams = [];
  captured.ioQuery = undefined;
  captured.meta = undefined;
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

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(QueryClientProvider, { client }, createElement(RolesPage)),
    );
  });
  await flush();
}

const lastParams = () => captured.listParams[captured.listParams.length - 1];
const column = (id: string) =>
  (captured.table?.columns as ColumnDef<RoleSummary, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("RolesPage list params", () => {
  it("sorts by name by default and sends the chosen sort field", async () => {
    await render();
    expect(lastParams()).toEqual({ limit: 20, offset: 0, sort: "name" });
    for (const id of ["name", "slug", "is_system", "created_at"]) {
      expect(column(id)?.enableSorting, id).toBe(true);
    }
    await act(async () => {
      captured.table?.state?.onSortingChange?.([
        { id: "created_at", desc: true },
      ]);
    });
    await flush();
    expect(lastParams().sort).toBe("-created_at");
  });

  it("maps the system filter to is_system and passes it to export", async () => {
    await render();
    // The old name text filter was never mapped; it is gone (q searches).
    expect(column("name")?.meta?.filterVariant).toBeUndefined();
    await act(async () => {
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "is_system", value: "false" },
      ]);
    });
    await flush();
    expect(lastParams()).toMatchObject({ is_system: "false", offset: 0 });
    expect(captured.ioQuery).toMatchObject({ is_system: "false" });
  });

  it("uses meta default_sort and the list total as rowCount", async () => {
    captured.meta = { default_sort: "-updated_at" };
    await render();
    expect(captured.table?.rowCount).toBe(42);
    expect(lastParams().sort).toBe("-updated_at");
  });
});
