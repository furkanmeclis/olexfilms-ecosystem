// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type {
  CatalogCategory,
  CatalogProduct,
} from "@/features/catalog/services/catalog.service";
import type { DistributorPrice } from "@/features/catalog/services/pricing.service";

type AnyRow = CatalogProduct | CatalogCategory | DistributorPrice;

const captured = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<AnyRow>>[],
  bulkMenus: [] as Record<string, unknown>[],
  exports: [] as Record<string, unknown>[],
  catalog: {
    listProducts: vi.fn(),
    listCategories: vi.fn(),
    reorderCategories: vi.fn(),
    updateCategory: vi.fn(),
    bulkActive: vi.fn(),
  },
  pricing: {
    listDistributorPrices: vi.fn(),
    listDistributors: vi.fn(),
  },
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), back: vi.fn() }),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: {
      number: (v: number) => String(v),
      dateTime: (v: string) => v,
      currency: (v: number | null, c: string) => `${v} ${c}`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ uuid: "o1", slug: "acme", type: "center" }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/features/catalog/services/catalog.service", async (orig) => ({
  ...(await orig<object>()),
  catalogService: captured.catalog,
}));
vi.mock("@/features/catalog/services/pricing.service", async (orig) => ({
  ...(await orig<object>()),
  pricingService: captured.pricing,
}));
vi.mock("@/features/bulk-engine", async (orig) => ({
  ...(await orig<object>()),
  BulkActionMenu: (props: Record<string, unknown>) => {
    captured.bulkMenus.push(props);
    return null;
  },
}));
vi.mock("@/features/io/components/export-menu", () => ({
  ExportMenu: (props: Record<string, unknown>) => {
    captured.exports.push(props);
    return null;
  },
}));
vi.mock("@/features/io/components/import-button", () => ({
  ImportButton: () => null,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: ReactNode }) => children,
  EntityCreateButton: () => null,
  EntityToolbar: () => null,
  EntityDeleteDialog: () => null,
  EntityTable: (props: Partial<DataTableProps<AnyRow>>) => {
    captured.tables.push(props);
    return createElement(
      "div",
      null,
      (props as { toolbarExtra?: ReactNode }).toolbarExtra,
    );
  },
}));
vi.mock("@/features/catalog/components/category-form-dialog", () => ({
  CategoryFormDialog: () => null,
}));
vi.mock("@/features/catalog/components/price-dialog", () => ({
  PriceDialog: () => null,
}));
vi.mock("@/components/dialogs/delete-dialog", () => ({
  DeleteDialog: () => null,
}));

import { CategoriesPage } from "./categories-page";
import { DistributorPricesCard } from "./distributor-prices-card";
import { ProductsPage } from "./products-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

function category(uuid: string, sort: number): CatalogCategory {
  return {
    uuid,
    name: uuid,
    available_parts: [],
    sort,
    active: true,
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-01T00:00:00Z",
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  captured.bulkMenus = [];
  captured.exports = [];
  for (const fn of [
    ...Object.values(captured.catalog),
    ...Object.values(captured.pricing),
  ]) {
    fn.mockReset();
  }
  captured.catalog.listProducts.mockResolvedValue({
    items: [],
    total: 42,
    limit: 20,
    offset: 0,
  });
  captured.catalog.listCategories.mockResolvedValue({
    items: [category("c1", 10), category("c2", 20), category("c3", 30)],
    total: 3,
    limit: 100,
    offset: 0,
  });
  captured.catalog.reorderCategories.mockResolvedValue({ items: [] });
  captured.catalog.updateCategory.mockResolvedValue(category("c1", 10));
  captured.pricing.listDistributorPrices.mockResolvedValue({
    items: [],
    total: 0,
    limit: 20,
    offset: 0,
  });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function flush() {
  for (let i = 0; i < 4; i++) {
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

const lastTable = () => captured.tables[captured.tables.length - 1];
const lastCall = (fn: ReturnType<typeof vi.fn>) =>
  fn.mock.calls[fn.mock.calls.length - 1];
const column = (id: string) =>
  (lastTable().columns as ColumnDef<AnyRow, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("ProductsPage", () => {
  it("maps column filters and sort to the list params", async () => {
    await render(createElement(ProductsPage, { slug: "acme" }));
    expect(lastCall(captured.catalog.listProducts)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "name",
    });

    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "category", value: ["c1", "c2"] },
        { id: "unit_type", value: ["roll_meter"] },
        { id: "active", value: false },
        { id: "uses_fixed_barcode", value: true },
        { id: "warranty_duration_months", value: [12, 60] },
        { id: "created_at", value: ["2026-01-01", "2026-02-01"] },
      ]);
    });
    await act(async () => {
      lastTable().state?.onSortingChange?.([
        { id: "warranty_duration_months", desc: true },
      ]);
    });
    await flush();

    expect(lastCall(captured.catalog.listProducts)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "-warranty_duration_months",
      category_uuid: "c1,c2",
      unit_type: "roll_meter",
      active: "false",
      uses_fixed_barcode: "true",
      warranty_duration_months_min: "12",
      warranty_duration_months_max: "60",
      created_from: "2026-01-01",
      created_to: "2026-02-01",
    });
    expect(lastTable().rowCount).toBe(42);
  });

  it("exports with the same filters, search and sort", async () => {
    await render(createElement(ProductsPage, { slug: "acme" }));
    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "active", value: true },
      ]);
      lastTable().state?.onGlobalFilterChange?.("ppf");
    });
    await flush();
    const exportProps = captured.exports[captured.exports.length - 1];
    expect(exportProps?.resource).toBe("tenant.catalog.products");
    expect(exportProps?.query).toEqual({
      active: "true",
      q: "ppf",
      sort: "name",
    });
  });

  it("offers bulk activate / deactivate / set_category with category options", async () => {
    await render(createElement(ProductsPage, { slug: "acme" }));
    const menu = captured.bulkMenus[captured.bulkMenus.length - 1];
    expect(menu?.resource).toBe("catalog.products");
    expect(
      (menu?.actions as { id: string }[]).map((action) => action.id),
    ).toEqual(["activate", "deactivate", "set_category"]);
    expect(menu?.paramOptions).toEqual({
      category_uuid: [
        { value: "c1", label: "c1" },
        { value: "c2", label: "c2" },
        { value: "c3", label: "c3" },
      ],
    });
    expect(column("category")?.meta?.filterOptions).toHaveLength(3);
    expect(lastTable().renderGridItem).toBeTypeOf("function");
    expect(lastTable().features?.rowSelection).toBe(true);
  });
});

