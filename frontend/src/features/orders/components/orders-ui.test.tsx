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

import { OrderDetailPage } from "./order-detail-page";
import { OrderFormPage } from "./order-form-page";
import { OrdersListPage } from "./orders-list-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

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
const actions = () =>
  $$("[data-action]").map((b) => b.getAttribute("data-action"));

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

describe("OrderDetailPage: buttons by role and status", () => {
  beforeEach(() => {
    state.grants = new Set([
      "orders.read",
      "orders.write",
      "orders.approve",
      "orders.ship",
      "orders.receive",
      "orders.cancel",
    ]);
  });

  it("buyer draft: edit, submit, cancel", async () => {
    api.get.mockResolvedValue(
      order({ available_transitions: ["submitted", "cancelled"] }),
    );
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    expect(actions()).toEqual(["submit", "cancel"]);
    expect($("[data-testid=order-edit]")?.getAttribute("href")).toBe(
      "/t/acme/orders/o1/edit",
    );
    expect($("[data-testid=order-rate]")?.textContent).toBe(
      "orders.detail.rate_pending",
    );
    expect($$("[data-testid=history-row]")).toHaveLength(1);
  });

  it("seller submitted: approve and reject, no edit", async () => {
    api.get.mockResolvedValue(
      order({
        role: "seller",
        status: "submitted",
        available_transitions: ["approved", "cancelled"],
      }),
    );
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    expect(actions()).toEqual(["approve", "reject"]);
    expect($("[data-testid=order-edit]")).toBeNull();
  });

  it("observer sees no action bar", async () => {
    api.get.mockResolvedValue(
      order({ role: "observer", available_transitions: ["submitted"] }),
    );
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    expect($("[data-testid=order-actions]")).toBeNull();
  });

  it("seller preparing: barcode form, ready disabled until assigned", async () => {
    api.get.mockResolvedValue(
      order({
        role: "seller",
        status: "preparing",
        available_transitions: ["ready", "cancelled"],
      }),
    );
    api.assignUnit.mockResolvedValue(
      order({
        role: "seller",
        status: "preparing",
        available_transitions: ["ready", "cancelled"],
        items: [
          {
            ...order().items![0],
            assigned: "3",
            units: [
              {
                unit_uuid: "u1",
                barcode: "FIX-1",
                unit_kind: "fixed",
                quantity: 3,
                meters: null,
                shipped: false,
                assigned_at: "2026-10-01T09:00:00Z",
              },
            ],
          },
        ],
      }),
    );
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    const ready = $("[data-action=mark_ready]") as HTMLButtonElement;
    expect(ready.disabled).toBe(true);
    await type($('input[name="barcode"]'), "FIX-1");
    await type($('input[name="quantity"]'), "3");
    await click($("[data-testid=assign-submit]"));
    expect(api.assignUnit).toHaveBeenCalledWith("o1", "i1", {
      barcode: "FIX-1",
      quantity: 3,
    });
    expect(($("[data-action=mark_ready]") as HTMLButtonElement).disabled).toBe(
      false,
    );
    expect($("[data-testid=assign-form]")).toBeNull();
  });

  it("seller preparing roll: meters split the roll, new barcode in the toast", async () => {
    const rollItem = {
      ...order().items![0],
      product: {
        uuid: "p2",
        sku: "PPF-190",
        name: "PPF",
        unit_type: "roll_meter",
      },
      quantity: null,
      meters: "12.00",
    };
    const preparing = {
      role: "seller" as const,
      status: "preparing" as const,
      available_transitions: ["ready", "cancelled"] as never,
    };
    api.get.mockResolvedValue(order({ ...preparing, items: [rollItem] }));
    api.assignUnit.mockResolvedValue(
      order({
        ...preparing,
        items: [{ ...rollItem, assigned: "12.00" }],
        split: {
          uuid: "s1",
          meters: "12.00",
          source_unit_uuid: "r1",
          source_barcode: "ROLL-1",
          source_remaining_meters: "38.00",
          new_unit_uuid: "n1",
          new_barcode: "ROLL-1-S1",
          replayed: false,
        },
      }),
    );
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    expect($('input[name="quantity"]')).toBeNull();
    await type($('input[name="barcode"]'), "ROLL-1");
    await type($('input[name="meters"]'), "12");
    await click($("[data-testid=assign-submit]"));
    const body = api.assignUnit.mock.calls.at(-1)![2];
    expect(body).toMatchObject({ barcode: "ROLL-1", meters: "12" });
    expect(typeof body.idempotency_key).toBe("string");
    expect(toast.success).toHaveBeenCalledWith(
      'orders.assign.split_success {"barcode":"ROLL-1-S1","rest":"38.00"}',
    );
  });

  it("buyer shipped: receive confirms through the dialog", async () => {
    api.get.mockResolvedValue(
      order({
        status: "shipped",
        available_transitions: ["received", "cancelling"],
      }),
    );
    api.transition.mockResolvedValue(order({ status: "received" }));
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    expect(actions()).toEqual(["receive", "request_cancel"]);
    await click($("[data-action=receive]"));
    await click(document.querySelector("[data-testid=action-confirm]"));
    expect(api.transition).toHaveBeenCalledWith("o1", "received", undefined);
    expect(toast.success).toHaveBeenCalledWith(
      "orders.actions.receive.success",
    );
  });

  it("reject needs a reason before confirming", async () => {
    api.get.mockResolvedValue(
      order({
        role: "seller",
        status: "submitted",
        available_transitions: ["approved", "cancelled"],
      }),
    );
    api.transition.mockResolvedValue(order({ status: "cancelled" }));
    await render(createElement(OrderDetailPage, { slug: "acme", uuid: "o1" }));
    await click($("[data-action=reject]"));
    const confirm = document.querySelector(
      "[data-testid=action-confirm]",
    ) as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);
    const textarea = document.querySelector(
      "#order-reason",
    ) as HTMLTextAreaElement;
    const setter = Object.getOwnPropertyDescriptor(
      HTMLTextAreaElement.prototype,
      "value",
    )?.set;
    await act(async () => {
      setter?.call(textarea, "Stok yok");
      textarea.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await flush();
    expect(confirm.disabled).toBe(false);
    await click(confirm);
    expect(api.transition).toHaveBeenCalledWith("o1", "cancelled", "Stok yok");
  });
});

describe("OrderFormPage: validation", () => {
  beforeEach(() => {
    state.grants = new Set(["orders.read", "orders.write", "catalog.read"]);
    state.orgType = "dealer";
    catalog.listProducts.mockResolvedValue({
      items: [
        { uuid: "p1", sku: "KIT-1", name: "Bakım kiti", unit_type: "piece" },
        { uuid: "p2", sku: "PPF-190", name: "PPF", unit_type: "roll_meter" },
      ],
      total: 2,
      limit: 10,
      offset: 0,
    });
  });

  it("is forbidden for the center", async () => {
    state.orgType = "center";
    await render(createElement(OrderFormPage, { slug: "acme" }));
    expect($("[data-testid=save-draft]")).toBeNull();
    expect(container.textContent).toContain("orders.form.forbidden");
  });

  it("blocks an empty order and invalid amounts, then creates the draft", async () => {
    api.create.mockResolvedValue(order());
    await render(createElement(OrderFormPage, { slug: "acme" }));
    await click($("[data-testid=save-draft]"));
    expect(api.create).not.toHaveBeenCalled();
    expect($("[data-testid=lines-empty]")?.className).toContain(
      "text-destructive",
    );

    await type($("#order-product-search"), "ki");
    expect(catalog.listProducts).toHaveBeenCalledWith({
      q: "ki",
      active: true,
      limit: 10,
    });
    const adds = $$("[data-testid=product-add]");
    await click(adds[0]);
    await click(adds[1]);
    expect($$("[data-testid=order-line]")).toHaveLength(2);

    // Roll line starts empty: meters are required.
    await click($("[data-testid=save-draft]"));
    expect(api.create).not.toHaveBeenCalled();
    expect($$("[data-testid=line-error]").map((e) => e.textContent)).toEqual([
      "orders.form.errors.meters_invalid",
    ]);

    await type($("#line-p1"), "0");
    await type($("#line-p2"), "12,5");
    expect($$("[data-testid=line-error]").map((e) => e.textContent)).toEqual([
      "orders.form.errors.quantity_invalid",
    ]);
    await type($("#line-p1"), "3");
    await click($("[data-testid=save-draft]"));
    expect(api.create).toHaveBeenCalledWith({
      items: [
        { product_uuid: "p1", quantity: 3 },
        { product_uuid: "p2", meters: "12.5" },
      ],
    });
    expect(nav.replace).toHaveBeenCalledWith("/t/acme/orders/o1/edit");
  });

  it("submit saves then moves the draft to submitted", async () => {
    api.create.mockResolvedValue(order());
    api.transition.mockResolvedValue(order({ status: "submitted" }));
    await render(createElement(OrderFormPage, { slug: "acme" }));
    await type($("#order-product-search"), "kit");
    await click($$("[data-testid=product-add]")[0]);
    await click($("[data-testid=submit-order]"));
    expect(api.create).toHaveBeenCalledTimes(1);
    expect(api.transition).toHaveBeenCalledWith("o1", "submitted");
    expect(nav.push).toHaveBeenCalledWith("/t/acme/orders/o1");
  });
});

describe("OrdersListPage: filters", () => {
  beforeEach(() => {
    state.grants = new Set(["orders.read", "orders.write", "catalog.read"]);
    api.list.mockResolvedValue({
      items: [order({ status: "submitted", status_label: "Gönderildi" })],
      total: 45,
      limit: 20,
      offset: 0,
    });
  });
  const lastQuery = () => api.list.mock.calls.at(-1)?.[0];

  it("distributor: incoming and outgoing tabs", async () => {
    await render(createElement(OrdersListPage, { slug: "acme" }));
    expect(lastQuery()).toEqual({ side: "seller", limit: 20, offset: 0 });
    expect($$("[role=tab]")).toHaveLength(2);
    await click($("[data-testid=side-buyer]"));
    expect(lastQuery()?.side).toBe("buyer");
    expect($("[data-testid=new-order]")).not.toBeNull();
  });

  it("center: incoming only, no new order", async () => {
    state.orgType = "center";
    await render(createElement(OrdersListPage, { slug: "acme" }));
    expect($$("[role=tab]")).toHaveLength(0);
    expect(lastQuery()?.side).toBe("seller");
    expect($("[data-testid=new-order]")).toBeNull();
  });

  it("dealer: outgoing only", async () => {
    state.orgType = "dealer";
    await render(createElement(OrdersListPage, { slug: "acme" }));
    expect(lastQuery()?.side).toBe("buyer");
  });

  it("status, dates and paging reach the query", async () => {
    await render(createElement(OrdersListPage, { slug: "acme" }));
    await click($('[data-status="shipped"]'));
    expect(lastQuery()?.status).toBe("shipped");
    await type($("#order-from"), "2026-10-01");
    await type($("#order-to"), "2026-10-02");
    expect(lastQuery()?.created_from).toBeTruthy();
    expect(lastQuery()?.created_to).toBeTruthy();
    await click($("[data-testid=page-next]"));
    expect(lastQuery()?.offset).toBe(20);

    await type($("#order-to"), "2026-09-01");
    expect($("[data-testid=date-error]")).not.toBeNull();
    expect(lastQuery()?.created_to).toBeUndefined();

    await click($("[data-testid=clear-filters]"));
    expect(lastQuery()).toEqual({ side: "seller", limit: 20, offset: 0 });
    expect($$("[data-testid=order-row]")).toHaveLength(1);
  });

  it("is forbidden without orders.read", async () => {
    state.grants = new Set();
    await render(createElement(OrdersListPage, { slug: "acme" }));
    expect(api.list).not.toHaveBeenCalled();
    expect(container.textContent).toContain("orders.list.forbidden");
  });
});
