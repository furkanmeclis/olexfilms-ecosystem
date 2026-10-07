// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, Fragment, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";

type AnyRow = Record<string, unknown> & { uuid: string };
type RowAction = { id: string; onSelect: () => void };

const captured = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<AnyRow>>[],
  rowActions: [] as RowAction[][],
  exports: [] as Record<string, unknown>[],
  settlements: [] as Record<string, unknown>[],
  push: vi.fn(),
  access: {
    orgUuid: "dist-1",
    orgType: "distributor",
    parentUuid: "center-1",
    canRead: true,
    canWrite: true,
    canDispute: false,
    canResolve: true,
    readOnlyDealer: false,
  },
  service: {
    listCategories: vi.fn(),
    listCari: vi.fn(),
    listAccounts: vi.fn(),
    listEntries: vi.fn(),
    listDisputes: vi.fn(),
  },
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: captured.push }),
}));
vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: ReactNode }) =>
    createElement("a", { href }, children),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: {
      number: (v: number) => String(v),
      date: (v: string) => v,
      dateTime: (v: string) => v,
      currency: (v: number, c: string) => `${v} ${c}`,
    },
  }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/features/accounting/hooks/use-accounting-access", async (orig) => ({
  ...(await orig<object>()),
  useAccountingAccess: () => captured.access,
}));
vi.mock("@/features/accounting/services/accounting.service", async (orig) => ({
  ...(await orig<object>()),
  accountingService: captured.service,
}));
vi.mock("@/features/accounting/components/settlement-dialog", () => ({
  SettlementDialog: (props: Record<string, unknown>) => {
    captured.settlements.push(props);
    return null;
  },
}));
vi.mock("@/features/accounting/components/entries-export", () => ({
  EntriesExportMenu: (props: Record<string, unknown>) => {
    captured.exports.push(props);
    return null;
  },
}));
vi.mock("@/features/accounting/components/account-form-dialog", () => ({
  AccountFormDialog: () => null,
}));
vi.mock("@/features/accounting/components/account-opening-dialog", () => ({
  AccountOpeningDialog: ({ account }: { account: { uuid: string } | null }) =>
    createElement("div", {
      "data-dialog": "opening",
      "data-uuid": account?.uuid ?? "",
    }),
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: ReactNode }) =>
    createElement(Fragment, null, children),
  EntityCreateButton: () => null,
  EntityToolbar: () => null,
  EntityRowActions: ({ actions }: { actions: RowAction[] }) => {
    captured.rowActions.push(actions);
    return null;
  },
  // Renders the toolbar and the actions cell of every row.
  EntityTable: (props: Partial<DataTableProps<AnyRow>>) => {
    captured.tables.push(props);
    const actions = (props.columns ?? []).find((c) => c.id === "actions");
    return createElement(
      "div",
      null,
      (props as { toolbarExtra?: ReactNode }).toolbarExtra,
      (props.data ?? []).map((row) =>
        createElement(
          Fragment,
          { key: row.uuid },
          typeof actions?.cell === "function"
            ? (actions.cell as (ctx: unknown) => ReactNode)({
                row: { original: row },
              })
            : null,
        ),
      ),
    );
  },
}));

import { AccountsPage, ACCOUNTS_PERSIST_KEY } from "./accounts-page";
import { CariPage, CARI_PERSIST_KEY } from "./cari-page";
import { DisputesPage, DISPUTES_PERSIST_KEY } from "./disputes-page";
import { EntriesPage, ENTRIES_PERSIST_KEY } from "./entries-page";
import { StatementView } from "./statement-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

const page = (items: unknown[], total = items.length) => ({
  items,
  total,
  limit: 20,
  offset: 0,
});

const cari = (over: Record<string, unknown> = {}) => ({
  uuid: "c-1",
  counterparty: {
    type: "organization",
    uuid: "dealer-1",
    name: "Kuzey Oto",
    org_type: "dealer",
  },
  currency: "TRY",
  active: true,
  balance: "150.00",
  entry_count: 4,
  last_entry_at: "2026-10-02T10:00:00Z",
  created_at: "2026-09-01T10:00:00Z",
  ...over,
});

