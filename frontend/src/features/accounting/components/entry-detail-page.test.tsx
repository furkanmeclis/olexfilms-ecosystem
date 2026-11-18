// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, Fragment, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { FinanceEntry } from "@/features/accounting/services/accounting.service";

const state = vi.hoisted(() => ({
  allow: true,
  getEntry: vi.fn(),
  listCategories: vi.fn(),
}));

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: { href: string; children: ReactNode } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string>) =>
      params ? `${key} ${params.source} #${params.id}` : key,
    locale: "en",
    format: {
      number: (v: number) => String(v),
      date: (v: string) => v,
      dateTime: (v: string) => `dt(${v})`,
      currency: (v: number, c: string) => `${v} ${c}`,
    },
  }),
}));
vi.mock("@/lib/toast", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => state.allow }),
}));
vi.mock("@/features/organizations/providers/tenant-provider", () => ({
  useOptionalTenant: () => ({ slug: "olex" }),
}));
vi.mock("@/features/accounting/hooks/use-accounting-access", async (orig) => ({
  ...(await orig<object>()),
  useAccountingAccess: () => ({
    orgUuid: "org-1",
    orgType: "center",
    parentUuid: null,
    canRead: true,
    canWrite: false,
    canDispute: false,
    canResolve: false,
    readOnlyDealer: false,
  }),
}));
vi.mock("@/features/accounting/services/accounting.service", async (orig) => ({
  ...(await orig<object>()),
  accountingService: {
    getEntry: state.getEntry,
    listCategories: state.listCategories,
  },
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: ReactNode }) =>
    createElement(Fragment, null, children),
}));

import { entrySourceKind } from "@/features/accounting/lib/entry-source";

import { EntryDetailPage } from "./entry-detail-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const ORDER = "0a1b2c3d-1111-2222-3333-444455556666";
const CARI = "9f8e7d6c-1111-2222-3333-444455556666";

function entry(over: Partial<FinanceEntry> = {}): FinanceEntry {
  return {
    uuid: "e-1",
    direction: "income",
    category: "sale",
    category_label_key: "accounting.category.sale",
    account: null,
    cari_uuid: CARI,
    counterparty_organization: { uuid: "org-2", name: "Dealer A" },
    orig_currency: "TRY",
    orig_amount: "100.00",
    currency: "TRY",
    amount: "100.00",
    rate: "1.00000000",
    rate_date: "2026-11-01",
    source_type: "order",
    source_uuid: ORDER,
    revision: 1,
    reversal_of_uuid: null,
    reversed_by_uuid: null,
    voided: false,
    description: "Sale",
    created_at: "2026-11-01T10:00:00Z",
    ...over,
  } as FinanceEntry;
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  state.allow = true;
  state.listCategories.mockResolvedValue({ items: [] });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
});

async function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(EntryDetailPage, { slug: "olex", uuid: "e-1" }),
      ),
    );
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

describe("entrySourceKind (TEC-1218)", () => {
  it("maps posting sources to record kinds", () => {
    expect(entrySourceKind("order")).toBe("order");
    expect(entrySourceKind("service_income")).toBe("service");
    expect(entrySourceKind("stock_return")).toBe("transfer");
    expect(entrySourceKind("warranty_claim")).toBe("warranty_claim");
    expect(entrySourceKind("opening_balance")).toBe("cari");
    expect(entrySourceKind("manual")).toBeNull();
    expect(entrySourceKind(null)).toBeNull();
  });
});

describe("EntryDetailPage (TEC-1218)", () => {
  it("links the source record and the cari account", async () => {
    state.getEntry.mockResolvedValue(entry());
    await renderPage();

    expect(state.getEntry).toHaveBeenCalledWith("e-1");
    const related = container.querySelector("[data-testid='entry-related']");
    const source = related?.querySelector("a[data-entity-kind='order']");
    expect(source?.getAttribute("href")).toBe(`/t/olex/orders/${ORDER}`);
    expect(source?.textContent).toBe(
      "accounting.entry_detail.source_ref accounting.sources.order #0a1b2c3d",
    );
    const cari = related?.querySelector("a[data-entity-kind='cari']");
    expect(cari?.getAttribute("href")).toBe(`/t/olex/accounting/cari/${CARI}`);
    expect(cari?.textContent).toBe("Dealer A");
  });

  it("renders plain text without permission or a page", async () => {
    state.allow = false;
    state.getEntry.mockResolvedValue(entry());
    await renderPage();
    const related = container.querySelector("[data-testid='entry-related']");
    expect(related?.querySelector("a[data-entity-kind]")).toBeNull();
    expect(related?.textContent).toContain(
      "accounting.sources.order #0a1b2c3d",
    );
  });

  it("keeps a source without a page as text", async () => {
    state.getEntry.mockResolvedValue(
      entry({ source_type: "staff_payment", source_uuid: ORDER }),
    );
    await renderPage();
    const related = container.querySelector("[data-testid='entry-related']");
    expect(
      related
        ?.querySelector("a[data-entity-kind]")
        ?.getAttribute("data-entity-kind"),
    ).toBe("cari");
    expect(related?.textContent).toContain(
      "accounting.sources.staff_payment #0a1b2c3d",
    );
  });
});
