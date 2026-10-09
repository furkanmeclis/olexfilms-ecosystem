// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, Fragment, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  listUnits: vi.fn(),
  listProducts: vi.fn(),
  listDealers: vi.fn(),
  unitLabels: vi.fn(),
}));
const captured = vi.hoisted(() => ({
  tables: [] as Record<string, unknown>[],
  exports: [] as Record<string, unknown>[],
  combos: [] as Record<string, unknown>[],
  download: vi.fn(),
}));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "dealer" as string | null,
  feature: true,
}));

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
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      currency: (v: number, c: string) => `${v} ${c}`,
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () =>
    state.orgType
      ? { uuid: "org-1", slug: "acme", name: "Acme", type: state.orgType }
      : null,
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/features/modules/hooks/use-features", () => ({
  useFeature: () => ({
    enabled: state.feature,
    isLoading: false,
    isError: false,
  }),
}));
vi.mock("@/features/stock/services/stock.service", async (orig) => ({
  ...(await orig<object>()),
  stockService: api,
}));
vi.mock("@/lib/api/platform-form-request", async (orig) => ({
  ...(await orig<object>()),
  triggerBrowserDownload: captured.download,
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/features/io/components/export-menu", () => ({
  ExportMenu: (props: Record<string, unknown>) => {
    captured.exports.push(props);
    return null;
  },
}));
vi.mock("@/components/ui/async-combobox", () => ({
  AsyncCombobox: (props: Record<string, unknown>) => {
    captured.combos.push(props);
    return null;
  },
}));
// The table renders each row's cells (no select column) and the toolbar.
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityRowActions: ({ actions }: { actions: { id: string }[] }) =>
    createElement("span", {
      "data-testid": "row-actions",
      "data-actions": actions.map((a) => a.id).join(","),
    }),
  EntityTable: (props: {
    columns: ColumnDef<unknown, unknown>[];
    data: unknown[];
    toolbarExtra?: ReactNode;
    emptyTitle?: string;
  }) => {
    captured.tables.push(props as Record<string, unknown>);
    const cells = props.columns.filter(
      (c) => c.id !== "__select" && typeof c.cell === "function",
    );
    return createElement(
      "div",
      null,
      props.toolbarExtra,
      props.data.length === 0
        ? createElement("p", { "data-testid": "table-empty" }, props.emptyTitle)
        : null,
      ...props.data.map((row, i) =>
        createElement(
          "div",
          { key: i },
          ...cells.map((c, j) =>
            createElement(
              Fragment,
              { key: j },
              (c.cell as (ctx: unknown) => ReactNode)({
                row: { original: row },
              }),
            ),
          ),
        ),
      ),
    );
  },
}));

import { Permission } from "@/config/permissions";
import {
  unitLabelsPath,
  type StockProduct,
  type StockUnitRow,
} from "@/features/stock/services/stock.service";

import { MyStockPage } from "./my-stock-page";
import { StockSummaryWidget } from "./stock-summary-widget";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  captured.exports = [];
  captured.combos = [];
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  api.listProducts.mockResolvedValue(page([]));
  api.listDealers.mockResolvedValue([]);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
  state.grants = new Set();
  state.orgType = "dealer";
  state.feature = true;
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

function page<T>(items: T[]) {
  return { items, total: items.length, limit: 20, offset: 0 };
}

function unit(patch: Partial<StockUnitRow> = {}): StockUnitRow {
  return {
    uuid: "u-1",
    barcode: "OLX-1",
    unit_kind: "serial",
    status: "available",
    quantity: 1,
    initial_meters: "50.00",
    remaining_meters: "37.50",
    product: {
      uuid: "p-1",
      sku: "PPF-190",
      name: "Olex PPF 190",
      unit_type: "roll_meter",
      uses_fixed_barcode: false,
    },
    location: null,
    purchase_price: null,
    updated_at: "2026-10-02T10:00:00Z",
    ...patch,
  };
}