const account = (over: Record<string, unknown> = {}) => ({
  uuid: "a-1",
  type: "cash",
  name: "Kasa",
  currency: "TRY",
  iban: null,
  active: true,
  balance: "500.00",
  entry_count: 2,
  last_entry_at: null,
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
  ...over,
});

const dispute = (over: Record<string, unknown> = {}) => ({
  uuid: "d-1",
  status: "open",
  organization: { uuid: "dealer-1", name: "Kuzey Oto" },
  counterparty_organization: { uuid: "dist-1", name: "Ege Dağıtım" },
  entry: {
    uuid: "e-1",
    direction: "charge",
    category: "sales",
    category_label_key: "accounting.category.sales",
    orig_currency: "TRY",
    orig_amount: "100.00",
    currency: "TRY",
    amount: "100.00",
    revision: 0,
    created_at: "2026-10-01T10:00:00Z",
  },
  source_type: "order",
  source_uuid: "o-1",
  reason: "Wrong amount",
  corrected_amount: null,
  resolution_note: null,
  reversal_entry_uuid: null,
  revision_entry_uuid: null,
  created_at: "2026-10-02T10:00:00Z",
  resolved_at: null,
  ...over,
});

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  captured.tables = [];
  captured.rowActions = [];
  captured.exports = [];
  captured.settlements = [];
  captured.access = { ...captured.access, canWrite: true, canResolve: true };
  const s = captured.service;
  s.listCategories.mockResolvedValue({
    items: [{ key: "sales", label: "Satış" }],
  });
  s.listCari.mockResolvedValue(
    page([cari(), cari({ uuid: "c-2", active: false })], 30),
  );
  s.listAccounts.mockResolvedValue({
    items: [account(), account({ uuid: "a-2", type: "bank", active: false })],
  });
  s.listEntries.mockResolvedValue(page([], 0));
  s.listDisputes.mockResolvedValue(
    page([dispute(), dispute({ uuid: "d-2", status: "rejected" })], 2),
  );
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

async function render(node: ReturnType<typeof createElement>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await flush();
}

const table = () => captured.tables.at(-1)!;
const colId = (c: ColumnDef<AnyRow, unknown>) =>
  c.id ?? (c as { accessorKey?: string }).accessorKey;
const sortable = () =>
  (table().columns ?? []).filter((c) => c.enableSorting).map(colId);
const lastCall = (fn: { mock: { calls: unknown[][] } }) =>
  fn.mock.calls.at(-1)![0] as Record<string, unknown>;

async function setFilters(value: { id: string; value: unknown }[]) {
  await act(async () => {
    table().state!.onColumnFiltersChange!(value);
  });
  await flush();
}

async function sortBy(id: string, desc = false) {
  await act(async () => {
    table().state!.onSortingChange!([{ id, desc }]);
  });
  await flush();
}

describe("CariPage (TEC-380)", () => {
  it("lists active cari by name and maps sort, search and filters", async () => {
    await render(createElement(CariPage, { slug: "acme" }));
    const list = captured.service.listCari;
    expect(lastCall(list)).toEqual({
      active: "true",
      sort: "name",
      limit: 20,
      offset: 0,
    });
    expect(table().rowCount).toBe(30);
    expect(table().features?.persistKey).toBe(CARI_PERSIST_KEY);
    expect(sortable()).toEqual([
      "name",
      "balance",
      "entry_count",
      "last_entry_at",
      "created_at",
    ]);

    await sortBy("balance", true);
    expect(lastCall(list).sort).toBe("-balance");

    await act(async () => {
      table().state!.onGlobalFilterChange!("kuzey");
    });
    await setFilters([
      { id: "kind", value: ["dealer", "customer"] },
      { id: "balance", value: ["-100", "2500"] },
      { id: "active", value: false },
    ]);
    expect(lastCall(list)).toEqual({
      q: "kuzey",
      counterparty_kind: "dealer,customer",
      balance_min: "-100",
      balance_max: "2500",
      active: "false",
      sort: "-balance",
      limit: 20,
      offset: 0,
    });
  });

  it("opens the statement and a fixed-cari settlement from the row menu", async () => {
    await render(createElement(CariPage, { slug: "acme" }));
    const [active, inactive] = captured.rowActions.slice(-2);
    expect(active.map((a) => a.id)).toEqual([
      "open",
      "statement",
      "collection",
      "payment",
    ]);
    expect(inactive.map((a) => a.id)).toEqual(["open", "statement"]);

    active.find((a) => a.id === "statement")!.onSelect();
    expect(captured.push).toHaveBeenCalledWith(
      "/t/acme/accounting/cari/c-1/statement",
    );
    await act(async () => {
      active.find((a) => a.id === "payment")!.onSelect();
    });
    expect(captured.settlements.at(-1)).toMatchObject({
      kind: "payment",
      fixedCari: { uuid: "c-1" },
    });
  });
});

