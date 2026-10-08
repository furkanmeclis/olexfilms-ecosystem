// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";

import type { StockForecastRow } from "@/features/stock-forecast/services/stock-forecast.service";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  detail: vi.fn(),
  updateThreshold: vi.fn(),
  createOrderDraft: vi.fn(),
  networkDemand: vi.fn(),
  dealerSummary: vi.fn(),
  widget: vi.fn(),
}));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "dealer" as string | null,
}));
const captured = vi.hoisted(() => ({
  tables: [] as Record<string, unknown>[],
  exports: [] as Record<string, unknown>[],
  toasts: [] as string[],
}));

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
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "en",
    format: {
      number: (v: number) => String(v),
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
    state.orgType ? { slug: "acme", uuid: "org-1", type: state.orgType } : null,
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: {
    success: (message: string) => captured.toasts.push(message),
    error: (message: string) => captured.toasts.push(message),
  },
}));
vi.mock(
  "@/features/stock-forecast/services/stock-forecast.service",
  async (orig) => ({
    ...(await orig<object>()),
    stockForecastService: api,
  }),
);
vi.mock("@/features/io/components/export-menu", () => ({
  ExportMenu: (props: Record<string, unknown>) => {
    captured.exports.push(props);
    return null;
  },
}));
vi.mock("@/components/charts", () => ({
  AppChart: () => createElement("div", { "data-testid": "chart" }),
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (props: Record<string, unknown>) => {
    captured.tables.push(props);
    return createElement(
      "div",
      { "data-testid": "stock-forecast-table" },
      props.toolbarExtra as ReactNode,
    );
  },
}));

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import { StockForecastPage } from "./stock-forecast-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

function row(partial: Partial<StockForecastRow>): StockForecastRow {
  return {
    uuid: partial.uuid ?? "f1",
    product: partial.product ?? {
      uuid: "p1",
      sku: "SKU-1",
      name: "Film A",
      category: { uuid: "c1", name: "PPF" },
      unit_type: "piece",
    },
    on_hand_qty: partial.on_hand_qty ?? 4,
    avg_daily_30: partial.avg_daily_30 ?? "1.0",
    avg_daily_90: partial.avg_daily_90 ?? "0.8",
    seasonality_factor: partial.seasonality_factor ?? "1.0",
    days_left: partial.days_left ?? "4",
    depletion_date: partial.depletion_date ?? "2026-10-12",
    data_days: partial.data_days ?? 90,
    min_data_days: partial.min_data_days ?? 90,
    status: partial.status ?? "critical",
    suggested_qty: partial.suggested_qty ?? 10,
    suggested_meters: partial.suggested_meters ?? null,
    warning_days: partial.warning_days ?? 14,
    cover_days: partial.cover_days ?? 30,
    vehicles_left: partial.vehicles_left ?? null,
  };
}

let container: HTMLDivElement;
let root: Root | null;

function clearLocalStorage() {
  window.localStorage?.clear();
}

beforeEach(() => {
  clearLocalStorage();
  captured.tables = [];
  captured.exports = [];
  captured.toasts = [];
  state.grants = new Set([
    Permission.StockForecastRead,
    Permission.StockForecastManage,
    Permission.StockForecastNetworkRead,
  ]);
  state.orgType = "dealer";
  api.list.mockResolvedValue({
    items: [
      row({ uuid: "ok", status: "critical" }),
      row({
        uuid: "no",
        product: { uuid: "p2", sku: "SKU-2", name: "Film B" },
        status: "insufficient_data",
        data_days: 50,
        min_data_days: 90,
        suggested_qty: null,
      }),
    ],
    total: 2,
    limit: 20,
    offset: 0,
  });
  api.createOrderDraft.mockResolvedValue({ uuid: "o1", order_no: "ORD-1" });
  api.networkDemand.mockResolvedValue({
    items: [],
    total: 0,
    limit: 20,
    offset: 0,
  });
  api.dealerSummary.mockResolvedValue({ items: [] });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  if (root) {
    act(() => root?.unmount());
  }
  container?.remove();
  vi.clearAllMocks();
  root = null;
});

async function flush() {
  for (let i = 0; i < 6; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const currentRoot = root;
  if (!currentRoot) {
    throw new Error("test root is not initialized");
  }
  await act(async () => {
    currentRoot.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(StockForecastPage, { slug: "acme" }),
      ),
    );
  });
  await flush();
}

describe("StockForecastPage (TEC-486)", () => {
  it("does not allow selecting insufficient_data rows", async () => {
    await renderPage();
    const props = captured.tables[0]!;
    const canSelect = (
      props.features as { rowSelection: (r: unknown) => boolean }
    ).rowSelection;
    expect(canSelect({ original: row({ status: "critical" }) })).toBe(true);
    expect(canSelect({ original: row({ status: "insufficient_data" }) })).toBe(
      false,
    );
    expect(container.textContent).toContain(
      "stock_forecast.insufficient.banner",
    );
  });

  it("shows the created draft link after bulk draft creation", async () => {
    await renderPage();
    const props = captured.tables[0]!;
    const actions = props.bulkActions as {
      onClick: (rows: StockForecastRow[]) => void;
    }[];
    await act(async () => actions[0]!.onClick([row({ uuid: "ok" })]));
    await flush();
    const create = Array.from(document.body.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("stock_forecast.draft.create"),
    ) as HTMLButtonElement;
    await act(async () => create.click());
    await flush();
    expect(api.createOrderDraft).toHaveBeenCalledWith([
      { product_uuid: "p1", quantity: 10, meters: null },
    ]);
    const link = container.querySelector(
      '[data-testid="stock-forecast-draft-link"]',
    ) as HTMLAnchorElement | null;
    expect(link?.getAttribute("href")).toBe("/t/acme/orders/o1");
  });

  it("keeps the threshold cell read-only without manage permission", async () => {
    state.grants = new Set([Permission.StockForecastRead]);
    await renderPage();
    const props = captured.tables[0]!;
    const columns = props.columns as ColumnDef<StockForecastRow, unknown>[];
    const threshold = columns.find((col) => col.id === "cover_days");
    expect(threshold?.meta?.editVariant).toBeUndefined();
    expect(props.features).toMatchObject({ inlineEdit: false });
  });

  it("hides the menu item when the module is disabled", () => {
    const access = {
      can: (p: string | string[]) =>
        (Array.isArray(p) ? p : [p]).every((x) =>
          [Permission.StockRead, Permission.StockForecastRead].includes(
            x as never,
          ),
        ),
      canAny: () => false,
      org: { type: "dealer", role: "owner", features: ["stock"] },
    };
    const ids = tenantNav("acme").groups.flatMap((group) =>
      visibleNavItems(group, access).map((item) => item.id),
    );
    expect(ids).toContain("stock-mine");
    expect(ids).not.toContain("stock-forecast");
  });
});
