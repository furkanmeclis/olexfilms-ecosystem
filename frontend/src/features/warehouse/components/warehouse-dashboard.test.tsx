// @vitest-environment jsdom
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Permission } from "@/config/permissions";

const orders = vi.hoisted(() => ({ list: vi.fn() }));
const stock = vi.hoisted(() => ({ warehouseSummary: vi.fn() }));
const reports = vi.hoisted(() => ({ report: vi.fn() }));
const warehouse = vi.hoisted(() => ({
  listEntries: vi.fn(),
  listTransfers: vi.fn(),
  listCounts: vi.fn(),
}));
const granted = vi.hoisted(() => ({ list: [] as string[] }));

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
      dateTime: (v: string) => v,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => granted.list.includes(p) }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ uuid: "org-1", type: "center" }),
}));
vi.mock("@/components/charts", () => ({
  AppChart: ({ data }: { data: unknown[] }) =>
    createElement("div", {
      "data-testid": "chart",
      "data-rows": String(data.length),
    }),
}));
vi.mock("@/features/orders/services/orders.service", async (orig) => ({
  ...(await orig<object>()),
  ordersService: orders,
}));
vi.mock("@/features/stock/services/stock.service", async (orig) => ({
  ...(await orig<object>()),
  stockService: stock,
}));
vi.mock(
  "@/features/performance/services/performance.service",
  async (orig) => ({
    ...(await orig<object>()),
    performanceService: reports,
  }),
);
vi.mock("@/features/warehouse/services/warehouse.service", async (orig) => ({
  ...(await orig<object>()),
  warehouseService: warehouse,
}));

import { WarehouseDashboard } from "@/features/warehouse/components/warehouse-dashboard";
import {
  flush,
  mount,
  render,
  unmount,
  type Mounted,
} from "@/features/warehouse/components/test-helpers";

const order = (no: string) => ({
  uuid: `u-${no}`,
  order_no: no,
  status: "approved",
  status_label: "Approved",
  buyer: { uuid: "b1", name: "Bayi A", type: "dealer" },
  seller: { uuid: "org-1", name: "Center", type: "center" },
  created_at: "2026-11-01T10:00:00Z",
  total: "999.00",
  currency: "TRY",
});

describe("WarehouseDashboard (TEC-1066)", () => {
  let m: Mounted;
  beforeEach(() => {
    m = mount();
    vi.clearAllMocks();
    granted.list = [
      Permission.WarehouseRead,
      Permission.StockRead,
      Permission.OrdersRead,
      Permission.OrdersShip,
    ];
    orders.list.mockImplementation(async (q: { limit: number }) =>
      q.limit === 1
        ? { items: [], total: 2, limit: 1, offset: 0 }
        : {
            items: [order("ORD-1"), order("ORD-2")],
            total: 2,
            limit: 8,
            offset: 0,
          },
    );
    warehouse.listEntries.mockResolvedValue({ items: [], total: 0 });
    warehouse.listTransfers.mockResolvedValue({ items: [], total: 0 });
    warehouse.listCounts.mockResolvedValue({ items: [], total: 0 });
    stock.warehouseSummary.mockResolvedValue({
      items: [
        {
          warehouse: {
            uuid: "w1",
            code: "YAK",
            name: "Yakuplu",
            is_primary: true,
          },
          products: 4,
          pieces: 10,
          rolls: 3,
          meters: "90.00",
        },
        { warehouse: null, products: 1, pieces: 2, rolls: 0, meters: "0" },
      ],
    });
    reports.report.mockResolvedValue({
      series: [
        {
          key: "created",
          label: "Created",
          points: [{ label: "1", value: 2 }],
        },
      ],
    });
  });
  afterEach(() => unmount(m));

  it("shows stock per warehouse, pending and latest orders, the chart and no price", async () => {
    await render(m, createElement(WarehouseDashboard, { slug: "acme" }));
    await flush();
    const rows = m.container.querySelectorAll(
      "[data-testid=warehouse-stock-row]",
    );
    expect(rows).toHaveLength(2);
    expect(rows[0]!.textContent).toContain("Yakuplu");
    expect(rows[1]!.textContent).toContain("warehouse.home.stock_unplaced");

    expect(orders.list).toHaveBeenCalledWith(
      expect.objectContaining({
        side: "seller",
        status: "approved,processing,preparing",
        sort: "created_at",
        limit: 8,
      }),
    );
    expect(orders.list).toHaveBeenCalledWith(
      expect.objectContaining({
        side: "seller",
        sort: "-created_at",
        limit: 8,
      }),
    );
    const pending = m.container.querySelector(
      "[data-testid=warehouse-pending-orders]",
    )!;
    expect(pending.textContent).toContain("Bayi A");
    expect(pending.textContent).toContain("ORD-1");
    expect(m.container.textContent).not.toContain("999");

    expect(reports.report).toHaveBeenCalledWith("orders.trend", {
      period: "30d",
    });
    expect(
      m.container
        .querySelector("[data-testid=chart]")
        ?.getAttribute("data-rows"),
    ).toBe("1");
  });

  it("drops the order cards without orders.read", async () => {
    granted.list = [Permission.WarehouseRead, Permission.StockRead];
    await render(m, createElement(WarehouseDashboard, { slug: "acme" }));
    expect(
      m.container.querySelector("[data-testid=warehouse-pending-orders]"),
    ).toBeNull();
    expect(
      m.container.querySelector("[data-testid=warehouse-stock-card]"),
    ).not.toBeNull();
  });
});