describe("EntriesPage (TEC-380)", () => {
  it("maps sort, search and every column filter; the export follows", async () => {
    await render(
      createElement(EntriesPage, { slug: "acme", initialCari: "c-1" }),
    );
    const list = captured.service.listEntries;
    expect(lastCall(list)).toEqual({
      cari_uuid: "c-1",
      sort: "-created_at",
      limit: 20,
      offset: 0,
    });
    expect(table().features?.persistKey).toBe(ENTRIES_PERSIST_KEY);
    expect(sortable()).toEqual([
      "created_at",
      "direction",
      "category",
      "amount",
    ]);

    await sortBy("amount", true);
    await act(async () => {
      table().state!.onGlobalFilterChange!("kira");
    });
    await setFilters([
      { id: "created_at", value: ["2026-10-01", "2026-10-31"] },
      { id: "direction", value: ["income", "expense"] },
      { id: "category", value: ["sales"] },
      { id: "counterparty", value: "c-2" },
      { id: "account", value: "a-1" },
      { id: "source_type", value: ["order", "manual"] },
      { id: "amount", value: ["-50", "1000"] },
    ]);
    const params = {
      created_from: "2026-10-01",
      created_to: "2026-10-31",
      direction: "income,expense",
      category: "sales",
      cari_uuid: "c-2",
      account_uuid: "a-1",
      source_type: "order,manual",
      amount_min: "-50",
      amount_max: "1000",
    };
    expect(lastCall(list)).toEqual({
      ...params,
      q: "kira",
      sort: "-amount",
      limit: 20,
      offset: 0,
    });
    expect(captured.exports.at(-1)).toEqual({
      orgUuid: "dist-1",
      query: { ...params, q: "kira", sort: "-amount" },
    });
  });

  it("fills the cari, account and category filters from the book", async () => {
    await render(createElement(EntriesPage, { slug: "acme" }));
    const meta = (id: string) =>
      (table().columns ?? []).find((c) => colId(c) === id)!.meta!;
    expect(meta("counterparty").filterOptions).toEqual([
      { value: "c-1", label: "Kuzey Oto" },
      { value: "c-2", label: "Kuzey Oto" },
    ]);
    expect(meta("account").filterOptions).toEqual([
      { value: "a-1", label: "Kasa" },
      { value: "a-2", label: "Kasa" },
    ]);
    expect(meta("category").filterOptions).toEqual([
      { value: "sales", label: "Satış" },
    ]);
  });
});

