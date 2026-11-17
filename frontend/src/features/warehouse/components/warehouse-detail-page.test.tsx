// @vitest-environment jsdom
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  getWarehouse: vi.fn(),
  listRooms: vi.fn(),
  listWarehouseUnits: vi.fn(),
  listEntries: vi.fn(),
  listTransfers: vi.fn(),
}));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "center",
}));
const captured = vi.hoisted(() => ({ keys: [] as string[] }));

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: { href: string; children: unknown } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    t: (key: string) => key,
    format: {
      number: (v: number) => String(v),
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({
    uuid: "org-1",
    slug: "olex",
    type: state.orgType,
  }),
}));
vi.mock("@/features/organizations/providers/tenant-provider", () => ({
  useOptionalTenant: () => ({ slug: "olex" }),
}));
vi.mock("@/features/warehouse/components/active-warehouse", () => ({
  ActiveWarehouseSelect: () => null,
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/features/warehouse/services/warehouse.service", async (orig) => ({
  ...(await orig<object>()),
  warehouseService: api,
}));
// Renders every cell of the rows (the real table is covered elsewhere).
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: ({
    columns,
    data,
    features,
  }: {
    columns: {
      id?: string;
      accessorKey?: string;
      cell?: (ctx: unknown) => ReactNode;
    }[];
    data: { uuid: string }[];
    features?: { persistKey?: string };
  }) => {
    captured.keys.push(features?.persistKey ?? "");
    return createElement(
      "div",
      { "data-testid": "table", "data-key": features?.persistKey },
      data.map((row) =>
        createElement(
          "div",
          { key: row.uuid, "data-row": row.uuid },
          columns.map((c, i) =>
            typeof c.cell === "function"
              ? createElement(
                  "span",
                  { key: c.id ?? c.accessorKey ?? i },
                  c.cell({ row: { original: row } }),
                )
              : null,
          ),
        ),
      ),
    );
  },
}));

import { Permission } from "@/config/permissions";

import { mount, render, unmount, type Mounted } from "./test-helpers";
import {
  WAREHOUSE_DETAIL_UNITS_KEY,
  WarehouseDetailPage,
} from "./warehouse-detail-page";

const PAGE = <T,>(items: T[]) => ({
  items,
  total: items.length,
  limit: 20,
  offset: 0,
});

const unit = {
  uuid: "un-1",
  barcode: "OLX-0001",
  unit_kind: "roll",
  status: "in_stock",
  initial_meters: "15",
  remaining_meters: "10",
  product: { uuid: "p-1", sku: "PPF-190", name: "Olex PPF 190" },
  source: "stock",
  in_warehouse: true,
  owner_type: null,
  location_code: "A-01-01",
  holder: null,
  order: null,
  service: null,
  created_at: "2026-11-01T08:00:00Z",
  updated_at: "2026-11-02T08:00:00Z",
};

let m: Mounted;
beforeEach(() => {
  captured.keys = [];
  window.localStorage.clear();
  state.orgType = "center";
  api.getWarehouse.mockResolvedValue({
    uuid: "wh-1",
    code: "WH1",
    name: "Main",
    address: "Florya",
    active: true,
    require_location_placement: true,
    is_primary: true,
    sort_order: 0,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-11-01T00:00:00Z",
  });
  api.listRooms.mockResolvedValue({
    items: [{ uuid: "r-1" }, { uuid: "r-2" }],
  });
  api.listWarehouseUnits.mockResolvedValue(PAGE([unit]));
  api.listEntries.mockResolvedValue(PAGE([]));
  api.listTransfers.mockResolvedValue(PAGE([]));
  m = mount();
});

afterEach(() => {
  unmount(m);
  vi.clearAllMocks();
  state.grants = new Set();
});

const q = (sel: string) => m.container.querySelector(sel);

async function show() {
  await render(
    m,
    createElement(WarehouseDetailPage, { slug: "olex", uuid: "wh-1" }),
  );
}

describe("WarehouseDetailPage (TEC-1218)", () => {
  it("shows the warehouse and links the unit's product and barcode", async () => {
    state.grants = new Set([
      Permission.WarehouseRead,
      Permission.StockRead,
      Permission.CatalogRead,
    ]);
    await show();
    expect(api.getWarehouse).toHaveBeenCalledWith("wh-1");
    expect(q('[data-testid="warehouse-detail"]')?.textContent).toContain(
      "Florya",
    );
    expect(q('[data-testid="warehouse-detail-rooms"]')?.textContent).toBe("2");
    expect(api.listWarehouseUnits).toHaveBeenCalledWith(
      "wh-1",
      expect.objectContaining({ limit: 20, offset: 0, sort: "-updated_at" }),
    );
    expect(captured.keys).toContain(WAREHOUSE_DETAIL_UNITS_KEY);
    const row = q('[data-row="un-1"]');
    expect(
      row?.querySelector('a[data-entity-kind="product"]')?.getAttribute("href"),
    ).toBe("/t/olex/catalog/products/p-1");
    expect(
      row
        ?.querySelector('a[data-entity-kind="stock_unit"]')
        ?.getAttribute("href"),
    ).toBe("/t/olex/stock/units/OLX-0001");
    // Warehouse roles never see money.
    expect(row?.textContent ?? "").not.toMatch(/price|TRY|₺/i);
  });

  it("renders the product as plain text without catalog.read", async () => {
    state.grants = new Set([Permission.WarehouseRead, Permission.StockRead]);
    await show();
    const row = q('[data-row="un-1"]');
    expect(row?.querySelector('a[data-entity-kind="product"]')).toBeNull();
    expect(
      row?.querySelector('span[data-entity-kind="product"]')?.textContent,
    ).toBe("Olex PPF 190");
  });

  it("hides the units tab without stock.read and lists the entries", async () => {
    state.grants = new Set([Permission.WarehouseRead]);
    await show();
    expect(api.listWarehouseUnits).not.toHaveBeenCalled();
    expect(api.listEntries).toHaveBeenCalledWith(
      expect.objectContaining({ warehouse_uuid: "wh-1" }),
    );
  });

  it("does not load for a dealer organization", async () => {
    state.grants = new Set([Permission.WarehouseRead]);
    state.orgType = "dealer";
    await show();
    expect(api.getWarehouse).not.toHaveBeenCalled();
  });
});
