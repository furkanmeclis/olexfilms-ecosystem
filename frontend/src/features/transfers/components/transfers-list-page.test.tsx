// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { StockTransfer } from "@/features/transfers/services/transfers.service";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  listOrganizations: vi.fn(),
  targets: vi.fn(),
  transition: vi.fn(),
}));
const state = vi.hoisted(() => ({ grants: new Set<string>() }));
const captured = vi.hoisted(() => ({
  tables: [] as Record<string, unknown>[],
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
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    format: {
      number: (v: number) => String(v),
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/features/transfers/services/transfers.service", async (orig) => ({
  ...(await orig<object>()),
  transfersService: api,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (props: Record<string, unknown>) => {
    captured.tables.push(props);
    return createElement("div", null, props.toolbarExtra as ReactNode);
  },
}));

import { Permission } from "@/config/permissions";

import { TransfersListPage } from "./transfers-list-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

type TableProps = {
  columns: ColumnDef<StockTransfer, unknown>[];
  rowCount?: number;
  state?: {
    onColumnFiltersChange?: (v: { id: string; value: unknown }[]) => void;
    onSortingChange?: (v: { id: string; desc: boolean }[]) => void;
    onGlobalFilterChange?: (v: string) => void;
  };
};
type Action = { id: string; onSelect: () => void };

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  api.list.mockResolvedValue({ items: [], total: 7, limit: 20, offset: 0 });
  api.listOrganizations.mockResolvedValue([
    { uuid: "a", name: "Bayi A" },
    { uuid: "b", name: "Bayi B" },
  ]);
  api.targets.mockResolvedValue({
    items: [
      { uuid: "b", name: "Bayi B", type: "dealer" },
      { uuid: "c", name: "Bayi C", type: "dealer" },
    ],
  });
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
        createElement(TransfersListPage, { slug: "s" }),
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

function transfer(patch: Partial<StockTransfer> = {}): StockTransfer {
  const org = (name: string) => ({ uuid: name, name, type: "dealer" });
  return {
    uuid: "t-1",
    transfer_no: "TRF-1",
    kind: "sibling",
    status: "requested",
    role: "parent",
    sender: org("Bayi A"),
    receiver: org("Bayi B"),
    parent: org("Dist"),
    currency: "TRY",
    total: null,
    note: null,
    decision_note: null,
    cancel_reason: null,
    item_count: 1,
    decided_at: null,
    shipped_at: null,
    received_at: null,
    cancelled_at: null,
    created_at: "2026-10-02T10:00:00Z",
    updated_at: "2026-10-02T10:00:00Z",
    available_transitions: ["approved", "rejected"],
    ...patch,
  } as StockTransfer;
}

function rowActions(t: StockTransfer): Action[] {
  const cell = column("actions")?.cell as (
    ctx: unknown,
  ) => ReactElement<{ actions: Action[] }>;
  return cell({ row: { original: t } }).props.actions;
}

describe("TransfersListPage (TEC-197, TEC-374 DataTable)", () => {
  it("maps column filters, search and sort to the list params", async () => {
    state.grants = new Set([
      Permission.TransfersApprove,
      Permission.OrganizationsRead,
    ]);
    await render();
    expect(lastQuery()).toEqual({ limit: 20, offset: 0, sort: "-created_at" });
    expect(lastTable().rowCount).toBe(7);
    expect(container.querySelector('[data-testid="transfer-new"]')).toBeNull();

    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "kind", value: ["return"] },
        { id: "status", value: ["requested", "approved"] },
        { id: "organization", value: ["a", "b"] },
        { id: "created_at", value: ["2026-10-01", "2026-10-03"] },
      ]);
    });
    await act(async () => {
      lastTable().state?.onSortingChange?.([
        { id: "transfer_no", desc: false },
      ]);
      lastTable().state?.onGlobalFilterChange?.("TRF");
    });
    await flush();
    expect(lastQuery()).toEqual({
      limit: 20,
      offset: 0,
      sort: "transfer_no",
      q: "TRF",
      kind: "return",
      status: "requested,approved",
      organization_uuid: "a,b",
      created_from: "2026-10-01",
      created_to: "2026-10-03",
    });
    expect(column("sender")?.enableSorting).toBe(false);
  });

  it("keeps the direction as tabs next to the filters", async () => {
    state.grants = new Set([Permission.TransfersApprove]);
    await render();
    await act(async () => {
      (
        container.querySelector('[data-direction="approval"]') as HTMLElement
      ).click();
    });
    await flush();
    expect(lastQuery()).toEqual(
      expect.objectContaining({ direction: "approval", offset: 0 }),
    );
    await act(async () => {
      (
        container.querySelector('[data-direction="all"]') as HTMLElement
      ).click();
    });
    await flush();
    expect(lastQuery()?.direction).toBeUndefined();
  });

  it("offers the scope's organizations and the siblings as filter options", async () => {
    state.grants = new Set([
      Permission.TransfersRequest,
      Permission.OrganizationsRead,
    ]);
    await render();
    expect(
      container.querySelector('[data-testid="transfer-new-return"]'),
    ).not.toBeNull();
    const org = column("organization");
    expect(org?.enableColumnFilter).toBe(true);
    expect(org?.meta?.filterOptions).toEqual([
      { value: "a", label: "Bayi A" },
      { value: "b", label: "Bayi B" },
      { value: "c", label: "Bayi C" },
    ]);
  });

  it("row actions are the server's moves; reject asks for a reason", async () => {
    state.grants = new Set([Permission.TransfersApprove]);
    api.transition.mockResolvedValue(transfer({ status: "approved" }));
    await render();
    const actions = rowActions(transfer());
    expect(actions.map((a) => a.id)).toEqual(["view", "approved", "rejected"]);

    await act(async () => actions[1].onSelect());
    await flush();
    expect(api.transition).toHaveBeenLastCalledWith(
      "t-1",
      "approved",
      undefined,
    );

    await act(async () => rowActions(transfer())[2].onSelect());
    await flush();
    await act(async () => {
      (
        document.querySelector(
          '[data-testid="transfer-reason-confirm"]',
        ) as HTMLElement
      ).click();
    });
    await flush();
    expect(api.transition).toHaveBeenLastCalledWith(
      "t-1",
      "rejected",
      undefined,
    );
    expect(
      rowActions(transfer({ available_transitions: [] })).map((a) => a.id),
    ).toEqual(["view"]);
  });

  it("is forbidden without a transfer permission", async () => {
    await render();
    expect(api.list).not.toHaveBeenCalled();
    expect(container.textContent).toContain("transfers.list.forbidden");
  });
});