describe("DisputesPage (TEC-380)", () => {
  it("starts on open disputes and maps sort, search and filters", async () => {
    await render(createElement(DisputesPage, { slug: "acme" }));
    const list = captured.service.listDisputes;
    expect(lastCall(list)).toEqual({
      status: "open",
      sort: "-created_at",
      limit: 20,
      offset: 0,
    });
    expect(table().features?.persistKey).toBe(DISPUTES_PERSIST_KEY);
    expect(sortable()).toEqual([
      "created_at",
      "organization",
      "amount",
      "status",
      "resolved_at",
    ]);

    await sortBy("organization");
    await act(async () => {
      table().state!.onGlobalFilterChange!("iade");
    });
    await setFilters([
      { id: "status", value: ["open", "rejected"] },
      { id: "organization", value: ["dealer-1"] },
      { id: "counterparty", value: ["dist-1"] },
      { id: "created_at", value: [undefined, "2026-10-31"] },
    ]);
    expect(lastCall(list)).toEqual({
      q: "iade",
      status: "open,rejected",
      organization_uuid: "dealer-1",
      counterparty_organization_uuid: "dist-1",
      created_to: "2026-10-31",
      sort: "organization",
      limit: 20,
      offset: 0,
    });
  });

  it("offers resolve on open disputes addressed to the organization", async () => {
    await render(createElement(DisputesPage, { slug: "acme" }));
    const [open, rejected] = captured.rowActions.slice(-2);
    expect(open.map((a) => a.id)).toEqual(["open", "resolve"]);
    expect(rejected.map((a) => a.id)).toEqual(["open"]);
    open.find((a) => a.id === "resolve")!.onSelect();
    expect(captured.push).toHaveBeenCalledWith(
      "/t/acme/accounting/disputes/d-1",
    );
  });
});

describe("AccountsPage (TEC-380)", () => {
  it("is a client-side grid with type / active filters and row actions", async () => {
    await render(createElement(AccountsPage, { slug: "acme" }));
    expect(table().manual).toEqual({
      sorting: false,
      filtering: false,
      pagination: false,
    });
    expect(table().initialState?.viewMode).toBe("grid");
    expect(table().features?.persistKey).toBe(ACCOUNTS_PERSIST_KEY);
    expect(table().data).toHaveLength(2);
    expect(sortable()).toEqual([
      "name",
      "type",
      "balance",
      "entry_count",
      "last_entry_at",
      "created_at",
    ]);
    const active = (table().columns ?? []).find((c) => colId(c) === "active")!;
    const filter = active.filterFn as (
      row: unknown,
      id: string,
      value: unknown,
    ) => boolean;
    const row = { getValue: () => false };
    expect(filter(row, "active", undefined)).toBe(true);
    expect(filter(row, "active", true)).toBe(false);
    expect(filter(row, "active", false)).toBe(true);

    const [cash, bank] = captured.rowActions.slice(-2);
    expect(cash.map((a) => a.id)).toEqual(["edit", "opening"]);
    expect(bank.map((a) => a.id)).toEqual(["edit"]);
    await act(async () => {
      cash.find((a) => a.id === "opening")!.onSelect();
    });
    expect(
      container
        .querySelector("[data-dialog=opening]")
        ?.getAttribute("data-uuid"),
    ).toBe("a-1");
  });

  it("has no row actions without accounting.write", async () => {
    captured.access = { ...captured.access, canWrite: false };
    await render(createElement(AccountsPage, { slug: "acme" }));
    expect(captured.rowActions).toHaveLength(0);
  });
});

describe("StatementView (TEC-380)", () => {
  it("keeps the chronological order: client-side, no sorting", async () => {
    await render(
      createElement(StatementView, {
        statement: {
          cari: cari(),
          currency: "TRY",
          opening_balance: "0",
          total_debit: "100",
          total_credit: "0",
          closing_balance: "100",
          lines: [
            {
              uuid: "l-1",
              date: "2026-10-01",
              description: "Sipariş",
              category_label: "Satış",
              source_label: "Order",
              debit: "100",
              credit: "0",
              balance: "100",
              reversal_of_uuid: null,
              reversed: false,
            },
          ],
        } as never,
        orgUuid: "dist-1",
        parentUuid: "center-1",
        canDispute: false,
        openDisputes: new Set<string>(),
      }),
    );
    expect(table().manual).toEqual({
      sorting: false,
      filtering: false,
      pagination: false,
    });
    expect(table().features).toMatchObject({ sorting: false });
    expect(sortable()).toEqual([]);
  });
});
