// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Order } from "@/features/orders/services/orders.service";

import {
  BackorderBadge,
  OrderBackordersCard,
  receiptDiscrepancies,
  receiptMarksValid,
  shortLines,
} from "./order-partial";

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
  useLocale: () => ({ t: (key: string) => key }),
}));
vi.mock("@tanstack/react-query", () => ({
  useMutation: () => ({ mutate: vi.fn(), isPending: false }),
  useQueryClient: () => ({ setQueryData: vi.fn(), invalidateQueries: vi.fn() }),
}));

const order = (over: Partial<Order>): Order =>
  ({
    uuid: "o1",
    order_no: "SP1",
    items: [],
    ...over,
  }) as unknown as Order;

const item = (
  name: string,
  quantity: number,
  assigned: string,
  units: string[] = [],
) => ({
  uuid: name,
  product: { name },
  quantity,
  meters: null,
  assigned,
  units: units.map((u) => ({ unit_uuid: u, barcode: `B-${u}` })),
});

describe("TEC-1227 partial shipment helpers", () => {
  it("lists the short lines with the picked amount and the rest", () => {
    const o = order({
      items: [
        item("Film", 2, "1"),
        item("Plexi", 3, "3"),
        item("Kit", 1, "0"),
      ] as never,
    });
    expect(shortLines(o)).toEqual([
      { name: "Film", picked: 1, rest: 1 },
      { name: "Kit", picked: 0, rest: 1 },
    ]);
  });

  it("sends only reported units and needs a reason and one received unit", () => {
    const o = order({ items: [item("Film", 2, "2", ["u1", "u2"])] as never });
    const marks = {
      u1: { kind: "missing" as const, reason: " Yok " },
      u2: { kind: "ok" as const, reason: "" },
    };
    expect(receiptDiscrepancies(marks)).toEqual([
      { unit_uuid: "u1", kind: "missing", reason: "Yok" },
    ]);
    expect(receiptMarksValid(o, marks)).toBe(true);
    expect(
      receiptMarksValid(o, { u1: { kind: "damaged", reason: "  " } }),
    ).toBe(false);
    expect(
      receiptMarksValid(o, {
        u1: { kind: "missing", reason: "a" },
        u2: { kind: "missing", reason: "b" },
      }),
    ).toBe(false);
  });
});

let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe("TEC-1227 backorder links", () => {
  it("links the source order and the backorders", () => {
    const link = (uuid: string) => ({
      uuid,
      order_no: `NO-${uuid}`,
      status: "approved",
      status_label: "Approved",
    });
    act(() =>
      root.render(
        createElement(OrderBackordersCard, {
          slug: "acme",
          order: order({
            backorder_of: link("a"),
            backorders: [link("b")],
          } as never),
        }),
      ),
    );
    expect(
      container.querySelector('[data-testid="order-source-link"]')?.textContent,
    ).toBe("NO-a");
    expect(
      container.querySelector('[data-testid="order-backorder-link"]')
        ?.textContent,
    ).toBe("NO-b");
  });

  it("marks a backorder and hides the card without links", () => {
    act(() =>
      root.render(
        createElement(
          "div",
          null,
          createElement(BackorderBadge, { order: { is_backorder: true } }),
          createElement(OrderBackordersCard, {
            slug: "acme",
            order: order({}),
          }),
        ),
      ),
    );
    expect(
      container.querySelector('[data-testid="order-backorder-badge"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[data-testid="order-backorders"]'),
    ).toBeNull();
  });
});
