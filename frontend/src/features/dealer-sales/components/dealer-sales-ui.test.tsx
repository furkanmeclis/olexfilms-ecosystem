// @vitest-environment jsdom
import { createElement, Fragment, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import {
  click,
  fill,
  flush,
  mount,
  render,
  submit,
  unmount,
  type Mounted,
} from "@/features/warehouse/components/test-helpers";

type AnyRow = Record<string, unknown>;

const captured = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<AnyRow>>[],
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  },
  access: {
    orgUuid: "dealer-1",
    enabled: true,
    canPrices: true,
    canSales: true,
    canSuppliers: true,
    canPurchases: true,
    canSeePurchasePrice: true,
  },
  service: {
    listPrices: vi.fn(),
    setPrice: vi.fn(),
    lookup: vi.fn(),
    listSales: vi.fn(),
    createSale: vi.fn(),
    voidSale: vi.fn(),
  },
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "en",
    format: {
      number: (v: number) => String(v),
      date: (v: string) => v,
      dateTime: (v: string) => v,
      currency: (v: number, c: string) => `${v.toFixed(2)} ${c}`,
    },
  }),
}));
vi.mock("@/providers/toast-provider", () => ({ appToast: captured.toast }));
vi.mock("@/components/ui/async-combobox", () => ({
  AsyncCombobox: () => null,
}));
vi.mock("@/features/dealer-sales/hooks/use-dealer-sales-access", () => ({
  useDealerSalesAccess: () => captured.access,
}));
vi.mock(
  "@/features/dealer-sales/services/dealer-sales.service",
  async (orig) => ({
    ...(await orig<object>()),
    dealerSalesService: captured.service,
  }),
);
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: ReactNode }) =>
    createElement(Fragment, null, children),
  EntityToolbar: () => null,
  EntityTable: (props: Partial<DataTableProps<AnyRow>>) => {
    captured.tables.push(props);
    return null;
  },
}));

import { QuickSaleForm } from "./quick-sale-form";
import { VoidSaleButton } from "./quick-sale-page";
import {
  SALE_PRICES_PERSIST_KEY,
  SalePriceCell,
  SalePricesPage,
} from "./sale-prices-page";

let m: Mounted;
beforeEach(() => {
  m = mount();
  captured.tables.length = 0;
});
afterEach(() => {
  unmount(m);
  vi.clearAllMocks();
});

const priceItem = (over: Record<string, unknown> = {}) => ({
  product_uuid: "p-1",
  sku: "F-1",
  name: "Film",
  uses_fixed_barcode: false,
  currency: "TRY",
  sale_price: "1200.00",
  recommended_sale_price: "1500.00",
  purchase_price: "800.00",
  estimated_profit: "400.00",
  updated_at: null,
  ...over,
});

const lookupItem = (barcode: string) => ({
  product_uuid: "p-1",
  sku: "F-1",
  name: "Film",
  barcode,
  unit_kind: "serial",
  quantity_on_hand: 1,
  currency: "TRY",
  sale_price: "1200.00",
  recommended_sale_price: "1500.00",
  purchase_price: "800.00",
});

const badge = () =>
  m.container.querySelector('[data-testid="below-cost-badge"]');

describe("SalePriceCell (TEC-348)", () => {
  it("shows a warning badge when the sale price is under the purchase price", async () => {
    await render(
      m,
      createElement(SalePriceCell, {
        item: priceItem({ sale_price: "700.00" }) as never,
      }),
    );
    expect(badge()).not.toBeNull();
    expect(badge()?.textContent).toContain("dealer_sales.prices.below_cost");
  });

  it("has no badge at or above the purchase price or without one", async () => {
    await render(
      m,
      createElement(SalePriceCell, { item: priceItem() as never }),
    );
    expect(badge()).toBeNull();
    await render(
      m,
      createElement(SalePriceCell, {
        item: priceItem({
          sale_price: "700.00",
          purchase_price: null,
        }) as never,
      }),
    );
    expect(badge()).toBeNull();
  });
});

describe("SalePricesPage (TEC-348)", () => {
  it("is a server DataTable with inline sale price edit", async () => {
    captured.service.listPrices.mockResolvedValue({
      items: [priceItem()],
      total: 1,
      limit: 20,
      offset: 0,
    });
    captured.service.setPrice.mockResolvedValue({});
    await render(m, createElement(SalePricesPage, { slug: "acme" }));
    const props = captured.tables.at(-1) as Partial<DataTableProps<AnyRow>> & {
      onCellEdit?: (e: {
        row: AnyRow;
        columnId: string;
        value: unknown;
      }) => void;
    };
    expect(props.features?.persistKey).toBe(SALE_PRICES_PERSIST_KEY);
    expect(props.features?.inlineEdit).toBe(true);
    expect(captured.service.listPrices).toHaveBeenCalledWith(
      expect.objectContaining({ sort: "name" }),
    );
    const ids = (props.columns ?? []).map(
      (c) => c.id ?? (c as { accessorKey?: string }).accessorKey,
    );
    expect(ids).toEqual(
      expect.arrayContaining([
        "purchase_price",
        "recommended_sale_price",
        "sale_price",
        "estimated_profit",
      ]),
    );

    props.onCellEdit?.({
      row: priceItem(),
      columnId: "sale_price",
      value: "abc",
    });
    expect(captured.service.setPrice).not.toHaveBeenCalled();
    props.onCellEdit?.({
      row: priceItem(),
      columnId: "sale_price",
      value: "950,5",
    });
    await flush();
    expect(captured.service.setPrice).toHaveBeenCalledWith("p-1", "950.50");
  });
});