function product(patch: Partial<StockProduct> = {}): StockProduct {
  return {
    product: {
      uuid: "p-1",
      sku: "PPF-190",
      name: "Olex PPF 190",
      unit_type: "roll_meter",
      uses_fixed_barcode: false,
      active: true,
      category: { uuid: "c-1", name: "PPF" },
    },
    quantity: 0,
    meters: "37.50",
    fixed_barcodes: [],
    updated_at: "2026-10-02T10:00:00Z",
    ...patch,
  };
}

const rows = () => container.querySelectorAll('[data-testid="stock-row"]');
const byId = (id: string) =>
  container.querySelector(`[data-testid="${id}"]`) as HTMLElement | null;

type TableProps = {
  columns: ColumnDef<StockUnitRow, unknown>[];
  rowCount?: number;
  bulkActions?: { id: string; onClick: (rows: StockUnitRow[]) => void }[];
  state?: {
    onColumnFiltersChange?: (v: { id: string; value: unknown }[]) => void;
    onSortingChange?: (v: { id: string; desc: boolean }[]) => void;
    onGlobalFilterChange?: (v: string) => void;
  };
};
const lastTable = () =>
  captured.tables[captured.tables.length - 1] as unknown as TableProps;
const columnIds = () =>
  lastTable().columns.map(
    (c) => c.id ?? ("accessorKey" in c ? String(c.accessorKey) : "") ?? "",
  );
const lastUnitsCall = () =>
  api.listUnits.mock.calls[api.listUnits.mock.calls.length - 1];

