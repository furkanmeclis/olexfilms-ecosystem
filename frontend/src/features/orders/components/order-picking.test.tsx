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
  suggest: vi.fn(),
  applySuggestions: vi.fn(),
  pickLocations: vi.fn(),
  pickList: vi.fn(),
  regenerateShippingDocument: vi.fn(),
  downloadDocument: vi.fn(),
  downloadOrderSlip: vi.fn(),
  attachReceipt: vi.fn(),
}));
const methods = vi.hoisted(() => ({
  list: vi.fn(),
  listActive: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
}));
const catalog = vi.hoisted(() => ({ listProducts: vi.fn() }));
const pricing = vi.hoisted(() => ({ getProduct: vi.fn() }));
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
vi.mock("sonner", () => ({ toast }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "tr",
    timeZone: "Europe/Istanbul",
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
    state.orgType ? { slug: "acme", type: state.orgType } : null,
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
  deliveryMethodsService: methods,
}));
vi.mock("@/lib/api/platform-form-request", async (orig) => ({
  ...(await orig<object>()),
  triggerBrowserDownload: vi.fn(),
}));
vi.mock("@/features/catalog/services/catalog.service", async (orig) => ({
  ...(await orig<object>()),
  catalogService: catalog,
}));
vi.mock("@/features/catalog/services/pricing.service", async (orig) => ({
  ...(await orig<object>()),
  pricingService: pricing,
}));

import type { Order } from "@/features/orders/services/orders.service";

import { validTrackingTemplate } from "./delivery-methods-page";
import { OrderDetailPage } from "./order-detail-page";
import { suggestionAmount } from "./order-picking";
import { canOpenPickList } from "../lib/access";

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

async function click(el: Element | null | undefined) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
  await flush();
}