describe("CategoriesPage", () => {
  it("sends the active filter and sort; reorder only on the sort view", async () => {
    await render(createElement(CategoriesPage, { slug: "acme" }));
    expect(lastCall(captured.catalog.listCategories)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "sort",
    });
    expect(lastTable().features?.rowReorder).toBe(true);

    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "active", value: true },
      ]);
      lastTable().state?.onSortingChange?.([{ id: "name", desc: true }]);
    });
    await flush();
    expect(lastCall(captured.catalog.listCategories)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "-name",
      active: "true",
    });
    expect(lastTable().features?.rowReorder).toBe(false);
    expect(lastTable().onRowReorder).toBeUndefined();
  });

  it("saves a drag-and-drop order with PUT /categories/order", async () => {
    await render(createElement(CategoriesPage, { slug: "acme" }));
    const rows = lastTable().data as CatalogCategory[];
    await act(async () => {
      lastTable().onRowReorder?.([rows[2]!, rows[0]!, rows[1]!]);
    });
    await flush();
    expect(captured.catalog.reorderCategories).toHaveBeenCalledWith([
      "c3",
      "c1",
      "c2",
    ]);
  });

  it("inline-edits name and active through PATCH", async () => {
    await render(createElement(CategoriesPage, { slug: "acme" }));
    const row = (lastTable().data as CatalogCategory[])[0]!;
    await act(async () => {
      lastTable().onCellEdit?.({
        rowId: row.uuid,
        columnId: "active",
        value: false,
        row,
      });
    });
    await act(async () => {
      lastTable().onCellEdit?.({
        rowId: row.uuid,
        columnId: "name",
        value: " Wrap ",
        row,
      });
    });
    await flush();
    expect(captured.catalog.updateCategory).toHaveBeenCalledWith("c1", {
      active: false,
    });
    expect(captured.catalog.updateCategory).toHaveBeenCalledWith("c1", {
      name: "Wrap",
    });
  });

  it("runs bulk actions on catalog.categories", async () => {
    await render(createElement(CategoriesPage, { slug: "acme" }));
    const menu = captured.bulkMenus[captured.bulkMenus.length - 1];
    expect(menu?.resource).toBe("catalog.categories");
    expect(
      (menu?.actions as { id: string }[]).map((action) => action.id),
    ).toEqual(["activate", "deactivate", "delete"]);
  });
});

describe("DistributorPricesCard", () => {
  it("is a nested server table with sort, q and currency CSV", async () => {
    await render(
      createElement(DistributorPricesCard, {
        productUuid: "p1",
        access: {
          canView: true,
          canWriteList: true,
          canWriteRecommended: true,
          canReadDistributorPrices: true,
          canWriteDistributorPrices: true,
          canWriteDealerPrice: false,
        },
      }),
    );
    expect(lastCall(captured.pricing.listDistributorPrices)).toEqual([
      "p1",
      { limit: 20, offset: 0, sort: "distributor" },
    ]);
    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "currency", value: "try, eur" },
      ]);
      lastTable().state?.onSortingChange?.([{ id: "price", desc: true }]);
      lastTable().state?.onGlobalFilterChange?.("north");
    });
    await flush();
    expect(lastCall(captured.pricing.listDistributorPrices)).toEqual([
      "p1",
      {
        limit: 20,
        offset: 0,
        sort: "-price",
        q: "north",
        currency: "TRY,EUR",
      },
    ]);
  });
});
