// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, Fragment, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import { Permission } from "@/config/permissions";

type AnyRow = Record<string, unknown> & { uuid: string };
type RowAction = { id: string; onSelect: () => void };

const captured = vi.hoisted(() => ({
  granted: [] as string[],
  dealer: {
    features: [] as string[],
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
  useRouter: () => ({ push: vi.fn() }),
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
  usePermission: () => ({
    can: (p: string) => captured.granted.includes(p),
  }),
}));
// A dealer under distributor "dist-1"; dealer_accounting comes from the
// test. The real resolveAccountingAccess decides the controls.
vi.mock("@/features/accounting/hooks/use-accounting-access", async (orig) => {
  const actual =
    await orig<
      typeof import("@/features/accounting/hooks/use-accounting-access")
    >();
  const { resolveAccountingAccess } =
    await import("@/features/accounting/lib/access");
  return {
    ...actual,
    useAccountingAccess: () => ({
      orgUuid: "dealer-1",
      orgType: "dealer",
      parentUuid: "dist-1",
      ...resolveAccountingAccess({
        can: (p) => captured.granted.includes(p),
        orgType: "dealer",
        features: captured.dealer.features,
        parentUuid: "dist-1",
      }),
    }),
  };
});
vi.mock("@/features/accounting/services/accounting.service", async (orig) => ({
  ...(await orig<object>()),
  accountingService: captured.service,
}));
vi.mock("@/features/accounting/components/settlement-dialog", () => ({
  SettlementDialog: () => null,
}));
vi.mock("@/features/accounting/components/entries-export", () => ({
  EntriesExportMenu: () => null,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  // Page actions and children (the create buttons live in `actions`).
  EntityPage: ({
    actions,
    children,
  }: {
    actions?: ReactNode;
    children: ReactNode;
  }) => createElement(Fragment, null, actions, children),
  EntityToolbar: () => null,
  EntityRowActions: ({ actions }: { actions: RowAction[] }) =>
    createElement(
      "div",
      { "data-testid": "row-actions" },
      actions.map((a) =>
        createElement("span", { key: a.id, "data-action": a.id }),
      ),
    ),
  // Renders every cell of every row, keyed by the row uuid.
  EntityTable: (props: Partial<DataTableProps<AnyRow>>) =>
    createElement(
      "div",
      null,
      (props.data ?? []).map((row) =>
        createElement(
          "div",
          { key: row.uuid, "data-row": row.uuid },
          (props.columns ?? []).map((col, i) =>
            createElement(
              Fragment,
              { key: i },
              typeof col.cell === "function"
                ? (col.cell as (ctx: unknown) => ReactNode)({
                    row: { original: row },
                    getValue: () => undefined,
                  })
                : null,
            ),
          ),
        ),
      ),
    ),
}));

import {
  chooseValue,
  installRadixPolyfills,
  optionLabels,
} from "@/test/form-controls";
import { CariPage } from "./cari-page";
import { EntriesPage } from "./entries-page";
import { ManualEntryForm } from "./manual-entry-dialog";
import type {
  AccountingCategory,
  CariAccount,
  FinanceAccount,
  FinanceEntry,
} from "@/features/accounting/services/accounting.service";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
installRadixPolyfills();

let container: HTMLDivElement;
let root: Root;

const page = (items: unknown[]) => ({
  items,
  total: items.length,
  limit: 20,
  offset: 0,
});

function entry(over: Partial<FinanceEntry>): FinanceEntry {
  return {
    uuid: "e-1",
    direction: "charge",
    category: "sale",
    category_label_key: "accounting.category.sale",
    account: null,
    cari_uuid: "cari-parent",
    counterparty_organization: { uuid: "dist-1", name: "Distributor" },
    orig_currency: "TRY",
    orig_amount: "100.00",
    currency: "TRY",
    amount: "100.00",
    rate: "1.00000000",
    rate_date: "2026-10-01",
    source_type: "order",
    source_uuid: "order-1",
    revision: 1,
    reversal_of_uuid: null,
    reversed_by_uuid: null,
    voided: false,
    description: null,
    created_at: "2026-10-01T10:00:00Z",
    ...over,
  };
}

const sourced = entry({ uuid: "e-sourced" });
const manual = entry({
  uuid: "e-manual",
  direction: "expense",
  category: "rent",
  cari_uuid: null,
  counterparty_organization: null,
  account: { uuid: "acc-1", name: "Kasa" },
  source_type: "manual",
  source_uuid: "manual-1",
});

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  captured.granted = [
    Permission.AccountingRead,
    Permission.AccountingWrite,
    Permission.AccountingDispute,
    Permission.CustomersRead,
  ];
  captured.dealer.features = [];
  captured.service.listCategories.mockResolvedValue({ items: [] });
  captured.service.listCari.mockResolvedValue(page([]));
  captured.service.listAccounts.mockResolvedValue({ items: [] });
  captured.service.listDisputes.mockResolvedValue(page([]));
  captured.service.listEntries.mockResolvedValue(page([sourced, manual]));
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
});

