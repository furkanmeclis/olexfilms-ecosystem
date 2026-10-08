// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const portal = vi.hoisted(() => ({
  getFleetOverview: vi.fn(),
  listFleetLinks: vi.fn(),
  acceptFleetLink: vi.fn(),
  rejectFleetLink: vi.fn(),
  getFleetAccounting: vi.fn(),
}));
const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

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
vi.mock("sonner", () => ({ toast: toasts }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    dir: "ltr",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
      currency: (v: number, c: string) => `${v.toFixed(2)} ${c}`,
    },
  }),
}));
vi.mock("@/components/entity", async (orig) => {
  const actual = await orig<object>();
  return {
    ...actual,
    EntityToolbar: () => createElement("button", { type: "button" }, "refresh"),
    EntityTable: ({
      data,
      emptyTitle,
    }: {
      data?: Array<Record<string, unknown>>;
      emptyTitle?: string;
    }) =>
      createElement(
        "div",
        { "data-testid": "entity-table" },
        data?.length
          ? data
              .map((row) =>
                [row.dealerName, row.kind, row.balance]
                  .filter(Boolean)
                  .join(" "),
              )
              .join(" | ")
          : emptyTitle,
      ),
  };
});
vi.mock("@/features/portal/lib/portal-client", async (orig) => ({
  ...(await orig<object>()),
  portalApi: portal,
}));

import {
  PortalFleetAccount,
  PortalFleetHome,
} from "@/features/portal/components/portal-fleet";
import type {
  FleetLink,
  FleetPortalAccounting,
  FleetPortalOverview,
} from "@/features/portal/lib/portal-client";

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

async function click(el: Element | null) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
  await flush();
}

function link(over: Partial<FleetLink> = {}): FleetLink {
  return {
    uuid: "link-1",
    status: "pending",
    dealer_uuid: "dealer-1",
    dealer_name: "Bayi Kadıköy",
    started_at: null,
    ended_at: null,
    created_at: "2026-10-01T10:00:00Z",
    ...over,
  };
}

function overview(): FleetPortalOverview {
  return {
    fleet: { uuid: "fleet-1", name: "Acme Fleet" },
    period_from: "2026-10-01",
    period_to: "2026-10-31",
    vehicle_count: 2,
    service_count: 3,
    active_warranty_count: 1,
    upcoming_appointment_count: 0,
    upcoming_appointments: [],
    dealers: [],
    pending_link_count: 1,
  };
}

describe("PortalFleetHome", () => {
  it("opens a confirmation dialog before rejecting a dealer link", async () => {
    portal.getFleetOverview.mockResolvedValue(overview());
    portal.listFleetLinks.mockResolvedValue({
      items: [link()],
      total: 1,
      limit: 20,
      offset: 0,
    });
    portal.rejectFleetLink.mockResolvedValue(link({ status: "ended" }));

    await render(createElement(PortalFleetHome));
    await click(
      container.querySelector('[data-testid="portal-fleet-link-reject-open"]'),
    );
    const dialog = document.body.querySelector(
      '[data-testid="portal-fleet-link-reject-dialog"]',
    );
    expect(dialog?.textContent).toContain("portal.fleet.links.reject_title");
    expect(portal.rejectFleetLink).not.toHaveBeenCalled();

    await click(
      document.body.querySelector(
        '[data-testid="portal-fleet-link-reject-confirm"]',
      ),
    );
    expect(portal.rejectFleetLink).toHaveBeenCalledWith("link-1");
    expect(toasts.success).toHaveBeenCalledWith("portal.fleet.links.rejected");
  });
});

describe("PortalFleetAccount", () => {
  const accounting: FleetPortalAccounting = {
    period_from: "2026-10-01",
    period_to: "2026-10-31",
    dealers: [
      {
        dealer: { uuid: "dealer-1", name: "Bayi Kadıköy" },
        link_status: "active",
        currency: "TRY",
        opening_balance: "0",
        service_income_total: "100",
        collection_total: "20",
        closing_balance: "80",
        lines: [
          {
            uuid: "line-1",
            date: "2026-10-02T10:00:00Z",
            kind: "service_income",
            debit: "100",
            credit: "0",
            balance: "100",
            is_reversal: false,
            service: null,
          },
        ],
      },
      {
        dealer: { uuid: "dealer-2", name: "Bayi Ankara" },
        link_status: "active",
        currency: "TRY",
        opening_balance: "0",
        service_income_total: "50",
        collection_total: "50",
        closing_balance: "0",
        lines: [
          {
            uuid: "line-2",
            date: "2026-10-03T10:00:00Z",
            kind: "collection",
            debit: "0",
            credit: "50",
            balance: "0",
            is_reversal: false,
            service: null,
          },
        ],
      },
    ],
  };

  it("filters account movements by dealer", async () => {
    portal.getFleetAccounting.mockResolvedValue(accounting);
    await render(createElement(PortalFleetAccount));
    expect(portal.getFleetAccounting).toHaveBeenCalled();
    expect(container.textContent).toContain("Bayi Kadıköy");
    expect(container.textContent).toContain("Bayi Ankara");

    const filter = container.querySelector(
      '[data-testid="portal-fleet-account-dealer-filter"]',
    );
    expect(filter).toBeInstanceOf(HTMLSelectElement);
    await act(async () => {
      (filter as HTMLSelectElement).value = "dealer-2";
      filter?.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await flush();

    expect(
      container.querySelector('[data-testid="entity-table"]')?.textContent,
    ).toContain("Bayi Ankara");
    expect(
      container.querySelector('[data-testid="entity-table"]')?.textContent,
    ).not.toContain("Bayi Kadıköy");
  });
});
