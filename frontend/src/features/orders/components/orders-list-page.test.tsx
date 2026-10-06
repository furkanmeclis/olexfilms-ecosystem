// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Order } from "@/features/orders/services/orders.service";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  listOrganizations: vi.fn(),
  transition: vi.fn(),
}));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "distributor" as string | null,
}));
const captured = vi.hoisted(() => ({
  tables: [] as Record<string, unknown>[],
  exports: [] as Record<string, unknown>[],
  push: vi.fn(),
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
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: captured.push }),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: {
      number: (v: number) => String(v),
      currency: (v: number, c: string) => `${v} ${c}`,
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
vi.mock("@/features/orders/services/orders.service", async (orig) => ({
  ...(await orig<object>()),
  ordersService: api,
}));
vi.mock("@/features/io/components/export-menu", () => ({
  ExportMenu: (props: Record<string, unknown>) => {
    captured.exports.push(props);
    return null;
  },
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (props: Record<string, unknown>) => {
    captured.tables.push(props);
    return createElement(
      "div",
      { "data-testid": "orders-table" },
      props.toolbarExtra as ReactNode,
    );
  },
}));

import { OrdersListPage } from "./orders-list-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

type TableProps = {
  columns: ColumnDef<Order, unknown>[];
  rowCount?: number;
  state?: {
    onColumnFiltersChange?: (v: { id: string; value: unknown }[]) => void;
    onSortingChange?: (v: { id: string; desc: boolean }[]) => void;
    onGlobalFilterChange?: (v: string) => void;
  };
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  captured.exports = [];
  state.grants = new Set([
    "orders.read",
    "orders.write",
    "catalog.read",
    "organizations.read",
  ]);
  state.orgType = "distributor";
  api.list.mockResolvedValue({ items: [], total: 45, limit: 20, offset: 0 });
  api.listOrganizations.mockResolvedValue([
    { uuid: "d1", name: "Bayi 1" },
    { uuid: "d2", name: "Bayi 2" },
  ]);
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
});

async function flush() {
  for (let i = 0; i < 5; i++) {
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
        createElement(OrdersListPage, { slug: "acme" }),
      ),
    );
  });
  await flush();
}

const lastTable = () =>
  captured.tables[captured.tables.length - 1] as unknown as TableProps;
const lastQuery = () => api.list.mock.calls.at(-1)?.[0];
const column = (id: string) =>
  lastTable().columns.find(
    (c) => c.id === id || ("accessorKey" in c && c.accessorKey === id),
  );

function order(over: Partial<Order> = {}): Order {
  return {
    uuid: "o1",
    order_no: "ORD-1",
    status: "draft",
    status_label: "Draft",
    role: "buyer",
    seller: { uuid: "s", name: "Merkez", type: "center" },
    buyer: { uuid: "b", name: "Dist", type: "distributor" },
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
    ...over,
  };
}

/** Row action ids of the actions cell for one order. */
function rowActions(o: Order): string[] {
  const cell = column("actions")?.cell as (ctx: unknown) => ReactElement<{
    actions: { id: string }[];
  }>;
  return cell({ row: { original: o } }).props.actions.map((a) => a.id);
}