async function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

const rowActions = (uuid: string) =>
  Array.from(
    container.querySelectorAll(`[data-row="${uuid}"] [data-action]`),
  ).map((el) => el.getAttribute("data-action"));

describe("Dealer accounting write screens (TEC-347)", () => {
  it("module off: no 'New entry', read-only notice, no void action", async () => {
    await render(createElement(EntriesPage, { slug: "dealer" }));
    expect(
      container.querySelector('[data-testid="manual-entry-open"]'),
    ).toBeNull();
    expect(container.textContent).toContain("accounting.read_only_dealer");
    expect(rowActions("e-manual")).not.toContain("void");
  });

  it("module on: 'New entry' and the collection / payment buttons", async () => {
    captured.dealer.features = ["accounting", "dealer_accounting"];
    await render(createElement(EntriesPage, { slug: "dealer" }));
    expect(
      container.querySelector('[data-testid="manual-entry-open"]'),
    ).not.toBeNull();
    expect(container.textContent).toContain(
      "accounting.settlement.new_collection",
    );
    expect(container.textContent).not.toContain("accounting.read_only_dealer");
  });

  it("a sourced parent row has no edit (void) but a dispute; a manual row can be reversed", async () => {
    captured.dealer.features = ["accounting", "dealer_accounting"];
    await render(createElement(EntriesPage, { slug: "dealer" }));
    const sourcedRow = container.querySelector('[data-row="e-sourced"]');
    expect(rowActions("e-sourced")).not.toContain("void");
    expect(sourcedRow?.textContent).toContain(
      "accounting.disputes.open_action",
    );
    const manualRow = container.querySelector('[data-row="e-manual"]');
    expect(rowActions("e-manual")).toContain("void");
    expect(manualRow?.textContent).not.toContain(
      "accounting.disputes.open_action",
    );
  });

  it("customer cari is opened only with the module on", async () => {
    await render(createElement(CariPage, { slug: "dealer" }));
    expect(
      container.querySelector('[data-testid="customer-cari-open"]'),
    ).toBeNull();
    act(() => root.unmount());
    root = createRoot(container);
    captured.dealer.features = ["accounting", "dealer_accounting"];
    await render(createElement(CariPage, { slug: "dealer" }));
    expect(
      container.querySelector('[data-testid="customer-cari-open"]'),
    ).not.toBeNull();
  });
});

async function choose(el: Element | null, value: string) {
  await chooseValue(el, value);
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
}

describe("ManualEntryForm", () => {
  const accounts = [
    { uuid: "acc-1", name: "Kasa", currency: "TRY", active: true },
  ] as FinanceAccount[];
  const cari = [
    {
      uuid: "cari-customer",
      counterparty: { type: "user", uuid: "u-1", name: "Ayşe Yılmaz" },
    },
  ] as CariAccount[];
  const categories = [
    { key: "service_misc", direction: "income", label: "Misc", manual: true },
    { key: "sale", direction: "income", label: "Sale", manual: false },
    { key: "fee", direction: "charge", label: "Fee", manual: true },
  ] as AccountingCategory[];

  it("a charge hides the account and books the (customer) cari", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    await render(
      createElement(ManualEntryForm, {
        accounts,
        cari,
        categories,
        idempotencyKey: "key-1",
        onCancel: () => {},
        onSubmit,
      }),
    );
    // Income lists manual categories only and shows the account.
    const category = await optionLabels(
      container.querySelector("#manual-entry-category"),
    );
    expect(category.join(" ")).toContain("Misc");
    expect(category.join(" ")).not.toContain("Sale");
    expect(container.querySelector("#manual-entry-account")).not.toBeNull();

    await choose(container.querySelector("#manual-entry-direction"), "charge");
    expect(container.querySelector("#manual-entry-account")).toBeNull();
    await choose(container.querySelector("#manual-entry-category"), "fee");
    await type(container.querySelector("#manual-entry-amount"), "1.250,50");
    await act(async () => {
      (
        container.querySelector(
          '[data-testid="manual-entry-form"]',
        ) as HTMLFormElement
      ).requestSubmit();
    });
    // A charge without a cari is refused before the request.
    expect(onSubmit).not.toHaveBeenCalled();
    await choose(
      container.querySelector("#manual-entry-cari"),
      "cari-customer",
    );
    await act(async () => {
      (
        container.querySelector(
          '[data-testid="manual-entry-form"]',
        ) as HTMLFormElement
      ).requestSubmit();
    });
    expect(onSubmit).toHaveBeenCalledWith({
      direction: "charge",
      category: "fee",
      amount: "1250.50",
      cari_uuid: "cari-customer",
      idempotency_key: "key-1",
    });
  });
});
