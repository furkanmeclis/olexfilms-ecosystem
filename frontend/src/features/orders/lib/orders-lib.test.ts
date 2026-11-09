import { describe, expect, it } from "vitest";

import {
  canAssignUnits,
  canEditDraft,
  orderActions,
  orderFullyAssigned,
  resolveOrderListAccess,
} from "@/features/orders/lib/access";
import {
  addLine,
  formIsValid,
  toItemInputs,
  unitPriceValid,
  validateOrderForm,
  type OrderFormLine,
} from "@/features/orders/lib/form";
import type { Order } from "@/features/orders/services/orders.service";

const grants =
  (...g: string[]) =>
  (p: string) =>
    g.includes(p);

function order(over: Partial<Order> = {}): Order {
  return {
    uuid: "o1",
    order_no: "ORD-00000001",
    created_by_seller: false,
    invoice_status: "pending",
    status: "draft",
    status_label: "Draft",
    role: "buyer",
    seller: { uuid: "s", name: "Merkez", type: "center" },
    buyer: { uuid: "b", name: "Dist", type: "distributor" },
    currency: "EUR",
    subtotal: "100.00",
    tax_total: "0.00",
    total: "100.00",
    rate_snapshot: null,
    try_rate: null,
    note: null,
    cancel_reason: null,
    submitted_at: null,
    approved_at: null,
    ready_at: null,
    shipped_at: null,
    cancelled_at: null,
    received_at: null,
    delivery_method: null,
    delivery_mode: null,
    tracking_no: null,
    tracking_url: null,
    has_shipping_document: false,
    has_receipt_document: false,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T08:00:00Z",
    available_transitions: [],
    ...over,
  };
}

const kinds = (o: Order) => orderActions(o).map((a) => a.kind);

describe("resolveOrderListAccess", () => {
  it("tabs follow the organization type", () => {
    const can = grants("orders.read", "orders.write", "catalog.read");
    expect(resolveOrderListAccess(can, "center").sides).toEqual(["seller"]);
    expect(resolveOrderListAccess(can, "dealer").sides).toEqual(["buyer"]);
    expect(resolveOrderListAccess(can, "distributor").sides).toEqual([
      "seller",
      "buyer",
    ]);
  });

  it("the center cannot create; others need orders.write and catalog.read", () => {
    const all = grants("orders.read", "orders.write", "catalog.read");
    expect(resolveOrderListAccess(all, "center").canCreate).toBe(false);
    expect(resolveOrderListAccess(all, "center").canCreateForDealer).toBe(true);
    expect(resolveOrderListAccess(all, "dealer").canCreateForDealer).toBe(
      false,
    );
    expect(resolveOrderListAccess(all, "dealer").canCreate).toBe(true);
    expect(
      resolveOrderListAccess(grants("orders.read", "orders.write"), "dealer")
        .canCreate,
    ).toBe(false);
    expect(resolveOrderListAccess(grants(), "dealer").canRead).toBe(false);
  });
});

describe("orderActions (role and status)", () => {
  it("buyer draft: submit and cancel", () => {
    expect(
      kinds(order({ available_transitions: ["submitted", "cancelled"] })),
    ).toEqual(["submit", "cancel"]);
  });

  it("seller submitted: approve and reject (reason required)", () => {
    const o = order({
      role: "seller",
      status: "submitted",
      available_transitions: ["approved", "cancelled"],
    });
    expect(kinds(o)).toEqual(["approve", "reject"]);
    expect(orderActions(o)[1].reasonRequired).toBe(true);
  });

  it("seller shipped to a memberless pool recipient: receive on its behalf (TEC-1062)", () => {
    expect(
      kinds(
        order({
          role: "seller",
          status: "shipped",
          receive_on_behalf: true,
          available_transitions: ["delivered", "received", "cancelling"],
        }),
      ),
    ).toEqual(["deliver", "receive_on_behalf", "request_cancel"]);
    // The buyer's own receive keeps its label.
    expect(
      kinds(
        order({
          role: "buyer",
          status: "shipped",
          available_transitions: ["received"],
        }),
      ),
    ).toEqual(["receive"]);
  });

  it("buyer submitted: cancel, not reject", () => {
    expect(
      kinds(
        order({ status: "submitted", available_transitions: ["cancelled"] }),
      ),
    ).toEqual(["cancel"]);
  });

  it("seller shipped: deliver and request cancel; buyer delivered: receive (TEC-1024)", () => {
    expect(
      kinds(
        order({
          role: "seller",
          status: "shipped",
          available_transitions: ["delivered", "cancelling"],
        }),
      ),
    ).toEqual(["deliver", "request_cancel"]);
    expect(
      kinds(
        order({
          role: "buyer",
          status: "delivered",
          available_transitions: ["received", "cancelling"],
        }),
      ),
    ).toEqual(["receive", "request_cancel"]);
  });

  it("hides processing", () => {
    expect(
      kinds(
        order({
          role: "seller",
          status: "approved",
          available_transitions: ["preparing", "processing", "cancelled"],
        }),
      ),
    ).toEqual(["start_preparing", "cancel"]);
  });

  it("seller processing: start preparing and cancel (TEC-261)", () => {
    expect(
      kinds(
        order({
          role: "seller",
          status: "processing",
          available_transitions: ["preparing", "cancelled"],
        }),
      ),
    ).toEqual(["start_preparing", "cancel"]);
  });

  it("buyer shipped: receive and request cancellation", () => {
    expect(
      kinds(
        order({
          status: "shipped",
          available_transitions: ["received", "cancelling"],
        }),
      ),
    ).toEqual(["receive", "request_cancel"]);
  });

  it("seller cancelling: complete cancellation", () => {
    expect(
      kinds(
        order({
          role: "seller",
          status: "cancelling",
          available_transitions: ["cancelled"],
        }),
      ),
    ).toEqual(["complete_cancel"]);
  });

  it("observer and received orders get no action", () => {
    expect(
      kinds(order({ role: "observer", available_transitions: ["submitted"] })),
    ).toEqual([]);
    expect(kinds(order({ status: "received" }))).toEqual([]);
  });

  it("draft edit and unit assignment follow side, status and permission", () => {
    const write = grants("orders.write");
    expect(canEditDraft(write, order())).toBe(true);
    expect(canEditDraft(write, order({ status: "submitted" }))).toBe(false);
    expect(canEditDraft(write, order({ role: "seller" }))).toBe(false);
    const prep = order({ role: "seller", status: "preparing" });
    expect(canAssignUnits(grants("orders.ship"), prep)).toBe(true);
    expect(canAssignUnits(grants("orders.approve"), prep)).toBe(false);
    expect(
      canAssignUnits(grants("orders.ship"), { ...prep, role: "buyer" }),
    ).toBe(false);
  });

  it("fully assigned needs every line covered", () => {
    const item = (
      assigned: string,
      quantity: number | null,
      meters: string | null,
    ) => ({
      uuid: assigned,
      product: {
        uuid: "p",
        sku: "S",
        name: "N",
        unit_type: meters ? "roll_meter" : "piece",
      },
      quantity,
      meters,
      unit_price: "1.0000",
      price_source: "list" as const,
      price_missing: false,
      line_total: "1.00",
      note: null,
      assigned,
      units: [],
      invoiced: false,
      invoice_no: null,
      invoice_uuid: null,
    });
    expect(orderFullyAssigned(order({ items: [item("3", 3, null)] }))).toBe(
      true,
    );
    expect(
      orderFullyAssigned(
        order({ items: [item("3", 3, null), item("10.00", null, "25.50")] }),
      ),
    ).toBe(false);
    expect(orderFullyAssigned(order({ items: [] }))).toBe(false);
  });
});

