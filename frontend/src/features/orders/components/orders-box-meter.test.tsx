// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  create: vi.fn(),
  replaceItems: vi.fn(),
  transition: vi.fn(),
  assignUnit: vi.fn(),
  unassignUnit: vi.fn(),
  setPlotterCut: vi.fn(),
  quote: vi.fn(),
}));
const catalog = vi.hoisted(() => ({ listProducts: vi.fn() }));
const pricing = vi.hoisted(() => ({ getProduct: vi.fn() }));
const warehouses = vi.hoisted(() => ({ listWarehouses: vi.fn() }));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "distributor" as string | null,
}));
const nav = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

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
vi.mock("next/navigation", () => ({ useRouter: () => nav }));
vi.mock("@/lib/toast", () => ({ toast: toast, appToast: toast }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      currency: (v: number, c: string) => `${v} ${c}`,
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
    state.orgType ? { uuid: "c1", slug: "acme", type: state.orgType } : null,
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/components/ui/date-picker", () => ({
  DatePicker: ({
    id,
    value,
    onChange,
  }: {
    id?: string;
    value?: string;
    onChange?: (v: string) => void;
  }) =>
    createElement("input", {
      id,
      value: value ?? "",
      onChange: (e: { target: { value: string } }) =>
        onChange?.(e.target.value),
    }),
}));
vi.mock("@/features/orders/services/orders.service", async (orig) => ({
  ...(await orig<object>()),
  ordersService: api,
}));
vi.mock("@/features/catalog/services/catalog.service", async (orig) => ({
  ...(await orig<object>()),
  catalogService: catalog,
}));
vi.mock("@/features/catalog/services/pricing.service", async (orig) => ({
  ...(await orig<object>()),
  pricingService: pricing,
}));
vi.mock("@/features/warehouse/services/warehouse.service", async (orig) => ({
  ...(await orig<object>()),
  warehouseService: warehouses,
}));

import type { Order } from "@/features/orders/services/orders.service";

import { OrderDetailPage } from "./order-detail-page";
import { OrderFormPage } from "./order-form-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
// DataTable reads the mobile breakpoint (jsdom has no matchMedia).
window.matchMedia ??= ((query: string) => ({
  matches: false,
  media: query,
  onchange: null,
  addEventListener: () => {},
  removeEventListener: () => {},
  addListener: () => {},
  removeListener: () => {},
  dispatchEvent: () => false,
})) as typeof window.matchMedia;
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
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.clearAllMocks();
  state.grants = new Set();
  state.orgType = "distributor";
});

