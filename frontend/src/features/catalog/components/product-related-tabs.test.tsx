// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  granted: new Set<string>(),
  orgType: "center" as "center" | "distributor" | "dealer",
  relatedCalls: [] as { kind: string; filter: Record<string, unknown> }[],
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    format: {
      number: (v: number) => String(v),
      date: (v: string) => `d(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (slug: string) => state.granted.has(slug) }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ uuid: "org-1", type: state.orgType }),
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  RelatedList: ({
    kind,
    filter,
  }: {
    kind: string;
    filter: Record<string, unknown>;
  }) => {
    state.relatedCalls.push({ kind, filter });
    return createElement("div", { "data-related": kind });
  },
}));
vi.mock("@/features/pricing/components/recommended-history", () => ({
  RecommendedHistory: ({ productUuid }: { productUuid: string }) =>
    createElement("div", { "data-price-history": productUuid }),
}));
vi.mock("@/features/stock/services/stock.service", async (orig) => ({
  ...(await orig<object>()),
  stockService: {
    listUnits: vi.fn().mockResolvedValue({
      items: [],
      total: 0,
      limit: 10,
      offset: 0,
    }),
  },
}));

import { Permission } from "@/config/permissions";

import {
  OPEN_ORDER_STATUSES,
  ProductRelatedTabs,
} from "./product-related-tabs";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.matchMedia ??= ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia;
  state.granted.clear();
  state.relatedCalls = [];
  state.orgType = "center";
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(ProductRelatedTabs, {
          slug: "olex",
          productUuid: "p-1",
        }) as ReactNode,
      ),
    );
  });
}

function tabLabels() {
  return [...container.querySelectorAll('[role="tab"]')].map(
    (el) => el.textContent,
  );
}

describe("ProductRelatedTabs (TEC-1218)", () => {
  it("renders nothing without any module permission", async () => {
    await render();
    expect(container.innerHTML).toBe("");
  });

  it("shows only the tabs the user may read; price history needs the price permission", async () => {
    state.granted = new Set([Permission.OrdersRead, Permission.WarrantiesRead]);
    await render();
    expect(tabLabels()).toEqual([
      "catalog.related.tabs.orders",
      "catalog.related.tabs.warranties",
    ]);
    expect(state.relatedCalls[0]).toEqual({
      kind: "order",
      filter: { product_uuid: "p-1", status: OPEN_ORDER_STATUSES.join(",") },
    });
    expect(container.querySelector("[data-price-history]")).toBeNull();
  });

  it("adds stock and price history for a center user with both permissions", async () => {
    state.granted = new Set([
      Permission.StockRead,
      Permission.PricingRecommendedRead,
    ]);
    await render();
    expect(tabLabels()).toEqual([
      "catalog.related.tabs.stock",
      "catalog.related.tabs.prices",
    ]);
  });

  it("hides price history outside the center", async () => {
    state.orgType = "dealer";
    state.granted = new Set([
      Permission.StockRead,
      Permission.PricingRecommendedRead,
    ]);
    await render();
    expect(tabLabels()).toEqual(["catalog.related.tabs.stock"]);
  });
});
