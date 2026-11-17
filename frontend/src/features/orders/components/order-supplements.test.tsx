// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Order } from "@/features/orders/services/orders.service";

import {
  AddSupplementButton,
  OrderSupplementsCard,
  SupplementBadge,
} from "./order-supplements";

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

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  (
    globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

function render(node: ReturnType<typeof createElement>) {
  act(() => root.render(node));
}

const base = {
  uuid: "main",
  order_no: "ORD-1",
  status: "shipped",
  is_supplement: false,
} as Order;

describe("order supplements (TEC-1222)", () => {
  it("shows the add button only when the server allows it", () => {
    render(createElement(AddSupplementButton, { slug: "acme", order: base }));
    expect(
      container.querySelector("[data-testid=order-add-supplement]"),
    ).toBeNull();
    render(
      createElement(AddSupplementButton, {
        slug: "acme",
        order: { ...base, can_add_supplement: true },
      }),
    );
    const link = container.querySelector("[data-testid=order-add-supplement]");
    expect(link?.getAttribute("href")).toBe(
      "/t/acme/orders/new?supplement_of=main",
    );
  });

  it("lists the supplements of a main order", () => {
    render(
      createElement(OrderSupplementsCard, {
        slug: "acme",
        order: {
          ...base,
          supplements: [
            {
              uuid: "s1",
              order_no: "ORD-2",
              status: "approved",
              status_label: "Approved",
            },
          ],
        },
      }),
    );
    expect(container.textContent).toContain("orders.supplements.title_list");
    const link = container.querySelector("[data-testid=order-supplement-link]");
    expect(link?.getAttribute("href")).toBe("/t/acme/orders/s1");
  });

  it("links a supplement to its main order and marks it", () => {
    const sup = {
      ...base,
      uuid: "s1",
      is_supplement: true,
      parent_order: {
        uuid: "main",
        order_no: "ORD-1",
        status: "shipped",
        status_label: "Shipped",
      },
    } as Order;
    render(createElement(OrderSupplementsCard, { slug: "acme", order: sup }));
    expect(container.textContent).toContain("orders.supplements.parent");
    expect(
      container
        .querySelector("[data-testid=order-parent-link]")
        ?.getAttribute("href"),
    ).toBe("/t/acme/orders/main");
    render(createElement(SupplementBadge, { order: sup }));
    expect(
      container.querySelector("[data-testid=order-supplement-badge]"),
    ).not.toBeNull();
  });

  it("renders nothing for a plain order", () => {
    render(createElement(OrderSupplementsCard, { slug: "acme", order: base }));
    expect(container.innerHTML).toBe("");
    render(createElement(SupplementBadge, { order: base }));
    expect(container.innerHTML).toBe("");
  });
});