async function flush() {
  for (let i = 0; i < 5; i++) {
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

async function type(el: Element | null, value: string) {
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

async function click(el: Element | null | undefined) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
  await flush();
}

const $ = (sel: string) => container.querySelector(sel);
const $$ = (sel: string) => Array.from(container.querySelectorAll(sel));

function order(over: Partial<Order> = {}): Order {
  return {
    uuid: "o1",
    order_no: "ORD-00000001",
    created_by_seller: false,
    invoice_status: "pending",
    can_edit_items: false,
    is_supplement: false,
    status: "draft",
    status_label: "Draft",
    role: "buyer",
    seller: { uuid: "s", name: "Olex Merkez", type: "center" },
    buyer: { uuid: "b", name: "Olex Dist", type: "distributor" },
    currency: "EUR",
    subtotal: "240.00",
    tax_total: "0.00",
    total: "240.00",
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
    priority: "normal",
    due_date: null,
    is_overdue: false,
    picker: null,
    claimed_at: null,
    claimed_by_me: false,
    can_claim: false,
    can_take_over: false,
    can_edit_schedule: false,
    package_count: null,
    desi: null,
    weight_kg: null,
    total_quantity: 1,
    total_meters: null,
    line_count: 1,
    live_try_rate: null,
    live_total_try: null,
    last_activity: null,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T08:00:00Z",
    available_transitions: [],
    items: [
      {
        uuid: "i1",
        product: {
          uuid: "p1",
          sku: "KIT-1",
          name: "Bakım kiti",
          unit_type: "piece",
        },
        quantity: 3,
        meters: null,
        unit_price: "80.0000",
        price_source: "list",
        price_missing: false,
        list_unit_price: null,
        discount_pct: null,
        is_gift: false,
        plotter_cut: false,
        sale_measure: "box" as const,
        line_credits: null,
        line_total: "240.00",
        note: null,
        assigned: "0",
        units: [],
        invoiced: false,
        invoice_no: null,
        invoice_uuid: null,
      },
    ],
    history: [
      {
        from_status: null,
        to_status: "draft",
        reason: null,
        created_at: "2026-10-01T08:00:00Z",
      },
    ],
    ...over,
  };
}

/** A piece product sold by the meter: 15 m boxes, 2 m minimum cut. */
const FILM = {
  uuid: "f1",
  sku: "BLK-PPF",
  name: "Black PPF",
  unit_type: "piece",
  usage_credits: 5,
  roll: {
    default_meters: 15,
    width_cm: 152,
    min_cut_meters: 2,
    waste_pct: null,
    sellable_by_meter: true,
  },
};

const filmItem = (
  measure: "box" | "meter",
  over: Record<string, unknown> = {},
) => ({
  ...order().items![0],
  uuid: `i-${measure}`,
  product: {
    uuid: "f1",
    sku: "BLK-PPF",
    name: "Black PPF",
    unit_type: "piece",
    sellable_by_meter: true,
    box_meters: "15.00",
    min_cut_meters: "2.00",
  },
  quantity: measure === "box" ? 1 : null,
  meters: measure === "meter" ? "3.00" : null,
  sale_measure: measure,
  line_credits: measure === "meter" ? 5 : null,
  unit_price: measure === "box" ? "360.0000" : "27.0000",
  line_total: measure === "box" ? "360.00" : "81.00",
  ...over,
});

describe("TEC-1239: box and meter lines on the order form", () => {
  beforeEach(() => {
    state.grants = new Set([
      "orders.read",
      "orders.write",
      "catalog.read",
      "pricing.purchase.read",
    ]);
    state.orgType = "dealer";
    catalog.listProducts.mockResolvedValue({
      items: [
        FILM,
        { uuid: "p1", sku: "KIT-1", name: "Bakım kiti", unit_type: "piece" },
      ],
      total: 2,
      limit: 20,
      offset: 0,
    });
    pricing.getProduct.mockResolvedValue({ prices: [] });
    api.quote.mockResolvedValue({
      currency: "USD",
      prices_visible: true,
      items: [
        {
          product_uuid: "f1",
          unit_price: "360.0000",
          sellable_by_meter: true,
          meter_unit_price: "27.0000",
          meter_list_unit_price: null,
          list_unit_price: null,
          price_source: "list",
        },
      ],
    });
  });

  it("adds 1 box + 3 m of one product, checks the cut and sends both lines", async () => {
    api.create.mockResolvedValue(order());
    await render(createElement(OrderFormPage, { slug: "acme" }));
    await type($("#order-product-search"), "p");
    // A product sold by the meter offers +Box and +Meter; others one button.
    expect($$("[data-testid=product-add-box]")).toHaveLength(1);
    expect($$("[data-testid=product-add-meter]")).toHaveLength(1);
    expect($$("[data-testid=product-add]")).toHaveLength(1);
    await click($("[data-testid=product-add-box]"));
    await click($("[data-testid=product-add-meter]"));
    // Each measure once: both buttons are now "added".
    expect(
      ($("[data-testid=product-add-box]") as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(
      ($("[data-testid=product-add-meter]") as HTMLButtonElement).disabled,
    ).toBe(true);
    const lines = $$("[data-testid=order-line]");
    expect(lines.map((l) => l.getAttribute("data-measure"))).toEqual([
      "box",
      "meter",
    ]);
    expect(
      $$("[data-testid=line-measure]").map((b) =>
        b.getAttribute("data-measure"),
      ),
    ).toEqual(["box", "meter"]);
    // Meter line hints: minimum cut and the default credits 5 (K3).
    expect($("[data-testid=line-min-cut]")?.textContent).toContain(
      '"meters":"2"',
    );
    expect(($("[data-testid=line-credits]") as HTMLInputElement).value).toBe(
      "5",
    );

    // 0 m and below the minimum cut are refused.
    await click($("[data-testid=save-draft]"));
    expect(api.create).not.toHaveBeenCalled();
    await type($("#line-f1-meter"), "1,5");
    await click($("[data-testid=save-draft]"));
    expect(api.create).not.toHaveBeenCalled();
    expect($("[data-testid=line-error]")?.textContent).toContain(
      "orders.form.errors.below_min_cut",
    );

    // The 0.5 m step: 1.5 -> 2 -> 2.5 -> 3.
    await click($("[data-testid=meters-step-up]"));
    await click($("[data-testid=meters-step-up]"));
    await click($("[data-testid=meters-step-up]"));
    expect(($("#line-f1-meter") as HTMLInputElement).value).toBe("3");
    expect($("[data-testid=line-boxes]")?.textContent).toContain(
      '"boxes":"0.2"',
    );
    expect($("[data-testid=line-meter-price]")?.textContent).toContain(
      '"total":"81 USD"',
    );
    expect($("[data-testid=whole-box-warning]")).toBeNull();

    await click($("[data-testid=save-draft]"));
    expect(api.create).toHaveBeenCalledWith({
      items: [
        { product_uuid: "f1", quantity: 1, sale_measure: "box" },
        {
          product_uuid: "f1",
          meters: "3",
          sale_measure: "meter",
          line_credits: 5,
        },
      ],
    });
  });

  it("warns that a whole box is better and converts in one click", async () => {
    await render(createElement(OrderFormPage, { slug: "acme" }));
    await type($("#order-product-search"), "p");
    await click($("[data-testid=product-add-meter]"));
    await type($("#line-f1-meter"), "18");
    expect($("[data-testid=whole-box-warning]")).not.toBeNull();
    await click($("[data-testid=whole-box-convert]"));
    // 18 m = 1 box + 3 m (3 m is still a valid cut).
    expect(
      $$("[data-testid=order-line]").map((l) => [
        l.getAttribute("data-measure"),
        (l.querySelector("input[name=amount]") as HTMLInputElement).value,
      ]),
    ).toEqual([
      ["box", "1"],
      ["meter", "3"],
    ]);
    expect($("[data-testid=whole-box-warning]")).toBeNull();
  });

  it("edits a saved draft with both lines and keeps a meter line whose switch was turned off", async () => {
    const draft = order({
      items: [
        filmItem("box"),
        filmItem("meter", {
          product: { ...filmItem("meter").product, sellable_by_meter: false },
        }),
      ] as never,
      can_edit_items: true,
    });
    state.grants = new Set(["orders.read", "orders.write", "catalog.read"]);
    api.get.mockResolvedValue(draft);
    api.replaceItems.mockResolvedValue(draft);
    await render(createElement(OrderFormPage, { slug: "acme", uuid: "o1" }));
    expect(
      $$("[data-testid=order-line]").map((l) => l.getAttribute("data-measure")),
    ).toEqual(["box", "meter"]);
    expect($("[data-testid=line-meter-stale]")).not.toBeNull();
    expect(($("#line-f1-meter") as HTMLInputElement).value).toBe("3.00");
    await type($("#line-f1-meter"), "4");
    await type($("#line-f1-meter-credits"), "7");
    await click($("[data-testid=save-draft]"));
    expect(api.replaceItems).toHaveBeenCalledWith("o1", [
      {
        product_uuid: "f1",
        quantity: 1,
        sale_measure: "box",
        plotter_cut: false,
      },
      {
        product_uuid: "f1",
        meters: "4",
        sale_measure: "meter",
        line_credits: 7,
        plotter_cut: false,
      },
    ]);
  });

  it("shows the measure, the meters, the credits and the cut barcodes on the detail", async () => {
    state.grants = new Set(["orders.read", "pricing.purchase.read"]);
    api.get.mockResolvedValue(
      order({
        status: "shipped",
        items: [
          filmItem("box", {
            assigned: "1",
            units: [
              {
                unit_uuid: "u1",
                barcode: "BOX-1",
                unit_kind: "serial",
                quantity: 1,
                meters: null,
                shipped: true,
                assigned_at: "2026-10-01T08:00:00Z",
              },
            ],
          }),
          filmItem("meter", {
            assigned: "3.00",
            units: [
              {
                unit_uuid: "u2",
                barcode: "CUT-1",
                unit_kind: "serial",
                quantity: 1,
                meters: "3.00",
                shipped: true,
                assigned_at: "2026-10-01T08:00:00Z",
              },
            ],
          }),
        ] as never,
      }),
    );
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    expect($$("[data-testid=order-item]")).toHaveLength(2);
    expect(
      $$("[data-testid=order-items] [data-testid=line-measure]").map((b) =>
        b.getAttribute("data-measure"),
      ),
    ).toEqual(["box", "meter"]);
    expect($("[data-testid=line-credits-value]")?.textContent).toContain(
      '"credits":5',
    );
    const text = $("[data-testid=order-items]")?.textContent ?? "";
    expect(text).toContain("3 orders.unit.m");
    expect(text).toContain("1 orders.unit.pcs");
    expect($$("[data-testid=assigned-unit]").map((u) => u.textContent)).toEqual(
      ["BOX-1", "CUT-1 3.00 m"],
    );
  });
});
