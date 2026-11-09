// @vitest-environment jsdom
import { act, createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Permission } from "@/config/permissions";

const acc = vi.hoisted(() => ({
  queueSummary: vi.fn(),
  listQueueReturns: vi.fn(),
  listCari: vi.fn(),
  getCariBilling: vi.fn(),
  updateCariBilling: vi.fn(),
  markReturnProcessed: vi.fn(),
  unmarkReturnProcessed: vi.fn(),
}));
const orders = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn() }));
const granted = vi.hoisted(() => ({ list: [] as string[] }));
const search = vi.hoisted(() => ({ tab: "orders" }));

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: { href: string; children: unknown } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(`tab=${search.tab}`),
  usePathname: () => "/t/acme/accounting/queue",
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    t: (key: string) => key,
    format: {
      number: (v: number) => String(v),
      dateTime: (v: string) => v,
      currency: (v: number, c: string) => `${v} ${c}`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => granted.list.includes(p) }),
}));
vi.mock("@/features/accounting/hooks/use-accounting-access", async (orig) => ({
  ...(await orig<object>()),
  useAccountingAccess: () => ({
    orgUuid: "org-1",
    orgType: "center",
    parentUuid: null,
    canRead: true,
    canWrite: granted.list.includes("accounting.write"),
    canDispute: false,
    canResolve: true,
    readOnlyDealer: false,
  }),
}));
vi.mock("@/features/accounting/services/accounting.service", async (orig) => ({
  ...(await orig<object>()),
  accountingService: acc,
}));
vi.mock("@/features/orders/services/orders.service", async (orig) => ({
  ...(await orig<object>()),
  ordersService: orders,
}));

import { AccountingDashboard } from "@/features/accounting/components/accounting-dashboard";
import { CariBillingCard } from "@/features/accounting/components/cari-billing-card";
import {
  flush,
  mount,
  render,
  unmount,
  type Mounted,
} from "@/features/warehouse/components/test-helpers";

const page = <T,>(items: T[]) => ({
  items,
  total: items.length,
  limit: 6,
  offset: 0,
});

describe("accounting home and billing (TEC-1070)", () => {
  let m: Mounted;
  beforeEach(() => {
    m = mount();
    vi.clearAllMocks();
    granted.list = [
      Permission.AccountingRead,
      Permission.AccountingWrite,
      Permission.OrdersRead,
    ];
    acc.queueSummary.mockResolvedValue({
      orders_to_invoice: 2,
      orders_in_transit: 1,
      returns_pending: 3,
    });
    acc.listQueueReturns.mockResolvedValue(
      page([
        {
          uuid: "r1",
          transfer_no: "RT-1",
          sender: { type: "organization", uuid: "d1", name: "Bayi B" },
          item_count: 2,
          currency: "TRY",
          total: "50.00",
          received_at: "2026-11-01T10:00:00Z",
          created_at: "2026-11-01T09:00:00Z",
          processed_at: null,
          processed_by: null,
          note: null,
        },
      ]),
    );
    acc.listCari.mockResolvedValue(
      page([
        {
          uuid: "c1",
          counterparty: { type: "organization", uuid: "d1", name: "Bayi B" },
          currency: "TRY",
          active: true,
          balance: "1200.00",
          entry_count: 4,
          last_entry_at: null,
          created_at: "2026-10-01T00:00:00Z",
        },
      ]),
    );
    orders.list.mockResolvedValue(
      page([
        {
          uuid: "o1",
          order_no: "ORD-9",
          buyer: { uuid: "d1", name: "Bayi A", type: "dealer" },
          total: "300.00",
          currency: "TRY",
          received_at: "2026-11-02T10:00:00Z",
        },
      ]),
    );
  });
  afterEach(() => unmount(m));

  it("shows the queue counters, orders to invoice, returns and top caris", async () => {
    await render(m, createElement(AccountingDashboard, { slug: "acme" }));
    await flush();
    const kpi = (id: string) =>
      m.container
        .querySelector(`[data-kpi="${id}"]`)
        ?.getAttribute("data-count");
    expect(kpi("to_invoice")).toBe("2");
    expect(kpi("returns")).toBe("3");
    expect(orders.list).toHaveBeenCalledWith(
      expect.objectContaining({
        side: "seller",
        status: "received",
        invoice_status: "pending,partial",
      }),
    );
    expect(acc.listQueueReturns).toHaveBeenCalledWith(
      expect.objectContaining({ processed: false }),
    );
    expect(acc.listCari).toHaveBeenCalledWith(
      expect.objectContaining({ sort: "-balance" }),
    );
    expect(m.container.textContent).toContain("Bayi A");
    expect(m.container.textContent).toContain("RT-1");
  });

  it("shows the billing details and edits them when the book may", async () => {
    acc.getCariBilling.mockResolvedValue({
      title: "Bayi Ltd.",
      tax_office: "Kadıköy",
      tax_no: "123",
      address: null,
      email: null,
      iban: "TR330006100519786457841326",
      editable: true,
      updated_at: null,
      updated_by: null,
    });
    await render(
      m,
      createElement(CariBillingCard, {
        orgUuid: "org-1",
        cariUuid: "c1",
        canWrite: true,
      }),
    );
    await flush();
    expect(
      m.container.querySelector("[data-testid=cari-billing-title]")
        ?.textContent,
    ).toBe("Bayi Ltd.");
    expect(
      m.container.querySelector("[data-testid=cari-billing-address]")
        ?.textContent,
    ).toBe("—");
    expect(
      m.container.querySelector("[data-testid=cari-billing-edit]"),
    ).not.toBeNull();
  });

  it("hides the edit button when the details belong to the supplier", async () => {
    acc.getCariBilling.mockResolvedValue({
      title: null,
      tax_office: null,
      tax_no: null,
      address: null,
      email: null,
      iban: null,
      editable: false,
      updated_at: null,
      updated_by: null,
    });
    await render(
      m,
      createElement(CariBillingCard, {
        orgUuid: "org-1",
        cariUuid: "c1",
        canWrite: true,
      }),
    );
    await act(async () => {
      await flush();
    });
    expect(
      m.container.querySelector("[data-testid=cari-billing-edit]"),
    ).toBeNull();
  });
});
