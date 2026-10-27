import { describe, expect, it } from "vitest";

import { Permission } from "@/config/permissions";
import {
  OPEN_COUNT_STATUSES,
  OPEN_TRANSFER_STATUSES,
  PREPARE_STATUSES,
  visibleKpis,
  WAREHOUSE_KPIS,
} from "@/features/warehouse/lib/kpis";
import { statusFilterFromParam } from "@/features/warehouse/lib/list-seed";

describe("warehouse KPIs (TEC-1032)", () => {
  const can = (granted: string[]) => (p: string) => granted.includes(p);

  it("counts the seller queues and the open warehouse documents", () => {
    const ids = WAREHOUSE_KPIS.map((k) => k.id);
    expect(ids).toEqual([
      "prepare_queue",
      "ready_shipped",
      "open_orders",
      "draft_entries",
      "open_transfers",
      "open_counts",
    ]);
    const prepare = WAREHOUSE_KPIS[0];
    expect(prepare.params).toEqual({
      side: "seller",
      status: PREPARE_STATUSES,
    });
    expect(prepare.href("acme")).toBe(
      `/t/acme/orders?status=${PREPARE_STATUSES}`,
    );
    expect(WAREHOUSE_KPIS[4].href("acme")).toBe(
      `/t/acme/warehouse/transfers?status=${OPEN_TRANSFER_STATUSES}`,
    );
    expect(WAREHOUSE_KPIS[5].href("acme")).toBe(
      `/t/acme/warehouse/counts?status=${OPEN_COUNT_STATUSES}`,
    );
  });

  it("hides order KPIs without orders.read / orders.ship", () => {
    const ids = visibleKpis(can([Permission.WarehouseRead]), "center").map(
      (k) => k.id,
    );
    expect(ids).toEqual(["draft_entries", "open_transfers", "open_counts"]);
    const withOrders = visibleKpis(
      can([Permission.WarehouseRead, Permission.OrdersRead]),
      "center",
    ).map((k) => k.id);
    expect(withOrders).toContain("open_orders");
    expect(withOrders).not.toContain("prepare_queue");
    const ship = visibleKpis(
      can([
        Permission.WarehouseRead,
        Permission.OrdersRead,
        Permission.OrdersShip,
      ]),
      "distributor",
    ).map((k) => k.id);
    expect(ship).toContain("prepare_queue");
    expect(ship).toContain("ready_shipped");
  });

  it("shows nothing to a dealer (K12)", () => {
    expect(
      visibleKpis(
        can([
          Permission.WarehouseRead,
          Permission.OrdersRead,
          Permission.OrdersShip,
        ]),
        "dealer",
      ),
    ).toEqual([]);
  });
});

describe("statusFilterFromParam", () => {
  const allowed = ["draft", "in_transit", "completed"];

  it("seeds the status column from ?status=a,b and drops unknown values", () => {
    expect(statusFilterFromParam("draft, in_transit,bogus", allowed)).toEqual([
      { id: "status", value: ["draft", "in_transit"] },
    ]);
  });

  it("leaves the list unfiltered without a usable value", () => {
    expect(statusFilterFromParam(null, allowed)).toBeUndefined();
    expect(statusFilterFromParam("", allowed)).toBeUndefined();
    expect(statusFilterFromParam("bogus", allowed)).toBeUndefined();
  });
});