describe("QuickSaleForm (TEC-348)", () => {
  const scanInput = () =>
    m.container.querySelector<HTMLInputElement>(
      '[data-testid="quick-sale-scan-input"]',
    );
  const scanForm = () => scanInput()?.closest("form") ?? null;
  const lines = () =>
    m.container.querySelectorAll('[data-testid="quick-sale-line"]');

  it("adds a line per scanned barcode and never the same barcode twice", async () => {
    captured.service.lookup.mockImplementation(
      async ({ barcode }: { barcode: string }) => lookupItem(barcode),
    );
    await render(
      m,
      createElement(QuickSaleForm, {
        currency: "TRY",
        canSeePurchasePrice: true,
      }),
    );
    expect(lines()).toHaveLength(0);

    await fill(scanInput(), "OLX-0001");
    await submit(scanForm());
    expect(captured.service.lookup).toHaveBeenCalledWith({
      barcode: "OLX-0001",
    });
    expect(lines()).toHaveLength(1);
    expect(lines()[0]?.textContent).toContain("OLX-0001");
    expect(
      m.container.querySelector('[data-testid="quick-sale-total"]')
        ?.textContent,
    ).toBe("1200.00 TRY");
    expect(
      m.container.querySelector('[data-testid="quick-sale-profit"]')
        ?.textContent,
    ).toBe("400.00 TRY");

    await fill(scanInput(), "OLX-0001");
    await submit(scanForm());
    expect(lines()).toHaveLength(1);
    expect(captured.service.lookup).toHaveBeenCalledTimes(1);
    expect(
      m.container.querySelector('[data-testid="quick-sale-notice"]')
        ?.textContent,
    ).toContain("dealer_sales.quick_sale.already_added");

    await fill(scanInput(), "OLX-0002");
    await submit(scanForm());
    expect(lines()).toHaveLength(2);
  });

  it("sends the scanned lines as one sale", async () => {
    captured.service.lookup.mockImplementation(
      async ({ barcode }: { barcode: string }) => lookupItem(barcode),
    );
    captured.service.createSale.mockResolvedValue({ uuid: "s-1" });
    const onSaved = vi.fn();
    await render(
      m,
      createElement(QuickSaleForm, {
        currency: "TRY",
        canSeePurchasePrice: true,
        onSaved,
      }),
    );
    await fill(scanInput(), "OLX-0001");
    await submit(scanForm());
    await click(m.container.querySelector('[data-testid="quick-sale-submit"]'));
    expect(captured.service.createSale).toHaveBeenCalledWith({
      customer_uuid: null,
      payment_method: "cash",
      note: undefined,
      lines: [{ barcode: "OLX-0001", quantity: 1, unit_price: "1200.00" }],
    });
    expect(onSaved).toHaveBeenCalled();
    expect(lines()).toHaveLength(0);
  });
});

describe("VoidSaleButton (TEC-348)", () => {
  const sale = (soldAt: Date, voided = false) => ({
    uuid: "s-1",
    sold_at: soldAt.toISOString(),
    payment_method: "cash" as const,
    currency: "TRY",
    total: "1200.00",
    profit: "400.00",
    customer_uuid: null,
    customer_name: null,
    products: "Film",
    line_count: 1,
    note: "",
    voided,
  });
  const button = () =>
    m.container.querySelector<HTMLButtonElement>(
      '[data-testid="void-sale-s-1"]',
    );
  const now = new Date(2026, 9, 7, 12, 0);

  it("is disabled for a sale of an earlier day", async () => {
    const onVoid = vi.fn();
    await render(
      m,
      createElement(VoidSaleButton, {
        sale: sale(new Date(2026, 9, 6, 17, 0)),
        now,
        onVoid,
      }),
    );
    expect(button()?.disabled).toBe(true);
    expect(button()?.title).toBe("dealer_sales.sales.void_window");
    await click(button());
    expect(onVoid).not.toHaveBeenCalled();
  });

  it("is enabled on the sale day and disabled once voided", async () => {
    const onVoid = vi.fn();
    await render(
      m,
      createElement(VoidSaleButton, {
        sale: sale(new Date(2026, 9, 7, 9, 0)),
        now,
        onVoid,
      }),
    );
    expect(button()?.disabled).toBe(false);
    await click(button());
    expect(onVoid).toHaveBeenCalledTimes(1);

    await render(
      m,
      createElement(VoidSaleButton, {
        sale: sale(new Date(2026, 9, 7, 9, 0), true),
        now,
        onVoid,
      }),
    );
    expect(button()?.disabled).toBe(true);
  });
});