describe("MyStockPage (TEC-224, TEC-374 DataTable)", () => {
  it("hides the purchase price column when no row carries a price", async () => {
    state.grants = new Set([Permission.StockRead]);
    api.listUnits.mockResolvedValue(page([unit(), unit({ uuid: "u-2" })]));
    await render(createElement(MyStockPage, { slug: "acme" }));
    expect(rows()).toHaveLength(2);
    expect(columnIds()).not.toContain("purchase_price");
    expect(byId("stock-price")).toBeNull();
    expect(container.textContent).toContain(
      'stock.list.meters_of {"remaining":"37.50","initial":"50.00"}',
    );
    expect(lastUnitsCall()).toEqual([
      "org-1",
      { limit: 20, offset: 0, sort: "product" },
    ]);
    expect(lastTable().rowCount).toBe(2);
  });

  it("shows the purchase price column when the API answers a price", async () => {
    state.grants = new Set([Permission.StockRead]);
    api.listUnits.mockResolvedValue(
      page([
        unit({
          purchase_price: { amount: "120.00", currency: "TRY", source: "list" },
        }),
        unit({ uuid: "u-2" }),
      ]),
    );
    await render(createElement(MyStockPage, { slug: "acme" }));
    expect(columnIds()).toContain("purchase_price");
    const cells = container.querySelectorAll('[data-testid="stock-price"]');
    expect(cells).toHaveLength(2);
    expect(cells[0].textContent).toBe("120 TRY");
    expect(cells[1].textContent).toBe("—");
  });

  it("maps column filters and sort to the units params", async () => {
    state.grants = new Set([Permission.StockRead]);
    api.listUnits.mockResolvedValue(page([unit()]));
    await render(createElement(MyStockPage, { slug: "acme" }));
    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "barcode", value: " OLX-" },
        { id: "status", value: ["available", "placed"] },
        { id: "updated_at", value: ["2026-10-01", "2026-10-05"] },
      ]);
    });
    await act(async () => {
      lastTable().state?.onSortingChange?.([{ id: "meters", desc: true }]);
      lastTable().state?.onGlobalFilterChange?.("ppf");
    });
    await flush();
    expect(lastUnitsCall()).toEqual([
      "org-1",
      {
        limit: 20,
        offset: 0,
        sort: "-meters",
        q: "ppf",
        barcode: "OLX-",
        barcode_match: "prefix",
        status: "available,placed",
        updated_from: "2026-10-01",
        updated_to: "2026-10-05",
      },
    ]);
  });

  it("filters by a product picked from the async search", async () => {
    state.grants = new Set([Permission.StockRead]);
    api.listUnits.mockResolvedValue(page([unit()]));
    api.listProducts.mockResolvedValue(page([product()]));
    await render(createElement(MyStockPage, { slug: "acme" }));
    const combo = captured.combos[captured.combos.length - 1] as {
      loadOptions: (q: string) => Promise<{ value: string; label: string }[]>;
      onValueChange: (v: string) => void;
    };
    const options = await combo.loadOptions(" ppf ");
    // Searched page by page with q, under the endpoint's 100 cap.
    expect(api.listProducts).toHaveBeenLastCalledWith("org-1", {
      q: "ppf",
      limit: 20,
      offset: 0,
    });
    expect(options[0]).toMatchObject({ value: "p-1", label: "Olex PPF 190" });
    await act(async () => combo.onValueChange("p-1"));
    await flush();
    expect(lastUnitsCall()?.[1]).toMatchObject({ product_uuid: "p-1" });
    const picked = captured.combos[captured.combos.length - 1] as {
      value: string;
      options: { label: string }[];
    };
    expect(picked.value).toBe("p-1");
    expect(picked.options[0].label).toBe("Olex PPF 190");
  });

  it("the consumed tab pins status=used and drops the status filter", async () => {
    state.grants = new Set([Permission.StockRead]);
    api.listUnits.mockResolvedValue(page([]));
    await render(createElement(MyStockPage, { slug: "acme" }));
    expect(byId("table-empty")?.textContent).toBe("stock.list.empty_title");
    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "status", value: ["placed"] },
      ]);
    });
    await act(async () => {
      byId("stock-tab-consumed")?.click();
    });
    await flush();
    expect(byId("table-empty")?.textContent).toBe(
      "stock.list.empty_consumed_title",
    );
    expect(lastUnitsCall()).toEqual([
      "org-1",
      { limit: 20, offset: 0, sort: "product", status: "used" },
    ]);
    const status = lastTable().columns.find(
      (c) => "accessorKey" in c && c.accessorKey === "status",
    );
    expect(status?.enableColumnFilter).toBe(false);
  });

  it("exports with the same tab, filters, search and sort", async () => {
    state.grants = new Set([Permission.StockRead]);
    api.listUnits.mockResolvedValue(page([unit()]));
    await render(createElement(MyStockPage, { slug: "acme" }));
    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "status", value: ["available"] },
      ]);
      lastTable().state?.onGlobalFilterChange?.("ppf");
    });
    await flush();
    const props = captured.exports[captured.exports.length - 1];
    expect(props?.exportPath).toBe(
      "/v1/stock/organizations/org-1/units/export",
    );
    expect(props?.formats).toEqual(["xlsx", "csv", "pdf"]);
    expect(props?.query).toEqual({
      status: "available",
      q: "ppf",
      sort: "product",
    });
  });

  it("prints labels for the selected units and for one row", async () => {
    state.grants = new Set([Permission.StockRead]);
    const a = unit();
    const b = unit({ uuid: "u-2", barcode: "OLX-2" });
    api.listUnits.mockResolvedValue(page([a, b]));
    api.unitLabels.mockResolvedValue({
      blob: new Blob(["%PDF"]),
      filename: null,
    });
    await render(createElement(MyStockPage, { slug: "acme" }));
    const bulk = lastTable().bulkActions ?? [];
    expect(bulk.map((x) => x.id)).toEqual(["print_labels"]);
    await act(async () => bulk[0].onClick([a, b]));
    await flush();
    expect(api.unitLabels).toHaveBeenLastCalledWith(["OLX-1", "OLX-2"]);
    expect(captured.download).toHaveBeenLastCalledWith(
      expect.any(Blob),
      "labels.pdf",
    );
    expect(
      container
        .querySelector('[data-testid="row-actions"]')
        ?.getAttribute("data-actions"),
    ).toBe("print_label");
  });

  it("builds the units.pdf path with one barcode per unit", () => {
    expect(unitLabelsPath(["A 1", "B/2"])).toBe(
      "/v1/stock/labels/units.pdf?barcode=A%201&barcode=B%2F2",
    );
  });

  it("opens a palette deep link as a barcode filter", async () => {
    state.grants = new Set([Permission.StockRead]);
    api.listUnits.mockResolvedValue(page([unit()]));
    await render(
      createElement(MyStockPage, { slug: "acme", initialBarcode: "OLX-1" }),
    );
    expect(lastUnitsCall()?.[1]).toMatchObject({
      barcode: "OLX-1",
      barcode_match: "prefix",
    });
  });

  it("lets a distributor pick a dealer of its subtree", async () => {
    state.grants = new Set([Permission.StockRead]);
    state.orgType = "distributor";
    api.listDealers.mockResolvedValue([{ uuid: "d-1", name: "Bayi Kadıköy" }]);
    api.listUnits.mockResolvedValue(page([unit()]));
    await render(createElement(MyStockPage, { slug: "acme" }));
    type Combo = {
      id?: string;
      options: { value: string }[];
      onValueChange: (v: string) => void;
    };
    const picker = () =>
      captured.combos.findLast((c) => c.id === "stock-dealer") as
        Combo | undefined;
    expect(picker()).toBeDefined();
    expect(picker()!.options.map((o) => o.value)).toEqual(["own", "d-1"]);
    expect(lastUnitsCall()?.[0]).toBe("org-1");

    await act(async () => picker()!.onValueChange("d-1"));
    await flush();
    expect(api.listUnits).toHaveBeenLastCalledWith(
      "d-1",
      expect.objectContaining({ offset: 0 }),
    );
    expect(container.textContent).toContain(
      'stock.dealer.read_only {"name":"Bayi Kadıköy"}',
    );
  });

  it("has no dealer picker for a dealer", async () => {
    state.grants = new Set([Permission.StockRead]);
    api.listUnits.mockResolvedValue(page([]));
    await render(createElement(MyStockPage, { slug: "acme" }));
    expect(byId("stock-dealer")).toBeNull();
    expect(api.listDealers).not.toHaveBeenCalled();
  });

  it("is forbidden without stock.read", async () => {
    await render(createElement(MyStockPage, { slug: "acme" }));
    expect(api.listUnits).not.toHaveBeenCalled();
    expect(container.textContent).toContain("common.error_forbidden");
  });
});

