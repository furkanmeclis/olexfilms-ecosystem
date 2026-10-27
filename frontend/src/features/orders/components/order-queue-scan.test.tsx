// @vitest-environment jsdom
import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
  useParams: () => ({ slug: "acme" }),
  useSearchParams: () => new URLSearchParams(""),
}));

import { lineForProduct } from "@/features/orders/components/order-detail-page";
import {
  activeQueue,
  ORDER_QUEUES,
} from "@/features/orders/components/order-queue-chips";
import { orderActions } from "@/features/orders/lib/access";
import type {
  Order,
  OrderItem,
} from "@/features/orders/services/orders.service";

function item(product: string, quantity: number, assigned: number): OrderItem {
  return {
    uuid: `i-${product}-${quantity}`,
    product: { uuid: product, name: product, sku: product, unit_type: "piece" },
    quantity,
    meters: null,
    assigned: String(assigned),
    units: [],
  } as unknown as OrderItem;
}

describe("order queue chips (TEC-1032)", () => {
  it("maps the status filter back to a queue", () => {
    expect(activeQueue([])).toBe("all");
    expect(activeQueue(["processing", "approved"])).toBe("prepare");
    expect(activeQueue(["ready"])).toBe("ready");
    expect(activeQueue(["ready", "shipped"])).toBeNull();
    expect(ORDER_QUEUES.map((q) => q.id)).toEqual([
      "all",
      "prepare",
      "preparing",
      "ready",
      "shipped",
      "delivered",
    ]);
  });
});

describe("order-level scan (TEC-1032)", () => {
  it("picks the first open line of the scanned product", () => {
    const order = {
      items: [item("p1", 2, 2), item("p1", 3, 1), item("p2", 1, 0)],
    } as unknown as Order;
    expect(lineForProduct(order, "p1")?.uuid).toBe("i-p1-3");
    expect(lineForProduct(order, "p2")?.uuid).toBe("i-p2-1");
    expect(lineForProduct(order, "p3")).toBeNull();
    const full = { items: [item("p1", 2, 2)] } as unknown as Order;
    expect(lineForProduct(full, "p1")).toBeNull();
  });
});

describe("ready -> preparing (TEC-1032)", () => {
  it("offers the seller 'back to preparing' from ready", () => {
    const order = {
      role: "seller",
      status: "ready",
      available_transitions: ["preparing", "shipped", "cancelled"],
    } as unknown as Order;
    expect(orderActions(order).map((a) => a.kind)).toEqual([
      "back_to_preparing",
      "ship",
      "cancel",
    ]);
    const approved = {
      role: "seller",
      status: "approved",
      available_transitions: ["preparing"],
    } as unknown as Order;
    expect(orderActions(approved)[0]?.kind).toBe("start_preparing");
  });
});
