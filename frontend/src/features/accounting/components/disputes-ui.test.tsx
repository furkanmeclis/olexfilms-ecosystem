// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "tr",
    format: {
      number: (v: number | null) => (v === null ? "—" : String(v)),
      date: (v: string) => `d:${v}`,
      dateTime: (v: string) => `dt:${v}`,
      currency: (v: number | null, c: string) =>
        v === null ? "—" : `${v.toFixed(2)} ${c}`,
    },
  }),
}));

vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("next/link", async () => {
  const { createElement: h } = await import("react");
  return {
    default: ({ href, children, ...rest }: Record<string, unknown>) =>
      h("a", { href, ...rest }, children as ReactNode),
  };
});

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

// The page shell gates on the permission provider; the tests drive the
// access flags directly through the mocked hook below.
vi.mock("@/hooks/use-mobile", () => ({
  useIsMobile: () => false,
  useIsXl: () => true,
}));

vi.mock("@/components/entity", async (importOriginal) => {
  const { createElement: h } = await import("react");
  return {
    ...(await importOriginal<object>()),
    EntityPage: ({
      actions,
      children,
    }: {
      actions?: ReactNode;
      children: ReactNode;
    }) =>
      h(
        "div",
        null,
        h("div", { "data-testid": "page-actions" }, actions),
        children,
      ),
  };
});

const accessState = vi.hoisted(() => ({
  value: {
    orgUuid: "dealer-1",
    orgType: "dealer",
    parentUuid: "dist-1",
    canRead: true,
    canWrite: false,
    canDispute: true,
    canResolve: false,
    readOnlyDealer: true,
  },
}));

vi.mock("@/features/accounting/hooks/use-accounting-access", async () => {
  const actual = await vi.importActual<
    typeof import("@/features/accounting/hooks/use-accounting-access")
  >("@/features/accounting/hooks/use-accounting-access");
  return { ...actual, useAccountingAccess: () => accessState.value };
});

const service = vi.hoisted(() => ({
  listCategories: vi.fn(),
  getCari: vi.fn(),
  listEntries: vi.fn(),
  listDisputes: vi.fn(),
  openDispute: vi.fn(),
  exportStatement: vi.fn(),
  getExport: vi.fn(),
  downloadExport: vi.fn(),
}));

vi.mock(
  "@/features/accounting/services/accounting.service",
  async (importOriginal) => {
    const actual =
      await importOriginal<
        typeof import("@/features/accounting/services/accounting.service")
      >();
    return { ...actual, accountingService: service };
  },
);

import { installRadixPolyfills } from "@/test/form-controls";
import { CariDetailPage } from "./cari-detail-page";
import { DisputeForm } from "./dispute-dialog";
import { ResolveForm } from "./dispute-detail-page";
import { EntryStatusCell } from "./entries-page";
import { StatementExport } from "./statement-export";
import { StatementView } from "./statement-page";
import type {
  AccountingDispute,
  AccountingExportJob,
  CariAccount,
  CariStatement,
  CariStatementLine,
  FinanceEntry,
} from "@/features/accounting/services/accounting.service";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
installRadixPolyfills();

let container: HTMLDivElement;
let root: Root;
let client: QueryClient;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  for (const fn of Object.values(service)) fn.mockReset();
  service.listCategories.mockResolvedValue({ items: [] });
  service.listDisputes.mockResolvedValue({
    items: [],
    total: 0,
    limit: 100,
    offset: 0,
  });
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  client.clear();
});

async function render(node: ReactNode) {
  await act(async () => {
    root.render(
      createElement(QueryClientProvider, { client }, node as ReactNode),
    );
  });
}

async function flush(ms = 0) {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, ms));
  });
}

async function click(el: Element | null) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
}

