// @vitest-environment jsdom
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Permission } from "@/config/permissions";

const api = vi.hoisted(() => ({
  listEntries: vi.fn(),
  listTransfers: vi.fn(),
  listCounts: vi.fn(),
}));
const orders = vi.hoisted(() => ({ list: vi.fn() }));

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
    format: { number: (v: number) => String(v) },
  }),
}));
vi.mock("@/features/warehouse/services/warehouse.service", async (orig) => ({
  ...(await orig<object>()),
  warehouseService: api,
}));
vi.mock("@/features/orders/services/orders.service", async (orig) => ({
  ...(await orig<object>()),
  ordersService: orders,
}));

import { WarehouseKpis } from "@/features/warehouse/components/warehouse-kpis";
import {
  flush,
  mount,
  render,
  unmount,
  type Mounted,
} from "@/features/warehouse/components/test-helpers";

const page = (total: number) => ({ items: [], total, limit: 1, offset: 0 });

describe("WarehouseKpis (TEC-1032)", () => {
  let m: Mounted;
  beforeEach(() => {
    m = mount();
    vi.clearAllMocks();
    orders.list.mockImplementation(async (q: { status: string }) =>
      page(q.status.startsWith("approved") ? 3 : 1),
    );
    api.listEntries.mockResolvedValue(page(2));
    api.listTransfers.mockResolvedValue(page(0));
    api.listCounts.mockRejectedValue(new Error("boom"));
  });
  afterEach(() => unmount(m));

  it("renders one linked tile per visible KPI with the list total", async () => {
    const granted: string[] = [
      Permission.WarehouseRead,
      Permission.OrdersRead,
      Permission.OrdersShip,
    ];
    await render(
      m,
      createElement(WarehouseKpis, {
        slug: "acme",
        can: (p: string) => granted.includes(p),
        orgType: "center",
      }),
    );
    await flush();
    const tiles = [...m.container.querySelectorAll("[data-kpi]")];
    expect(tiles.map((el) => el.getAttribute("data-kpi"))).toEqual([
      "prepare_queue",
      "ready_shipped",
      "open_orders",
      "draft_entries",
      "open_transfers",
      "open_counts",
    ]);
    const byId = (id: string) =>
      m.container.querySelector(`[data-kpi="${id}"]`)!;
    expect(byId("prepare_queue").getAttribute("data-count")).toBe("3");
    expect(byId("prepare_queue").getAttribute("href")).toBe(
      "/t/acme/orders?status=approved,processing,preparing",
    );
    expect(byId("draft_entries").getAttribute("data-count")).toBe("2");
    expect(byId("open_transfers").getAttribute("data-count")).toBe("0");
    expect(
      byId("open_counts").querySelector("[data-testid=warehouse-kpi-error]"),
    ).not.toBeNull();
    // Every list call is a limit=1 probe.
    expect(orders.list).toHaveBeenCalledWith(
      expect.objectContaining({ side: "seller", limit: 1, offset: 0 }),
    );
    expect(api.listEntries).toHaveBeenCalledWith({
      status: "draft",
      limit: 1,
      offset: 0,
    });
  });

  it("renders nothing without a visible KPI", async () => {
    await render(
      m,
      createElement(WarehouseKpis, {
        slug: "acme",
        can: () => false,
        orgType: "center",
      }),
    );
    expect(
      m.container.querySelector("[data-testid=warehouse-kpis]"),
    ).toBeNull();
    expect(api.listEntries).not.toHaveBeenCalled();
  });
});