describe("OrdersListPage (TEC-374 DataTable)", () => {
  it("maps column filters and sort to the list params", async () => {
    await render();
    expect(lastQuery()).toEqual({
      side: "seller",
      limit: 20,
      offset: 0,
      sort: "-created_at",
    });
    expect(lastTable().rowCount).toBe(45);

    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "status", value: ["submitted", "shipped"] },
        { id: "party", value: ["d1", "d2"] },
        { id: "total", value: [100, 500] },
        { id: "created_at", value: ["2026-10-01", "2026-10-02"] },
      ]);
    });
    await act(async () => {
      lastTable().state?.onSortingChange?.([{ id: "total", desc: true }]);
      lastTable().state?.onGlobalFilterChange?.("ORD-7");
    });
    await flush();
    expect(lastQuery()).toEqual({
      side: "seller",
      limit: 20,
      offset: 0,
      sort: "-total",
      q: "ORD-7",
      status: "submitted,shipped",
      buyer_org_uuid: "d1,d2",
      total_min: "100",
      total_max: "500",
      created_from: "2026-10-01",
      created_to: "2026-10-02",
    });
  });

  it("offers the buyers of the scope as a party filter on the incoming tab", async () => {
    await render();
    const party = column("party");
    expect(party?.enableColumnFilter).toBe(true);
    expect(party?.meta?.filterOptions).toEqual([
      { value: "d1", label: "Bayi 1" },
      { value: "d2", label: "Bayi 2" },
    ]);
    expect(column("order_no")?.enableSorting).toBe(true);
    expect(column("status")?.enableSorting).toBe(true);
    expect(party?.enableSorting).toBe(false);
  });

  it("switching to outgoing sends side=buyer and drops the party filter", async () => {
    await render();
    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "party", value: ["d1"] },
      ]);
    });
    await flush();
    expect(lastQuery()?.buyer_org_uuid).toBe("d1");
    await act(async () => {
      (
        container.querySelector("[data-testid=side-buyer]") as HTMLElement
      ).click();
    });
    await flush();
    expect(lastQuery()).toEqual({
      side: "buyer",
      limit: 20,
      offset: 0,
      sort: "-created_at",
    });
    expect(column("party")?.meta?.param).toBe("seller_org_uuid");
    expect(column("party")?.enableColumnFilter).toBe(false);
  });

  it("exports with the same tab, filters, search and sort", async () => {
    await render();
    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "status", value: ["approved"] },
      ]);
      lastTable().state?.onGlobalFilterChange?.("x");
    });
    await flush();
    const props = captured.exports.at(-1);
    expect(props?.exportPath).toBe("/v1/orders/export");
    expect(props?.formats).toEqual(["xlsx", "csv", "pdf"]);
    expect(props?.query).toEqual({
      status: "approved",
      side: "seller",
      q: "x",
      sort: "-created_at",
    });
  });

  it("row actions follow the side, status and server transitions", async () => {
    await render();
    expect(
      rowActions(order({ available_transitions: ["submitted", "cancelled"] })),
    ).toEqual(["view", "edit", "submit", "cancel"]);
    expect(
      rowActions(
        order({
          role: "seller",
          status: "submitted",
          available_transitions: ["approved", "cancelled"],
        }),
      ),
    ).toEqual(["view", "approve", "reject"]);
    // Ready needs the lines: left to the detail page.
    expect(
      rowActions(
        order({
          role: "seller",
          status: "preparing",
          available_transitions: ["ready", "cancelled"],
        }),
      ),
    ).toEqual(["view", "cancel"]);
    expect(
      rowActions(
        order({ role: "observer", available_transitions: ["submitted"] }),
      ),
    ).toEqual(["view"]);
  });

  it("a row status action confirms through the dialog", async () => {
    api.transition.mockResolvedValue(order({ status: "submitted" }));
    await render();
    const target = order({ available_transitions: ["submitted"] });
    const cell = column("actions")?.cell as (ctx: unknown) => ReactElement<{
      actions: { id: string; onSelect: () => void }[];
    }>;
    const submit = cell({ row: { original: target } }).props.actions.find(
      (a) => a.id === "submit",
    );
    await act(async () => submit?.onSelect());
    await flush();
    await act(async () => {
      (
        document.querySelector("[data-testid=action-confirm]") as HTMLElement
      ).click();
    });
    await flush();
    expect(api.transition).toHaveBeenCalledWith("o1", "submitted", undefined);
  });

  it("center: incoming only, no new order, no tabs", async () => {
    state.orgType = "center";
    await render();
    expect(container.querySelectorAll("[role=tab]")).toHaveLength(0);
    expect(lastQuery()?.side).toBe("seller");
    expect(container.querySelector("[data-testid=new-order]")).toBeNull();
  });

  it("dealer: outgoing only, no party filter", async () => {
    state.orgType = "dealer";
    await render();
    expect(lastQuery()?.side).toBe("buyer");
    expect(api.listOrganizations).not.toHaveBeenCalled();
  });

  it("is forbidden without orders.read", async () => {
    state.grants = new Set();
    await render();
    expect(api.list).not.toHaveBeenCalled();
    expect(container.textContent).toContain("orders.list.forbidden");
  });
});
