// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Order } from "@/features/orders/services/orders.service";

const api = vi.hoisted(() => ({
  claim: vi.fn(),
  releaseClaim: vi.fn(),
  setSchedule: vi.fn(),
  get: vi.fn(),
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock("@/lib/toast", () => ({ toast: toast, appToast: toast }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/features/orders/services/orders.service", async (orig) => ({
  ...(await orig<typeof import("@/features/orders/services/orders.service")>()),
  ordersService: api,
}));

import {
  OrderQueueBadges,
  OrderWorkCard,
} from "@/features/orders/components/order-work-queue";
import {
  shipmentDetails,
  shipmentDetailsValid,
} from "@/features/orders/lib/shipment";

function order(over: Partial<Order> = {}): Order {
  return {
    uuid: "o1",
    order_no: "ORD-1",
    status: "preparing",
    priority: "normal",
    due_date: null,
    is_overdue: false,
    picker: null,
    claimed_at: null,
    claimed_by_me: false,
    can_claim: true,
    can_take_over: false,
    can_edit_schedule: false,
    package_count: null,
    desi: null,
    weight_kg: null,
    ...over,
  } as Order;
}

let container: HTMLDivElement;
let root: Root;

async function render(node: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client: qc }, node));
  });
}
const $ = (sel: string) => container.querySelector(sel);

beforeEach(() => {
  (
    globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  vi.clearAllMocks();
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe("TEC-1228 work queue", () => {
  it("shows urgent, overdue and the holder", async () => {
    await render(
      createElement(OrderQueueBadges, {
        order: order({
          priority: "urgent",
          due_date: "2026-11-01",
          is_overdue: true,
          picker: { uuid: "u2", name: "Ayşe" },
        }),
      }),
    );
    expect($('[data-testid="order-urgent"]')).not.toBeNull();
    expect($('[data-testid="order-overdue"]')?.textContent).toContain(
      "orders.queue.overdue",
    );
    expect($('[data-testid="order-picker"]')?.textContent).toContain("Ayşe");
  });

  it("takes an unclaimed job", async () => {
    api.claim.mockResolvedValue(
      order({ picker: { uuid: "u1", name: "Ben" }, claimed_by_me: true }),
    );
    await render(createElement(OrderWorkCard, { order: order() }));
    const btn = $('[data-testid="order-claim"]') as HTMLButtonElement;
    await act(async () => btn.click());
    expect(api.claim).toHaveBeenCalledWith("o1", false);
    expect(toast.success).toHaveBeenCalledWith("orders.queue.claimed");
  });

  it("warns on someone else's job; a manager takes it over", async () => {
    const held = order({ picker: { uuid: "u2", name: "Ayşe" } });
    await render(createElement(OrderWorkCard, { order: held }));
    expect($('[data-testid="order-claim"]')).toBeNull();
    expect($('[data-testid="claim-warning"]')?.textContent).toContain("Ayşe");
    expect($('[data-testid="order-take-over"]')).toBeNull();
    expect($('[data-testid="order-release"]')).toBeNull();

    api.claim.mockResolvedValue(held);
    await render(
      createElement(OrderWorkCard, { order: { ...held, can_take_over: true } }),
    );
    const btn = $('[data-testid="order-take-over"]') as HTMLButtonElement;
    await act(async () => btn.click());
    expect(api.claim).toHaveBeenCalledWith("o1", true);
  });

  it("shows the shipment details", async () => {
    await render(
      createElement(OrderWorkCard, {
        order: order({
          status: "shipped",
          can_claim: false,
          package_count: 3,
          desi: "12.50",
        }),
      }),
    );
    const text = $('[data-testid="order-shipment-details"]')?.textContent ?? "";
    expect(text).toContain('orders.shipping.packages_value {"count":3}');
    expect(text).toContain("12.50");
  });

  it("parses the shipment details", () => {
    expect(shipmentDetailsValid("", "", "")).toBe(true);
    expect(shipmentDetailsValid("0", "", "")).toBe(false);
    expect(shipmentDetailsValid("2", "1,5", "x")).toBe(false);
    expect(shipmentDetails("2", "1,5", "")).toEqual({
      package_count: 2,
      desi: 1.5,
    });
  });
});
