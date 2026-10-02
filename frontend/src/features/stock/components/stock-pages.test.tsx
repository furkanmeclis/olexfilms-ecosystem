// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  listUnits: vi.fn(),
  listProducts: vi.fn(),
  listDealers: vi.fn(),
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

import { Permission } from "@/config/permissions";
import type {
  StockProduct,
  StockUnitRow,
} from "@/features/stock/services/stock.service";

import { MyStockPage } from "./my-stock-page";
import { StockSummaryWidget } from "./stock-summary-widget";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
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

async function select(selector: string, value: string) {
  await act(async () => {
    const el = container.querySelector(selector) as HTMLSelectElement;
    el.value = value;
    el.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await flush();
}

describe("MyStockPage (TEC-224)", () => {
  it("hides the purchase price column when no row carries a price", async () => {
    state.grants = new Set([Permission.StockRead]);
    api.listUnits.mockResolvedValue(page([unit(), unit({ uuid: "u-2" })]));
    await render(createElement(MyStockPage, { slug: "acme" }));
    expect(rows()).toHaveLength(2);
    expect(byId("stock-price-header")).toBeNull();
    expect(byId("stock-price")).toBeNull();
    expect(container.textContent).toContain(
      'stock.list.meters_of {"remaining":"37.50","initial":"50.00"}',
    );
    expect(api.listUnits).toHaveBeenLastCalledWith("org-1", {
      limit: 20,
      offset: 0,
    });
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
    expect(byId("stock-price-header")).not.toBeNull();
    const cells = container.querySelectorAll('[data-testid="stock-price"]');
    expect(cells).toHaveLength(2);
    expect(cells[0].textContent).toBe("120 TRY");
    expect(cells[1].textContent).toBe("—");
  });

  it("renders the empty state and the consumed tab pins status=used", async () => {
    state.grants = new Set([Permission.StockRead]);
    api.listUnits.mockResolvedValue(page([]));
    await render(createElement(MyStockPage, { slug: "acme" }));
    expect(byId("stock-empty")).not.toBeNull();
    expect(container.textContent).toContain("stock.list.empty_title");
    expect(byId("page-info")?.closest(".hidden")).not.toBeNull();

    await act(async () => {
      byId("stock-tab-consumed")?.click();
    });
    await flush();
    expect(container.textContent).toContain("stock.list.empty_consumed_title");
    expect(api.listUnits).toHaveBeenLastCalledWith(
      "org-1",
      expect.objectContaining({ status: "used", offset: 0 }),
    );
    // The status filter belongs to the on-hand tab only.
    expect(container.querySelector("#stock-status")).toBeNull();
  });

  it("filters by status and product", async () => {
    state.grants = new Set([Permission.StockRead]);
    api.listUnits.mockResolvedValue(page([unit()]));
    api.listProducts.mockResolvedValue(page([product()]));
    await render(createElement(MyStockPage, { slug: "acme" }));
    await select("#stock-status", "placed");
    expect(api.listUnits).toHaveBeenLastCalledWith(
      "org-1",
      expect.objectContaining({ status: "placed" }),
    );
    await select("#stock-product", "p-1");
    expect(api.listUnits).toHaveBeenLastCalledWith(
      "org-1",
      expect.objectContaining({ status: "placed", product_uuid: "p-1" }),
    );
    expect(byId("stock-clear-filters")).not.toBeNull();
  });

  it("lets a distributor pick a dealer of its subtree", async () => {
    state.grants = new Set([Permission.StockRead]);
    state.orgType = "distributor";
    api.listDealers.mockResolvedValue([{ uuid: "d-1", name: "Bayi Kadıköy" }]);
    api.listUnits.mockResolvedValue(page([unit()]));
    await render(createElement(MyStockPage, { slug: "acme" }));
    const picker = byId("stock-dealer") as HTMLSelectElement;
    expect(picker).not.toBeNull();
    expect(picker.querySelectorAll("option")).toHaveLength(2);
    expect(api.listUnits).toHaveBeenLastCalledWith(
      "org-1",
      expect.objectContaining({ offset: 0 }),
    );

    await select('[data-testid="stock-dealer"]', "d-1");
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