const $ = (sel: string) => container.querySelector(sel);
function order(over: Partial<Order> = {}): Order {
  return {
    uuid: "o1",
    order_no: "ORD-00000001",
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
        line_total: "240.00",
        note: null,
        assigned: "0",
        units: [],
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

const grantsAll = [
  "orders.read",
  "orders.write",
  "orders.approve",
  "orders.ship",
  "orders.receive",
  "orders.cancel",
];

const suggestion = {
  unit_uuid: "u1",
  barcode: "BC-1",
  unit_kind: "serial" as const,
  quantity: 1,
  meters: null,
  remaining_meters: null,
  located: true,
  location_uuid: "l1",
  location_code: "D1-R1-A-01-01",
  entered_at: "2026-01-01T00:00:00Z",
};

describe("TEC-768: suggest and fill", () => {
  beforeEach(() => {
    state.grants = new Set(grantsAll);
  });

  it("proposes units, toggles the unlocated pool and applies them", async () => {
    api.get.mockResolvedValue(
      order({
        role: "seller",
        status: "preparing",
        available_transitions: ["ready", "cancelled"],
      }),
    );
    api.suggest.mockImplementation(
      async (_o: string, _i: string, unlocated: boolean) => ({
        item_uuid: "i1",
        missing: "3",
        covered: unlocated ? "3" : "1",
        complete: unlocated,
        include_unlocated: unlocated,
        suggestions: unlocated
          ? [
              suggestion,
              { ...suggestion, unit_uuid: "u2", barcode: "BC-2" },
              {
                ...suggestion,
                unit_uuid: "u3",
                barcode: "BC-3",
                located: false,
                location_uuid: null,
                location_code: null,
              },
            ]
          : [suggestion],
      }),
    );
    api.applySuggestions.mockResolvedValue(
      order({ role: "seller", status: "preparing" }),
    );
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    await click($("[data-testid=suggest-open]"));
    expect(api.suggest).toHaveBeenCalledWith("o1", "i1", false);
    expect(document.querySelectorAll("[data-testid=suggestion]")).toHaveLength(
      1,
    );
    await click(document.querySelector("[data-testid=suggest-unlocated]"));
    expect(api.suggest).toHaveBeenLastCalledWith("o1", "i1", true);
    const rows = document.querySelectorAll("[data-testid=suggestion]");
    expect(rows).toHaveLength(3);
    expect(rows[2].textContent).toContain("orders.picking.no_location");
    await click(document.querySelector("[data-testid=suggest-apply]"));
    expect(api.applySuggestions).toHaveBeenCalledWith(
      "o1",
      "i1",
      expect.arrayContaining([expect.objectContaining({ unit_uuid: "u3" })]),
    );
    expect(toast.success).toHaveBeenCalledWith(
      'orders.suggest.applied {"count":3}',
    );
  });

  it("the buyer sees no picking actions", async () => {
    api.get.mockResolvedValue(
      order({ role: "buyer", status: "preparing", available_transitions: [] }),
    );
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    expect($("[data-testid=suggest-open]")).toBeNull();
    expect($("[data-testid=order-pick-list]")).toBeNull();
  });

  it("the seller opens the pick list while preparing", async () => {
    api.get.mockResolvedValue(
      order({ role: "seller", status: "preparing", available_transitions: [] }),
    );
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    expect($("[data-testid=order-pick-list]")?.getAttribute("href")).toBe(
      "/t/acme/orders/o1/pick-list",
    );
  });
});

describe("TEC-768: shipping", () => {
  beforeEach(() => {
    state.grants = new Set(grantsAll);
  });

  it("ship needs a delivery method before confirming", async () => {
    api.get.mockResolvedValue(
      order({
        role: "seller",
        status: "ready",
        available_transitions: ["shipped", "cancelled"],
      }),
    );
    methods.listActive.mockResolvedValue([
      {
        uuid: "m1",
        code: "warehouse_pickup",
        name: "Ambar teslim",
        tracking_url_template: null,
        active: true,
        sort_order: 0,
        is_system: true,
        created_at: "2026-10-01T08:00:00Z",
        updated_at: "2026-10-01T08:00:00Z",
      },
    ]);
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    await click($("[data-action=ship]"));
    expect(document.querySelector("[data-testid=ship-fields]")).not.toBeNull();
    expect(methods.listActive).toHaveBeenCalled();
    const confirm = document.querySelector(
      "[data-testid=action-confirm]",
    ) as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);
  });

  it("downloads the order slip in the selected language and timezone before shipping", async () => {
    api.get.mockResolvedValue(order({ status: "draft", role: "buyer" }));
    api.downloadOrderSlip.mockResolvedValue({
      blob: new Blob(["pdf"]),
      filename: "order-slip-ORD-1.pdf",
    });
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    await click($("[data-testid=order-slip-download]"));
    expect(api.downloadOrderSlip).toHaveBeenCalledWith(
      "o1",
      "tr",
      "Europe/Istanbul",
    );
  });

  it("shows the tracking link and downloads the shipping note", async () => {
    api.get.mockResolvedValue(
      order({
        role: "seller",
        status: "shipped",
        shipped_at: "2026-10-02T08:00:00Z",
        delivery_method: {
          uuid: "m2",
          code: "aras",
          name: "Aras Kargo",
          is_system: false,
        },
        tracking_no: "AR123",
        tracking_url: "https://t.example/AR123",
        has_shipping_document: true,
      }),
    );
    api.downloadDocument.mockResolvedValue({
      blob: new Blob(["%PDF"]),
      filename: "shipping-note-ORD-00000001.pdf",
    });
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    expect($("[data-testid=shipping-method]")?.textContent).toBe("Aras Kargo");
    expect($("[data-testid=tracking-link]")?.getAttribute("href")).toBe(
      "https://t.example/AR123",
    );
    await click($("[data-testid=shipping-document-download]"));
    expect(api.downloadDocument).toHaveBeenCalledWith("o1", "shipping");
    expect($("[data-testid=shipping-document-regenerate]")).not.toBeNull();
    // Receipt upload is the buyer's.
    expect($("[data-testid=receipt-input]")).toBeNull();
  });

  it("the buyer attaches a delivery receipt after receiving", async () => {
    api.get.mockResolvedValue(
      order({
        role: "buyer",
        status: "received",
        shipped_at: "2026-10-02T08:00:00Z",
        has_shipping_document: true,
      }),
    );
    api.attachReceipt.mockResolvedValue(
      order({ role: "buyer", status: "received", has_receipt_document: true }),
    );
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    expect($("[data-testid=shipping-document-regenerate]")).toBeNull();
    const input = $("[data-testid=receipt-input]") as HTMLInputElement;
    const file = new File(["%PDF"], "r.pdf", { type: "application/pdf" });
    Object.defineProperty(input, "files", { value: [file] });
    await act(async () => {
      input.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await flush();
    expect(api.attachReceipt).toHaveBeenCalledWith("o1", file);
  });
});

describe("TEC-768 helpers", () => {
  it("validates tracking URL templates", () => {
    expect(validTrackingTemplate("")).toBe(true);
    expect(validTrackingTemplate("https://a.example/q?no={tracking_no}")).toBe(
      true,
    );
    expect(validTrackingTemplate("ftp://a.example/{tracking_no}")).toBe(false);
    expect(validTrackingTemplate("not a url")).toBe(false);
  });

  it("formats suggestion amounts", () => {
    const t = (k: string) => k;
    expect(suggestionAmount({ meters: "12.00", quantity: null }, t)).toBe(
      "12.00 orders.unit.m",
    );
    expect(suggestionAmount({ meters: null, quantity: 4 }, t)).toBe(
      "4 orders.unit.pcs",
    );
  });

  it("opens the pick list for the seller with orders.ship only", () => {
    const can = (p: string) => p === "orders.ship";
    expect(
      canOpenPickList(can, order({ role: "seller", status: "ready" })),
    ).toBe(true);
    expect(
      canOpenPickList(can, order({ role: "seller", status: "shipped" })),
    ).toBe(false);
    expect(
      canOpenPickList(() => false, order({ role: "seller", status: "ready" })),
    ).toBe(false);
  });
});
