// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { BrandCurrencyChange } from "@/features/brand-currency/services/brand-currency.service";

const captured = vi.hoisted(() => ({
  service: {
    list: vi.fn(),
    changeCurrency: vi.fn(),
    convertPrices: vi.fn(),
  },
  admin: true,
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, vars?: Record<string, unknown>) =>
      vars ? `${key} ${JSON.stringify(vars)}` : key,
    locale: "en",
    format: {
      number: (v: number) => String(v),
      currency: (v: number, c: string) => `${v} ${c}`,
    },
  }),
}));
vi.mock("@/lib/toast", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/lib/auth/session-store", () => ({
  useAuthStore: (sel: (s: { user: { isSuperAdmin: boolean } }) => unknown) =>
    sel({ user: { isSuperAdmin: captured.admin } }),
}));
vi.mock("@/lib/currency", () => ({
  useCurrencyList: () => ({ data: ["EUR", "TRY", "USD"] }),
}));
vi.mock("@/features/brand-currency/services/brand-currency.service", () => ({
  brandCurrencyKeys: {
    all: ["platform", "brands"],
    list: ["platform", "brands", "list"],
  },
  brandCurrencyService: captured.service,
}));

import { BrandCurrencyPage } from "./brand-currency-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const preview: BrandCurrencyChange = {
  brand: {
    uuid: "b1",
    slug: "olex",
    name: "Olex",
    status: "active",
    currency: "TRY",
  },
  from: "TRY",
  to: "EUR",
  local_currency: "TRY",
  open_orders_option: "block",
  active_products: 3,
  missing_price_count: 1,
  missing_prices: [{ product_uuid: "p1", sku: "PPF-1", name: "Film" }],
  open_order_count: 1,
  open_orders: [
    {
      uuid: "o1",
      order_no: "ORD-1",
      status: "draft",
      currency: "TRY",
      total: "100.00",
      seller_name: "Center",
      buyer_name: "Dealer",
      created_at: "2026-11-17T10:00:00Z",
    },
  ],
  rate_to_local: {
    base: "EUR",
    quote: "TRY",
    rate: "48",
    rate_date: "2026-11-17",
    source: "tcmb",
  },
  conversion_rate: null,
  blockers: ["open_orders", "missing_prices"],
  dry_run: true,
  applied: false,
  repriced_orders: 0,
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  captured.admin = true;
  for (const fn of Object.values(captured.service)) fn.mockReset();
  captured.service.list.mockResolvedValue({
    items: [preview.brand],
    local_currency: "TRY",
  });
  captured.service.changeCurrency.mockResolvedValue(preview);
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function flush() {
  for (let i = 0; i < 4; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(BrandCurrencyPage),
      ),
    );
  });
  await flush();
}

const byTestId = (id: string) =>
  container.querySelector<HTMLButtonElement>(`[data-testid="${id}"]`);

describe("BrandCurrencyPage (TEC-1214)", () => {
  it("previews a change: blockers, missing prices and open orders; apply stays disabled", async () => {
    await render();
    expect(container.textContent).toContain("Olex");
    await act(async () => {
      byTestId("brand-currency-preview-olex")?.click();
    });
    await flush();
    expect(captured.service.changeCurrency).toHaveBeenCalledWith("b1", {
      currency: "EUR",
      open_orders: "block",
      allow_missing_prices: false,
      dry_run: true,
    });
    expect(container.textContent).toContain(
      "rates.brand_currency.blocker.open_orders",
    );
    expect(container.textContent).toContain(
      "rates.brand_currency.blocker.missing_prices",
    );
    expect(byTestId("brand-currency-missing")?.textContent).toContain("PPF-1");
    expect(byTestId("brand-currency-orders")?.textContent).toContain("ORD-1");
    expect(byTestId("brand-currency-apply-olex")?.disabled).toBe(true);
  });

  it("is read-only for a non-admin", async () => {
    captured.admin = false;
    await render();
    expect(container.textContent).toContain("rates.brand_currency.admin_only");
    expect(byTestId("brand-currency-preview-olex")).toBeNull();
  });
});
