// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  create: vi.fn(),
  quote: vi.fn(),
  listChildDealers: vi.fn(),
  listChildBuyers: vi.fn(),
  getDiscount: vi.fn(),
  setDiscount: vi.fn(),
}));
const catalog = vi.hoisted(() => ({ listProducts: vi.fn() }));
const warehouses = vi.hoisted(() => ({ listWarehouses: vi.fn() }));
const state = vi.hoisted(() => ({ grants: new Set<string>() }));
const nav = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
}));

vi.mock("next/navigation", () => ({ useRouter: () => nav }));
vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: unknown }) =>
    createElement("a", { href }, children as never),
}));
vi.mock("sonner", () => ({ toast }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      currency: (v: number, c: string) => `${v} ${c}`,
      date: (v: string) => `date(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ uuid: "c1", slug: "acme", type: "center" }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
// A plain button stands in for the searchable picker.
vi.mock("@/components/ui/async-combobox", () => ({
  AsyncCombobox: ({
    onValueChange,
    "data-testid": testId,
  }: {
    onValueChange: (v: string) => void;
    "data-testid"?: string;
  }) =>
    createElement(
      "button",
      {
        type: "button",
        "data-testid": testId,
        onClick: () => onValueChange("d1"),
      },
      "pick",
    ),
}));
vi.mock("@/features/orders/services/orders.service", async (orig) => ({
  ...(await orig<object>()),
  ordersService: api,
}));
vi.mock("@/features/catalog/services/catalog.service", async (orig) => ({
  ...(await orig<object>()),
  catalogService: catalog,
}));
vi.mock("@/features/warehouse/services/warehouse.service", async (orig) => ({
  ...(await orig<object>()),
  warehouseService: warehouses,
}));

import { CenterOrderForm } from "./center-order-form";
import { DealerDiscountsPage } from "./dealer-discounts-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
(
  globalThis as typeof globalThis & { ResizeObserver?: typeof ResizeObserver }
).ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as typeof ResizeObserver;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  warehouses.listWarehouses.mockRejectedValue(new Error("forbidden"));
  catalog.listProducts.mockResolvedValue({
    items: [
      { uuid: "p1", sku: "FILM-1", name: "Film", unit_type: "piece" },
      { uuid: "p2", sku: "PLEKSI", name: "Pleksi seti", unit_type: "piece" },
    ],
    total: 2,
    limit: 20,
    offset: 0,
  });
  api.quote.mockImplementation(
    ({ product_uuids }: { product_uuids: string[] }) =>
      Promise.resolve({
        currency: "USD",
        prices_visible: true,
        first_order: true,
        discount_pct: "20.00",
        standard_discount_pct: "15.00",
        first_order_discount_pct: "20.00",
        try_rate: "35",
        rate_date: "2026-11-09",
        items: product_uuids.map((u) => ({
          product_uuid: u,
          list_unit_price: u === "p1" ? "100.0000" : "40.0000",
          unit_price: u === "p1" ? "80.0000" : "32.0000",
          price_source: "list",
        })),
      }),
  );
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.clearAllMocks();
  state.grants = new Set();
});

async function flush() {
  for (let i = 0; i < 6; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render(node: ReturnType<typeof createElement>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await flush();
}

async function click(el: Element | null | undefined) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
  await flush();
}

async function type(el: Element | null | undefined, value: string) {
  if (!(el instanceof HTMLInputElement)) throw new Error("input not found");
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  await act(async () => {
    setter?.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await flush();
}

const $ = (sel: string) => container.querySelector(sel);
const $$ = (sel: string) => Array.from(container.querySelectorAll(sel));

describe("center order prices and discounts (TEC-1064)", () => {
  it("shows list price, first-order discount, totals in USD and TRY; gift and typed discount reach the API", async () => {
    state.grants = new Set([
      "orders.write",
      "pricing.sale.read",
      "catalog.read",
    ]);
    api.create.mockResolvedValue({ uuid: "o1", order_no: "ORD-1", items: [] });
    await render(
      createElement(CenterOrderForm, { slug: "acme", centerUuid: "c1" }),
    );
    await click($("[data-testid=center-order-dealer]"));
    const adds = $$("[data-testid=product-add]");
    await click(adds[0]);
    await click(adds[1]);
    expect($$("[data-testid=order-line]")).toHaveLength(2);
    expect(
      $("[data-testid=center-order-discount-info]")?.textContent,
    ).toContain("orders.center_form.discount_first_order");
    const lists = $$("[data-testid=line-list-price]").map((e) => e.textContent);
    expect(lists).toEqual(["100 USD", "40 USD"]);
    const unit = $$("[data-testid=line-discounted-price]").map(
      (e) => e.textContent,
    );
    expect(unit).toEqual(["80 USD", "32 USD"]);
    expect($("[data-testid=center-order-total]")?.textContent).toBe("112 USD");
    expect($("[data-testid=center-order-total-try]")?.textContent).toContain(
      "3920 TRY",
    );

    // A typed discount on the film, the plexi set as a gift.
    await type($$("[data-testid=line-discount]")[0], "25");
    await click($$("[data-testid=line-gift]")[1]);
    expect($("[data-testid=center-order-total]")?.textContent).toBe("75 USD");
    await click($("[data-testid=center-order-create]"));
    expect(api.create).toHaveBeenCalledWith({
      buyer_org_uuid: "d1",
      items: [
        { product_uuid: "p1", quantity: 1, discount_pct: "25" },
        { product_uuid: "p2", quantity: 1, is_gift: true },
      ],
    });
  });

  it("hides prices without pricing.sale.read and keeps the typed unit price", async () => {
    state.grants = new Set(["orders.write", "catalog.read"]);
    await render(
      createElement(CenterOrderForm, { slug: "acme", centerUuid: "c1" }),
    );
    await click($("[data-testid=center-order-dealer]"));
    await click($$("[data-testid=product-add]")[0]);
    expect(api.quote).not.toHaveBeenCalled();
    expect($("[data-testid=center-order-totals]")).toBeNull();
    expect($("[data-testid=line-unit-price]")).not.toBeNull();
    expect($("[data-testid=line-list-price]")?.textContent).toBe("—");
  });
});

describe("dealer discounts page (TEC-1064)", () => {
  it("is forbidden without pricing.sale.write", async () => {
    await render(createElement(DealerDiscountsPage, { slug: "acme" }));
    expect(container.textContent).toContain("orders.discounts.forbidden");
  });

  it("loads and saves the discounts of a dealer", async () => {
    state.grants = new Set(["pricing.sale.write"]);
    const value = {
      organization: { uuid: "d1", name: "Bayi", type: "dealer" },
      standard_discount_pct: "15.00",
      first_order_discount_pct: null,
      first_order: true,
    };
    api.getDiscount.mockResolvedValue(value);
    api.setDiscount.mockResolvedValue({
      ...value,
      first_order_discount_pct: "20.00",
    });
    await render(createElement(DealerDiscountsPage, { slug: "acme" }));
    await click($("[data-testid=discount-buyer]"));
    expect(api.getDiscount).toHaveBeenCalledWith("d1");
    expect(
      ($("[data-testid=discount-standard]") as HTMLInputElement).value,
    ).toBe("15.00");
    expect($("[data-testid=discount-first-state]")?.textContent).toBe(
      "orders.discounts.state_first",
    );
    await type($("[data-testid=discount-first]"), "120");
    expect($("[data-testid=discount-error]")).not.toBeNull();
    await type($("[data-testid=discount-first]"), "20");
    await click($("[data-testid=discount-save]"));
    expect(api.setDiscount).toHaveBeenCalledWith("d1", {
      standard_discount_pct: "15.00",
      first_order_discount_pct: "20",
    });
  });
});