describe("StockSummaryWidget (TEC-224)", () => {
  it("sums products, pieces and meters", async () => {
    state.grants = new Set([Permission.StockRead]);
    api.listProducts.mockResolvedValue(
      page([
        product(),
        product({
          product: { ...product().product, uuid: "p-2", unit_type: "piece" },
          quantity: 3,
          meters: "0.00",
        }),
        product({
          product: { ...product().product, uuid: "p-3" },
          quantity: 0,
          meters: "2.50",
        }),
      ]),
    );
    await render(createElement(StockSummaryWidget, { slug: "acme" }));
    expect(byId("stock-widget-products")?.textContent).toBe("3");
    expect(byId("stock-widget-quantity")?.textContent).toBe("3");
    expect(byId("stock-widget-meters")?.textContent).toBe("40");
    expect(byId("stock-widget-top")?.querySelectorAll("li")).toHaveLength(3);
    expect(byId("stock-widget-link")?.getAttribute("href")).toBe(
      "/t/acme/stock",
    );
    expect(api.listProducts).toHaveBeenCalledWith("org-1", {
      status: "in_stock",
      limit: 200,
      offset: 0,
    });
  });

  it("shows the empty text with zero totals", async () => {
    state.grants = new Set([Permission.StockRead]);
    api.listProducts.mockResolvedValue(page([]));
    await render(createElement(StockSummaryWidget, { slug: "acme" }));
    expect(byId("stock-widget-quantity")?.textContent).toBe("0");
    expect(byId("stock-widget-empty")).not.toBeNull();
  });

  it("renders nothing without stock.read, for the center, or with the module off", async () => {
    await render(createElement(StockSummaryWidget, { slug: "acme" }));
    expect(byId("stock-summary-widget")).toBeNull();

    state.grants = new Set([Permission.StockRead]);
    state.orgType = "center";
    await render(createElement(StockSummaryWidget, { slug: "acme" }));
    expect(byId("stock-summary-widget")).toBeNull();

    state.orgType = "dealer";
    state.feature = false;
    await render(createElement(StockSummaryWidget, { slug: "acme" }));
    expect(byId("stock-summary-widget")).toBeNull();
    expect(api.listProducts).not.toHaveBeenCalled();
  });
});
