// @vitest-environment jsdom
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  getOrderPlacement: vi.fn(),
  placeOrder: vi.fn(),
  scan: vi.fn(),
  listWarehouses: vi.fn(),
  listRooms: vi.fn(),
  listLocations: vi.fn(),
}));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "distributor" as string | null,
}));
const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
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
    state.orgType ? { slug: "acme", type: state.orgType } : null,
}));
vi.mock("@/providers/toast-provider", () => ({ appToast: toast }));
vi.mock("@/hooks/use-mobile", () => ({
  useIsMobile: () => false,
  useIsXl: () => true,
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/features/warehouse/services/warehouse.service", async (orig) => ({
  ...(await orig<object>()),
  warehouseService: api,
}));

import { Permission } from "@/config/permissions";
import { ApiError } from "@/lib/api";

import { classifyUnitScan, showOrderPlacement } from "../lib/order-placement";
import type { WarehouseOrderPlacement } from "../services/warehouse.service";
import { OrderPlacementPanel } from "./order-placement-panel";
import {
  click,
  fill,
  flush,
  mount,
  render,
  unmount,
  type Mounted,
} from "./test-helpers";

const product = { uuid: "p-1", sku: "PPF-1", name: "PPF Gloss" };
const unit = (barcode: string, unplaced: boolean, kind = "serial") => ({
  unit_uuid: `u-${barcode}`,
  barcode,
  unit_kind: kind as "serial" | "fixed",
  product,
  meters: kind === "serial" ? "25.00" : null,
  quantity: kind === "serial" ? null : 10,
  unplaced,
});
const placement = (
  over: Partial<WarehouseOrderPlacement> = {},
): WarehouseOrderPlacement => ({
  order_uuid: "o-1",
  order_no: "ORD-00000001",
  status: "received",
  unplaced_count: 2,
  placed_count: 1,
  units: [
    unit("OLX-00000001", true),
    unit("OLX-00000002", true),
    unit("OLX-00000003", false),
  ],
  fixed: [unit("FIX-1", false, "fixed")],
  ...over,
});
const order = (over: Record<string, unknown> = {}) => ({
  uuid: "o-1",
  status: "received",
  role: "buyer",
  buyer: { type: "distributor" },
  ...over,
});

describe("showOrderPlacement (TEC-765)", () => {
  const received = order();
  it("shows the panel to a center or distributor buyer of a received order", () => {
    expect(showOrderPlacement(received, "distributor", true)).toBe(true);
    expect(
      showOrderPlacement(order({ buyer: { type: "center" } }), "center", true),
    ).toBe(true);
  });
  it("hides it from a dealer (K12), before receipt, from the seller and without warehouse.write", () => {
    expect(
      showOrderPlacement(order({ buyer: { type: "dealer" } }), "dealer", true),
    ).toBe(false);
    for (const status of ["shipped", "delivered", "preparing", "cancelled"]) {
      expect(showOrderPlacement(order({ status }), "distributor", true)).toBe(
        false,
      );
    }
    expect(
      showOrderPlacement(order({ role: "seller" }), "distributor", true),
    ).toBe(false);
    expect(showOrderPlacement(received, "distributor", false)).toBe(false);
    expect(showOrderPlacement(received, null, true)).toBe(false);
  });
});

describe("classifyUnitScan", () => {
  const p = placement();
  it("selects a waiting unit (QR or bare barcode, any case)", () => {
    expect(classifyUnitScan(p, "OFW:UNIT:OLX-00000001", []).kind).toBe(
      "select",
    );
    expect(classifyUnitScan(p, "olx-00000002", []).kind).toBe("select");
    expect(classifyUnitScan(p, "OLX-00000002", ["u-OLX-00000002"]).kind).toBe(
      "already_selected",
    );
  });
  it("names placed, fixed and foreign codes", () => {
    expect(classifyUnitScan(p, "OLX-00000003", []).kind).toBe("placed");
    expect(classifyUnitScan(p, "FIX-1", []).kind).toBe("fixed");
    expect(classifyUnitScan(p, "OLX-99999999", [])).toEqual({
      kind: "not_on_order",
      barcode: "OLX-99999999",
    });
  });
});

