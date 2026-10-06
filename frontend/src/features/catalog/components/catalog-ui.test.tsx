// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "center" as string,
  priceView: null as unknown,
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), back: vi.fn() }),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    format: {
      number: (v: number | null) => (v === null ? "—" : String(v)),
      dateTime: () => "date",
      currency: (v: number | null, c: string) =>
        v === null ? "—" : `${v.toFixed(2)} ${c}`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({
    uuid: "o1",
    slug: "acme",
    name: "Acme",
    role: "owner",
    status: "active",
    type: state.orgType,
  }),
}));
vi.mock("@/hooks/use-mobile", () => ({ useIsMobile: () => false }));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/features/catalog/services/catalog.service", async (orig) => ({
  ...(await orig<object>()),
  catalogService: {
    getProduct: () =>
      Promise.resolve({
        uuid: "p1",
        category: { uuid: "c1", name: "PPF" },
        sku: "PPF-190",
        name: "Olex PPF 190",
        description_md: "",
        warranty_duration_months: 120,
        micron_thickness: 190,
        images: [],
        unit_type: "roll_meter",
        uses_fixed_barcode: true,
        active: true,
        external_id: null,
        locked_fields: [],
        created_at: "2026-10-01T00:00:00Z",
        updated_at: "2026-10-01T00:00:00Z",
      }),
  },
}));
vi.mock("@/features/catalog/services/pricing.service", () => ({
  pricingService: {
    getProduct: () => Promise.resolve(state.priceView),
    listDistributorPrices: () =>
      Promise.resolve({ items: [], total: 0, limit: 100, offset: 0 }),
    listDistributors: () => Promise.resolve([]),
  },
}));

import { PriceTable } from "./price-table";
import { ProductDetailPage } from "./product-detail-page";

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
  state.grants = new Set();
});

async function render(node: ReturnType<typeof createElement>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  // Let the mocked queries resolve (product, then its price view).
  for (let i = 0; i < 100; i++) {
    if (container.querySelector("[data-testid=price-table]")) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
}

// Price cells carry `data-price-column` (one row per currency → unique).
const columns = () => [
  ...new Set(
    Array.from(container.querySelectorAll("[data-price-column]")).map((el) =>
      el.getAttribute("data-price-column"),
    ),
  ),
];
const buttonLabels = () =>
  Array.from(container.querySelectorAll("button, a")).map(
    (el) => el.getAttribute("aria-label") ?? el.textContent?.trim() ?? "",
  );

describe("PriceTable mask", () => {
  it("dealer sees only its purchase price", async () => {
    await render(
      createElement(PriceTable, {
        view: {
          product_uuid: "p1",
          sku: "S",
          name: "N",
          viewer: "dealer",
          prices: [
            {
              currency: "TRY",
              purchase_price: "100.00",
              purchase_price_source: "distributor",
            },
          ],
        },
      }),
    );
    expect(columns()).toEqual(["purchase_price"]);
    expect(container.textContent).toContain("100.00 TRY");
    expect(container.textContent).not.toContain("catalog.prices.recommended");
    expect(buttonLabels()).not.toContain("common.edit");
    expect(buttonLabels()).not.toContain("common.delete");
  });

  it("distributor sees its purchase and dealer price, not the center's", async () => {
    await render(
      createElement(PriceTable, {
        view: {
          product_uuid: "p1",
          sku: "S",
          name: "N",
          viewer: "distributor",
          prices: [
            {
              currency: "EUR",
              purchase_price: "8.00",
              purchase_price_source: "override",
              sale_price: "9.50",
            },
          ],
        },
        canEdit: true,
      }),
    );
    expect(columns()).toEqual(["purchase_price", "sale_price"]);
    expect(container.textContent).toContain("catalog.prices.my_purchase");
    expect(container.textContent).toContain("catalog.prices.sources.override");
    expect(container.textContent).not.toContain(
      "catalog.prices.center_purchase",
    );
  });

  it("center sees the three list columns", async () => {
    await render(
      createElement(PriceTable, {
        view: {
          product_uuid: "p1",
          sku: "S",
          name: "N",
          viewer: "center",
          prices: [
            {
              currency: "TRY",
              purchase_price: "5",
              purchase_price_source: "own",
              sale_price: "8",
              recommended_sale_price: "12",
            },
          ],
        },
      }),
    );
    expect(columns()).toEqual([
      "purchase_price",
      "sale_price",
      "recommended_sale_price",
    ]);
  });
});

describe("ProductDetailPage write controls", () => {
  const centerView = {
    product_uuid: "p1",
    sku: "PPF-190",
    name: "Olex PPF 190",
    viewer: "center",
    prices: [{ currency: "TRY", purchase_price: "5", sale_price: "8" }],
  };

  it("center with catalog.write and pricing.sale.write sees edit controls", async () => {
    state.orgType = "center";
    state.priceView = centerView;
    state.grants = new Set([
      "catalog.read",
      "catalog.write",
      "pricing.purchase.read",
      "pricing.sale.read",
      "pricing.sale.write",
    ]);
    await render(
      createElement(ProductDetailPage, { slug: "acme", uuid: "p1" }),
    );
    const labels = buttonLabels();
    expect(labels).toContain("common.edit");
    expect(labels).toContain("common.delete");
    expect(labels).toContain("catalog.prices.add_currency");
    expect(labels).toContain("catalog.distributor_prices.add");
  });

  it("dealer reads the product and its purchase price only", async () => {
    state.orgType = "dealer";
    state.priceView = {
      ...centerView,
      viewer: "dealer",
      prices: [
        {
          currency: "TRY",
          purchase_price: "11",
          purchase_price_source: "distributor",
        },
      ],
    };
    // Even a stray write grant does not unlock writes outside the center.
    state.grants = new Set([
      "catalog.read",
      "catalog.write",
      "pricing.purchase.read",
    ]);
    await render(
      createElement(ProductDetailPage, { slug: "acme", uuid: "p1" }),
    );
    expect(container.textContent).toContain("Olex PPF 190");
    const labels = buttonLabels();
    expect(labels).not.toContain("common.edit");
    expect(labels).not.toContain("common.delete");
    expect(labels).not.toContain("catalog.prices.add_currency");
    expect(
      container.querySelector("[data-testid=distributor-prices-card]"),
    ).toBeNull();
    expect(columns()).toEqual(["purchase_price"]);
  });

  it("distributor may set its dealer price but not the catalog", async () => {
    state.orgType = "distributor";
    state.priceView = {
      ...centerView,
      viewer: "distributor",
      prices: [{ currency: "TRY", purchase_price: "8", sale_price: "9" }],
    };
    state.grants = new Set([
      "catalog.read",
      "pricing.purchase.read",
      "pricing.sale.read",
      "pricing.sale.write",
    ]);
    await render(
      createElement(ProductDetailPage, { slug: "acme", uuid: "p1" }),
    );
    const labels = buttonLabels();
    expect(labels).toContain("catalog.prices.set_dealer_price");
    expect(labels).not.toContain("catalog.distributor_prices.add");
    expect(
      container.querySelector("a[href='/t/acme/catalog/products/p1/edit']"),
    ).toBeNull();
  });
});