describe("order form validation", () => {
  const piece = { uuid: "p1", sku: "KIT", name: "Kit", unit_type: "piece" };
  const roll = { uuid: "p2", sku: "PPF", name: "PPF", unit_type: "roll_meter" };

  it("needs at least one line", () => {
    const e = validateOrderForm([]);
    expect(e.form).toBe("empty");
    expect(formIsValid(e)).toBe(false);
  });

  it("checks quantity and meters", () => {
    let lines: OrderFormLine[] = addLine(addLine([], piece), roll);
    expect(lines.map((l) => l.amount)).toEqual(["1", ""]);
    expect(validateOrderForm(lines).lines).toEqual({ p2: "meters_invalid" });
    lines = lines.map((l) =>
      l.product_uuid === "p1"
        ? { ...l, amount: "1.5" }
        : { ...l, amount: "12,5" },
    );
    expect(validateOrderForm(lines).lines).toEqual({ p1: "quantity_invalid" });
    lines = lines.map((l) =>
      l.product_uuid === "p1" ? { ...l, amount: "3" } : l,
    );
    expect(formIsValid(validateOrderForm(lines))).toBe(true);
    expect(toItemInputs(lines)).toEqual([
      { product_uuid: "p1", quantity: 3 },
      { product_uuid: "p2", meters: "12.5" },
    ]);
  });

  it("refuses zero, too large and too many decimals", () => {
    const line = (amount: string, unit_type = "piece"): OrderFormLine => ({
      product_uuid: "x",
      sku: "X",
      name: "X",
      unit_type,
      amount,
    });
    expect(validateOrderForm([line("0")]).lines.x).toBe("quantity_invalid");
    expect(validateOrderForm([line("1000001")]).lines.x).toBe(
      "quantity_invalid",
    );
    expect(validateOrderForm([line("0", "roll_meter")]).lines.x).toBe(
      "meters_invalid",
    );
    expect(validateOrderForm([line("1.234", "roll_meter")]).lines.x).toBe(
      "meters_invalid",
    );
  });

  it("adds a product once and caps the line count", () => {
    expect(addLine(addLine([], piece), piece)).toHaveLength(1);
    const many = Array.from({ length: 201 }, (_, i): OrderFormLine => ({
      product_uuid: `p${i}`,
      sku: "S",
      name: "N",
      unit_type: "piece",
      amount: "1",
    }));
    expect(validateOrderForm(many).form).toBe("too_many");
  });
});

describe("center order for a dealer (TEC-1024)", () => {
  const line = (over: Partial<OrderFormLine> = {}): OrderFormLine => ({
    product_uuid: "p1",
    sku: "P1",
    name: "P1",
    unit_type: "piece",
    amount: "2",
    ...over,
  });

  it("validates the optional unit price", () => {
    expect(unitPriceValid(undefined)).toBe(true);
    expect(unitPriceValid("")).toBe(true);
    expect(unitPriceValid("0")).toBe(true);
    expect(unitPriceValid("12,5")).toBe(true);
    expect(unitPriceValid("-1")).toBe(false);
    expect(unitPriceValid("1.23456")).toBe(false);
    expect(validateOrderForm([line({ unit_price: "abc" })]).lines).toEqual({
      p1: "price_invalid",
    });
  });

  it("sends the typed price only for the center path", () => {
    const lines = [
      line({ unit_price: "12,5" }),
      line({ product_uuid: "p2", unit_price: "" }),
    ];
    expect(toItemInputs(lines, true)).toEqual([
      { product_uuid: "p1", quantity: 2, unit_price: "12.5" },
      { product_uuid: "p2", quantity: 2 },
    ]);
    expect(toItemInputs(lines)).toEqual([
      { product_uuid: "p1", quantity: 2 },
      { product_uuid: "p2", quantity: 2 },
    ]);
  });
});