async function type(el: Element | null, value: string) {
  const proto =
    el instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  if (!(el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement)) {
    throw new Error("input not found");
  }
  const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
  await act(async () => {
    setter?.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function submit(form: Element | null) {
  if (!(form instanceof HTMLFormElement)) throw new Error("form not found");
  await act(async () => {
    form.requestSubmit();
  });
  await flush();
}

const $ = (sel: string) => container.querySelector(sel);
const $$ = (sel: string) => Array.from(container.querySelectorAll(sel));

const PARENT = "dist-1";

const cari = (over: Partial<CariAccount> = {}): CariAccount => ({
  uuid: "c1",
  counterparty: {
    type: "organization",
    uuid: PARENT,
    name: "Distributor",
    org_type: "distributor",
  },
  currency: "TRY",
  active: true,
  balance: "-150.00",
  entry_count: 3,
  last_entry_at: "2026-10-01T10:00:00Z",
  created_at: "2026-09-01T00:00:00Z",
  ...over,
});

const line = (over: Partial<CariStatementLine> = {}): CariStatementLine => ({
  uuid: "l1",
  date: "2026-10-01T10:00:00Z",
  direction: "charge",
  category: "order",
  category_label: "Order",
  description: "Order",
  source_type: "order",
  source_uuid: "o1",
  source_label: "Order",
  reversal_of_uuid: null,
  reversed: false,
  orig_currency: "TRY",
  orig_amount: "100.00",
  debit: "0.00",
  credit: "100.00",
  balance: "-100.00",
  ...over,
});

const statement = (lines: CariStatementLine[]): CariStatement => ({
  organization: { uuid: "dealer-1", name: "Dealer" },
  cari: cari(),
  currency: "TRY",
  from: null,
  to: null,
  opening_balance: "0.00",
  total_debit: "0.00",
  total_credit: "150.00",
  closing_balance: "-150.00",
  lines,
  generated_at: "2026-10-02T00:00:00Z",
});

const entry = (over: Partial<FinanceEntry> = {}): FinanceEntry => ({
  uuid: "e1",
  direction: "charge",
  category: "order",
  category_label_key: "accounting.category.order",
  account: null,
  cari_uuid: "c1",
  counterparty_organization: { uuid: PARENT, name: "Distributor" },
  orig_currency: "TRY",
  orig_amount: "100.00",
  currency: "TRY",
  amount: "100.00",
  rate: "1.00000000",
  rate_date: "2026-10-01",
  source_type: "order",
  source_uuid: "o1",
  revision: 1,
  reversal_of_uuid: null,
  reversed_by_uuid: null,
  voided: false,
  description: null,
  created_at: "2026-10-01T10:00:00Z",
  ...over,
});

const dispute = (over: Partial<AccountingDispute> = {}): AccountingDispute => ({
  uuid: "d1",
  status: "open",
  organization: { uuid: "dealer-1", name: "Dealer" },
  counterparty_organization: { uuid: PARENT, name: "Distributor" },
  entry: {
    uuid: "e1",
    direction: "charge",
    category: "order",
    category_label_key: "accounting.category.order",
    orig_currency: "EUR",
    orig_amount: "100.00",
    currency: "TRY",
    amount: "3500.00",
    revision: 1,
    created_at: "2026-10-01T10:00:00Z",
  },
  source_type: "order",
  source_uuid: "o1",
  reason: "Wrong amount",
  corrected_amount: null,
  resolution_note: null,
  reversal_entry_uuid: null,
  revision_entry_uuid: null,
  created_at: "2026-10-02T10:00:00Z",
  resolved_at: null,
  ...over,
});

describe("dispute button visibility", () => {
  const lines = [
    line({ uuid: "ok" }),
    line({ uuid: "manual", source_type: "manual", source_uuid: null }),
    line({ uuid: "reversal", reversal_of_uuid: "ok-0" }),
    line({ uuid: "reversed", reversed: true }),
    line({ uuid: "disputed" }),
  ];

  it("shows on disputable statement rows only", async () => {
    await render(
      createElement(StatementView, {
        statement: statement(lines),
        orgUuid: "dealer-1",
        parentUuid: PARENT,
        canDispute: true,
        openDisputes: new Set(["disputed"]),
      }),
    );
    const withButton = $$("[data-testid=dispute-open]").map((b) =>
      b.getAttribute("data-entry"),
    );
    expect(withButton).toEqual(["ok"]);
    expect(
      $("[data-entry=disputed] [data-testid=statement-line-disputed]"),
    ).not.toBeNull();
    expect($("[data-testid=statement-closing]")?.textContent).toContain(
      "-150.00 TRY",
    );
  });

  it("is hidden without accounting.dispute or on a non-parent cari", async () => {
    await render(
      createElement(StatementView, {
        statement: statement(lines),
        orgUuid: "dealer-1",
        parentUuid: PARENT,
        canDispute: false,
        openDisputes: new Set<string>(),
      }),
    );
    expect($$("[data-testid=dispute-open]")).toHaveLength(0);

    await render(
      createElement(StatementView, {
        statement: statement(lines),
        orgUuid: "dealer-1",
        parentUuid: "someone-else",
        canDispute: true,
        openDisputes: new Set<string>(),
      }),
    );
    expect($$("[data-testid=dispute-open]")).toHaveLength(0);
  });

  it("follows the same rule on ledger rows", async () => {
    const opts = {
      orgUuid: "dealer-1",
      parentUuid: PARENT,
      openDisputes: new Set<string>(["e-disputed"]),
    };
    await render(
      createElement(
        "div",
        null,
        createElement(EntryStatusCell, { entry: entry(), dispute: opts }),
        createElement(EntryStatusCell, {
          entry: entry({ uuid: "e-void", voided: true, reversed_by_uuid: "x" }),
          dispute: opts,
        }),
        createElement(EntryStatusCell, {
          entry: entry({ uuid: "e-manual", source_type: "manual" }),
          dispute: opts,
        }),
        createElement(EntryStatusCell, {
          entry: entry({ uuid: "e-disputed" }),
          dispute: opts,
        }),
        createElement(EntryStatusCell, {
          entry: entry({ uuid: "e-nodispute" }),
          dispute: null,
        }),
      ),
    );
    expect(
      $$("[data-testid=dispute-open]").map((b) => b.getAttribute("data-entry")),
    ).toEqual(["e1"]);
    expect($$("[data-testid=entry-disputed]")).toHaveLength(1);
  });

  it("opens the reason form and posts the dispute", async () => {
    service.openDispute.mockResolvedValue(dispute());
    await render(
      createElement(StatementView, {
        statement: statement([line({ uuid: "ok" })]),
        orgUuid: "dealer-1",
        parentUuid: PARENT,
        canDispute: true,
        openDisputes: new Set<string>(),
      }),
    );
    await click($("[data-testid=dispute-open]"));
    const form = document.querySelector("[data-testid=dispute-form]");
    expect(form).not.toBeNull();
    await type(document.querySelector("#dispute-reason"), "  Returned  ");
    await submit(form);
    expect(service.openDispute).toHaveBeenCalledWith({
      entry_uuid: "ok",
      reason: "Returned",
    });
  });
});

describe("DisputeForm", () => {
  it("requires a reason", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    await render(
      createElement(DisputeForm, {
        entryUuid: "e1",
        onCancel: () => {},
        onSubmit,
      }),
    );
    await submit($("[data-testid=dispute-form]"));
    expect(onSubmit).not.toHaveBeenCalled();
    expect($("#dispute-reason-error")?.textContent).toBe(
      "accounting.validation.required",
    );
  });
});

describe("ResolveForm", () => {
  async function renderForm() {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    await render(createElement(ResolveForm, { dispute: dispute(), onSubmit }));
    return onSubmit;
  }

  it("posts a reversal", async () => {
    const onSubmit = await renderForm();
    expect($("#resolve-amount")).toBeNull();
    await submit($("[data-testid=resolve-form]"));
    expect(onSubmit).toHaveBeenCalledWith({ resolution: "reversal" });
  });

  it("asks a corrected amount in the entry currency for a revision", async () => {
    const onSubmit = await renderForm();
    await click($("[data-testid=resolve-revision]"));
    expect($("label[for=resolve-amount]")?.textContent).toContain("EUR");
    await submit($("[data-testid=resolve-form]"));
    expect(onSubmit).not.toHaveBeenCalled();
    expect($("#resolve-amount-error")?.textContent).toBe(
      "accounting.validation.required",
    );
    await type($("#resolve-amount"), "80,5");
    await submit($("[data-testid=resolve-form]"));
    expect(onSubmit).toHaveBeenCalledWith({
      resolution: "revision",
      corrected_amount: "80.50",
    });
  });

  it("needs a reason to reject", async () => {
    const onSubmit = await renderForm();
    await click($("[data-testid=resolve-reject]"));
    await submit($("[data-testid=resolve-form]"));
    expect(onSubmit).not.toHaveBeenCalled();
    expect($("#resolve-note-error")?.textContent).toBe(
      "accounting.disputes.validation.note_required",
    );
    await type($("#resolve-note"), "Invoice is correct");
    await submit($("[data-testid=resolve-form]"));
    expect(onSubmit).toHaveBeenCalledWith({
      resolution: "reject",
      note: "Invoice is correct",
    });
  });
});

describe("StatementExport", () => {
  const job = (
    over: Partial<AccountingExportJob> = {},
  ): AccountingExportJob => ({
    uuid: "j1",
    resource: "accounting_statement",
    format: "pdf",
    status: "queued",
    row_count: 0,
    error: null,
    download_url: null,
    created_at: "2026-10-02T10:00:00Z",
    ...over,
  });

  it("queues the job, polls it and downloads the file once", async () => {
    service.exportStatement.mockResolvedValue(job());
    service.getExport
      .mockResolvedValueOnce(job({ status: "processing" }))
      .mockResolvedValue(
        job({ status: "completed", download_url: "/v1/x/download" }),
      );
    service.downloadExport.mockResolvedValue(undefined);
    await render(
      createElement(StatementExport, {
        orgUuid: "dealer-1",
        cariUuid: "c1",
        period: { from: "2026-09-01", to: "2026-09-30" },
        pollMs: 5,
      }),
    );
    expect(
      $$("[data-testid^=statement-export-]").length,
    ).toBeGreaterThanOrEqual(3);
    await click($("[data-testid=statement-export-pdf]"));
    expect(service.exportStatement).toHaveBeenCalledWith(
      "c1",
      "pdf",
      { from: "2026-09-01", to: "2026-09-30" },
      "tr",
    );
    expect(
      ($("[data-testid=statement-export-xlsx]") as HTMLButtonElement).disabled,
    ).toBe(true);
    for (let i = 0; i < 10 && !service.downloadExport.mock.calls.length; i++) {
      await flush(10);
    }
    expect(service.getExport).toHaveBeenCalledWith("j1");
    expect(service.downloadExport).toHaveBeenCalledTimes(1);
    expect(
      $("[data-testid=statement-export]")?.getAttribute("data-status"),
    ).toBe("completed");
    await flush(20);
    expect(service.downloadExport).toHaveBeenCalledTimes(1);
    expect(
      ($("[data-testid=statement-export-xlsx]") as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  it("reports a failed job and downloads nothing", async () => {
    service.exportStatement.mockResolvedValue(job({ format: "csv" }));
    service.getExport.mockResolvedValue(
      job({ format: "csv", status: "failed", error: "boom" }),
    );
    await render(
      createElement(StatementExport, {
        orgUuid: "dealer-1",
        cariUuid: "c1",
        period: {},
        pollMs: 5,
      }),
    );
    await click($("[data-testid=statement-export-csv]"));
    for (let i = 0; i < 10; i++) await flush(10);
    expect(
      $("[data-testid=statement-export]")?.getAttribute("data-status"),
    ).toBe("failed");
    expect(service.downloadExport).not.toHaveBeenCalled();
  });
});

describe("dealer read-only cari view", () => {
  beforeEach(() => {
    service.getCari.mockResolvedValue(cari());
    service.listEntries.mockResolvedValue({
      items: [entry(), entry({ uuid: "e2", source_type: "manual" })],
      total: 2,
      limit: 10,
      offset: 0,
    });
  });

  it("shows the statement link and disputes but no write button", async () => {
    accessState.value = {
      ...accessState.value,
      canWrite: false,
      canDispute: true,
      readOnlyDealer: true,
    };
    await render(createElement(CariDetailPage, { slug: "acme", uuid: "c1" }));
    await flush();
    await flush();
    const actions = $("[data-testid=page-actions]");
    expect(actions?.textContent).not.toContain(
      "accounting.settlement.new_collection",
    );
    expect(actions?.textContent).not.toContain(
      "accounting.settlement.new_payment",
    );
    const link = $("[data-testid=cari-statement]");
    expect(link?.getAttribute("href")).toBe(
      "/t/acme/accounting/cari/c1/statement",
    );
    expect($("[data-testid=accounting-read-only]")).not.toBeNull();
    expect(
      $$("[data-testid=dispute-open]").map((b) => b.getAttribute("data-entry")),
    ).toEqual(["e1"]);
  });

  it("a writing distributor sees the settlement buttons", async () => {
    accessState.value = {
      ...accessState.value,
      orgType: "distributor",
      parentUuid: "center",
      canWrite: true,
      canDispute: false,
      readOnlyDealer: false,
    };
    await render(createElement(CariDetailPage, { slug: "acme", uuid: "c1" }));
    await flush();
    await flush();
    expect($("[data-testid=page-actions]")?.textContent).toContain(
      "accounting.settlement.new_collection",
    );
    expect($("[data-testid=accounting-read-only]")).toBeNull();
    expect($$("[data-testid=dispute-open]")).toHaveLength(0);
  });
});