describe("OrderPlacementPanel", () => {
  let m: Mounted;
  beforeEach(() => {
    m = mount();
    vi.clearAllMocks();
    state.grants = new Set([
      Permission.WarehouseRead,
      Permission.WarehouseWrite,
    ]);
    state.orgType = "distributor";
    api.listWarehouses.mockResolvedValue({ items: [] });
    api.getOrderPlacement.mockResolvedValue(placement());
  });
  afterEach(() => unmount(m));

  const q = (id: string) => m.container.querySelector(`[data-testid="${id}"]`);
  const scanInto = async (id: string, code: string) => {
    await fill(q(`${id}-input`), code);
    await click(q(`${id}-submit`));
    await flush();
  };

  it("renders nothing for a dealer or an order not yet received", async () => {
    state.orgType = "dealer";
    await render(
      m,
      createElement(OrderPlacementPanel, {
        slug: "acme",
        order: order({ buyer: { type: "dealer" } }),
      }),
    );
    expect(q("order-placement")).toBeNull();
    state.orgType = "distributor";
    await render(
      m,
      createElement(OrderPlacementPanel, {
        slug: "acme",
        order: order({ status: "delivered" }),
      }),
    );
    expect(q("order-placement")).toBeNull();
    expect(api.getOrderPlacement).not.toHaveBeenCalled();
  });

  it("scans a location, then units, and places the selected ones", async () => {
    api.scan.mockResolvedValue({
      type: "location",
      matched_by: "location_qr",
      code: "D1-R1-A-01",
      location: { uuid: "loc-1", full_code: "D1-R1-A-01", active: true },
      unit: null,
      product: null,
    });
    api.placeOrder.mockResolvedValue({
      location: { uuid: "loc-1" },
      units: [{ unit_uuid: "u-OLX-00000001", barcode: "OLX-00000001" }],
    });
    await render(
      m,
      createElement(OrderPlacementPanel, { slug: "acme", order: order() }),
    );
    expect(q("order-unplaced-badge")?.textContent).toContain('"count":2');
    expect(q("placement-fixed-note")).not.toBeNull();
    expect(
      m.container.querySelectorAll('[data-testid="placement-unit"]'),
    ).toHaveLength(2);
    expect((q("placement-submit") as HTMLButtonElement).disabled).toBe(true);

    // A unit in the location field is refused.
    await scanInto("placement-location", "OFW:UNIT:OLX-00000001");
    expect(q("placement-error")?.textContent).toBe(
      "warehouse.order_place.scan_location_first",
    );
    await scanInto("placement-location", "OFW:LOC:D1-R1-A-01");
    expect(api.scan).toHaveBeenCalledWith("OFW:LOC:D1-R1-A-01");
    expect(q("placement-target")?.textContent).toContain("D1-R1-A-01");

    await scanInto("placement-unit", "OLX-00000003");
    expect(q("placement-error")?.textContent).toContain("already_placed");
    await scanInto("placement-unit", "FIX-1");
    expect(q("placement-error")?.textContent).toContain("fixed_scanned");
    await scanInto("placement-unit", "OLX-12345678");
    expect(q("placement-error")?.textContent).toContain("not_on_order");
    await scanInto("placement-unit", "OFW:UNIT:OLX-00000001");
    expect(q("placement-error")).toBeNull();

    api.getOrderPlacement.mockResolvedValue(
      placement({
        unplaced_count: 0,
        placed_count: 3,
        units: [
          unit("OLX-00000001", false),
          unit("OLX-00000002", false),
          unit("OLX-00000003", false),
        ],
      }),
    );
    await click(q("placement-submit"));
    expect(api.placeOrder).toHaveBeenCalledWith("o-1", {
      location_uuid: "loc-1",
      barcodes: ["OLX-00000001"],
    });
    expect(toast.success).toHaveBeenCalled();
    await flush();
    expect(q("order-placement-done")?.textContent).toContain(
      "warehouse.order_place.all_placed",
    );
    expect(q("order-unplaced-badge")).toBeNull();
  });

  it("explains a location of another warehouse and a non-location code", async () => {
    api.scan.mockRejectedValueOnce(
      new ApiError({
        status: 404,
        code: "SCAN_LOCATION_NOT_FOUND",
        message: "x",
      }),
    );
    await render(
      m,
      createElement(OrderPlacementPanel, { slug: "acme", order: order() }),
    );
    await scanInto("placement-location", "OFW:LOC:OTHER-01");
    expect(q("placement-error")?.textContent).toBe(
      "warehouse.order_place.location_not_found",
    );
    api.scan.mockResolvedValueOnce({
      type: "product",
      matched_by: "sku",
      code: "PPF-1",
      location: null,
      unit: null,
      product,
    });
    await scanInto("placement-location", "PPF-1");
    expect(q("placement-error")?.textContent).toBe(
      "warehouse.order_place.not_location",
    );
  });

  it("hides itself when the order is not the caller's purchase (404)", async () => {
    api.getOrderPlacement.mockRejectedValue(
      new ApiError({ status: 404, code: "NOT_FOUND", message: "x" }),
    );
    await render(
      m,
      createElement(OrderPlacementPanel, { slug: "acme", order: order() }),
    );
    expect(q("order-placement")).toBeNull();
  });
});
