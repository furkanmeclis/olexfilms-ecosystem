import { describe, expect, it } from "vitest";

import type { MyWarehouses } from "../services/warehouse.service";
import { activeWarehouseKey, resolveActiveWarehouse } from "./active-warehouse";

const wh = (uuid: string, member: boolean, isDefault = false) =>
  ({
    uuid,
    code: uuid.toUpperCase(),
    name: uuid,
    address: null,
    active: true,
    sort_order: 0,
    created_at: "",
    updated_at: "",
    member,
    is_default: isDefault,
  }) satisfies MyWarehouses["items"][number];

describe("active warehouse (TEC-769)", () => {
  it("keys the local pick per organization", () => {
    expect(activeWarehouseKey("acme")).toBe("olex.warehouse.active.acme");
  });

  it("unrestricted: the stored pick, else the first warehouse", () => {
    const ctx: MyWarehouses = {
      restricted: false,
      default_warehouse_uuid: null,
      items: [wh("a", true), wh("b", true)],
    };
    expect(resolveActiveWarehouse(ctx, "b")).toBe("b");
    expect(resolveActiveWarehouse(ctx, null)).toBe("a");
    expect(resolveActiveWarehouse(ctx, "gone")).toBe("a");
    expect(resolveActiveWarehouse(undefined, "b")).toBeNull();
  });

  it("restricted: a non-member pick falls back to the member default", () => {
    const ctx: MyWarehouses = {
      restricted: true,
      default_warehouse_uuid: "c",
      items: [wh("a", false), wh("b", true), wh("c", true, true)],
    };
    expect(resolveActiveWarehouse(ctx, "a")).toBe("c");
    expect(resolveActiveWarehouse(ctx, "b")).toBe("b");
    expect(
      resolveActiveWarehouse({ ...ctx, default_warehouse_uuid: null }, null),
    ).toBe("b");
    expect(
      resolveActiveWarehouse(
        {
          restricted: true,
          default_warehouse_uuid: null,
          items: [wh("a", false)],
        },
        null,
      ),
    ).toBeNull();
  });
});
