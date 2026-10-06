// @vitest-environment jsdom
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type {
  ListOrganizationsParams,
  Organization,
} from "@/features/organizations/services/organizations.service";

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<Organization>>,
  listParams: [] as ListOrganizationsParams[],
  bulkQuery: undefined as undefined | Record<string, unknown>,
  bulkMenu: undefined as undefined | { resource: string; actions: unknown[] },
  io: undefined as
    | undefined
    | { resource: string; query?: Record<string, string | undefined> },
  meta: {
    default_sort: "-created_at",
    capabilities: { bulk: true, export: true },
    bulk_actions: [
      { id: "suspend", label_key: "bulk.actions.organizations.suspend" },
    ],
  } as Record<string, unknown>,
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
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: unknown }) => children,
  EntityCreateButton: () => null,
  EntityToolbar: () => null,
  EntityTable: (
    props: Partial<DataTableProps<Organization>> & { toolbarExtra?: unknown },
  ) => {
    captured.table = props;
    return props.toolbarExtra ?? null;
  },
}));
vi.mock("@/features/bulk-engine", async (orig) => {
  const actual =
    await orig<typeof import("@/features/bulk-engine/lib/bulk-action-icons")>();
  return {
    BulkActionMenu: (props: { resource: string; actions: unknown[] }) => {
      captured.bulkMenu = props;
      return null;
    },
    SelectionBanner: () => null,
    resolveBulkActionsWithIcons: actual.resolveBulkActionsWithIcons,
    useBulkSelection: (options: { bulkQuery: Record<string, unknown> }) => {
      captured.bulkQuery = options.bulkQuery;
      return {
        rowSelection: {},
        onRowSelectionChange: () => {},
        selectedCount: 0,
        showSelectAllBanner: false,
        scope: { mode: "none" },
        selectAllMatching: () => {},
        clearSelection: () => {},
      };
    },
  };
});
vi.mock("@/features/io", () => ({
  ResourceIOToolbar: (props: {
    resource: string;
    query?: Record<string, string | undefined>;
  }) => {
    captured.io = props;
    return null;
  },
}));
vi.mock("@/features/organizations/hooks/use-organizations-query", () => ({
  useOrganizationsList: (params: ListOrganizationsParams) => {
    captured.listParams.push(params);
    return {
      data: { items: [], total: 31, limit: 20, offset: 0 },
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    };
  },
  useOrganizationsMeta: () => ({ data: captured.meta }),
}));

import { OrganizationsPage } from "./organizations-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  captured.listParams = [];
  captured.bulkQuery = undefined;
  captured.bulkMenu = undefined;
  captured.io = undefined;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function render() {
  await act(async () => {
    root.render(createElement(OrganizationsPage));
  });
}

const lastParams = () => captured.listParams[captured.listParams.length - 1];
const column = (id: string) =>
  (captured.table?.columns as ColumnDef<Organization, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("OrganizationsPage list params", () => {
  it("maps status/type/plan/date filters to the TEC-365 params", async () => {
    await render();
    expect(lastParams()).toEqual({ limit: 20, offset: 0, sort: "-created_at" });

    await act(async () => {
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "status", value: ["active", "read_only"] },
        { id: "type", value: ["distributor", "dealer"] },
        { id: "plan_code", value: "pro,basic" },
        { id: "access_ends_at", value: ["2026-01-01", "2026-12-31"] },
        { id: "created_at", value: [undefined, "2026-06-30"] },
      ]);
    });

    expect(lastParams()).toMatchObject({
      status: "active,read_only",
      type: "distributor,dealer",
      plan_code: "pro,basic",
      access_ends_from: "2026-01-01",
      access_ends_to: "2026-12-31",
      created_to: "2026-06-30",
      offset: 0,
    });
    expect(lastParams().created_from).toBeUndefined();
    // Phone was an unmapped dead filter; q covers it.
    expect(column("phone")?.meta?.filterVariant).toBeUndefined();
  });

  it("sorts by city and access_ends_at (backend whitelist)", async () => {
    await render();
    for (const id of ["name", "slug", "city", "status", "access_ends_at"]) {
      expect(column(id)?.enableSorting, id).toBe(true);
    }
    await act(async () => {
      captured.table?.state?.onSortingChange?.([{ id: "city", desc: false }]);
    });
    expect(lastParams().sort).toBe("city");
    await act(async () => {
      captured.table?.state?.onSortingChange?.([
        { id: "access_ends_at", desc: true },
      ]);
    });
    expect(lastParams().sort).toBe("-access_ends_at");
  });

  it("wires selection, bulk actions and export with the list filters", async () => {
    await render();
    expect(captured.table?.features?.rowSelection).toBe(true);
    expect(captured.table?.rowCount).toBe(31);
    expect(captured.bulkMenu?.resource).toBe("platform.organizations");
    expect(captured.bulkMenu?.actions).toHaveLength(1);
    expect(captured.io?.resource).toBe("platform.organizations");

    await act(async () => {
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "status", value: ["suspended"] },
      ]);
      captured.table?.state?.onGlobalFilterChange?.("acme");
    });

    const expected = { status: "suspended", q: "acme", sort: "-created_at" };
    expect(captured.bulkQuery).toMatchObject(expected);
    expect(captured.io?.query).toMatchObject(expected);
  });
});
