// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type {
  CancelQueueItem,
  ServiceSubscription,
} from "@/features/service-subscriptions/services/service-subscriptions.service";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  assign: vi.fn(),
  previewPrice: vi.fn(),
  requestCancel: vi.fn(),
  listCancelRequests: vi.fn(),
  approveCancel: vi.fn(),
  rejectCancel: vi.fn(),
  listOrganizations: vi.fn(),
}));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  org: { uuid: "dealer-1", slug: "acme", type: "dealer" } as Record<
    string,
    string
  > | null,
}));
const captured = vi.hoisted(() => ({
  tables: [] as Record<string, unknown>[],
}));

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: { href: string; children: unknown } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "en",
    format: {
      currency: (v: number | null, c: string) =>
        v === null ? "—" : `${v.toFixed(2)} ${c}`,
      date: (v: string) => `d(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => state.org,
}));
vi.mock("@/hooks/use-mobile", () => ({ useIsMobile: () => false }));
vi.mock(
  "@/features/service-subscriptions/services/service-subscriptions.service",
  async (orig) => ({
    ...(await orig<object>()),
    serviceSubscriptionsService: api,
  }),
);
vi.mock("@/features/service-catalog/services/service-catalog.service", () => ({
  serviceCatalogService: {
    listVisible: () => Promise.resolve({ items: [] }),
  },
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (props: Record<string, unknown>) => {
    captured.tables.push(props);
    return createElement("div", { "data-testid": "table" });
  },
}));

import { AssignSubscriptionDialog } from "@/features/service-subscriptions/components/assign-subscription-dialog";
import { CancelRequestsPage } from "@/features/service-subscriptions/components/cancel-requests-page";
import { SubscriptionDetailPage } from "@/features/service-subscriptions/components/subscription-detail-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  state.grants = new Set([
    "service_subscriptions.read",
    "service_subscriptions.cancel_request",
  ]);
  state.org = { uuid: "dealer-1", slug: "acme", type: "dealer" };
  api.listOrganizations.mockResolvedValue([]);
  api.previewPrice.mockResolvedValue({
    amount: "80.00",
    currency: "TRY",
    source: "override",
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
});

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render(node: ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await flush();
}

const byTestId = <T extends HTMLElement = HTMLElement>(id: string) =>
  document.querySelector<T>(`[data-testid="${id}"]`);

function typeInto(el: HTMLTextAreaElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLTextAreaElement.prototype,
    "value",
  )?.set;
  setter?.call(el, value);
  el.dispatchEvent(new Event("input", { bubbles: true }));
}

function subscription(over: Partial<ServiceSubscription> = {}) {
  return {
    uuid: "sub-1",
    organization_uuid: "dealer-1",
    item_uuid: "item-1",
    starts_on: "2026-10-01",
    ends_on: "2027-10-01",
    recurrence: "monthly",
    price: "80.00",
    currency: "TRY",
    rate_snapshot: {},
    cancellation_fee: "25.00",
    status: "active",
    created_at: "2026-10-01T08:00:00Z",
    organization_name: "Bayi 1",
    item_name: "Reklam paketi",
    item_category: "advertising",
    ...over,
  } as ServiceSubscription;
}

function queueItem(over: Partial<CancelQueueItem> = {}) {
  return {
    uuid: "req-1",
    subscription_uuid: "sub-1",
    reason: "not needed",
    status: "pending",
    cancellation_fee: "25.00",
    currency: "TRY",
    created_at: "2026-10-02T08:00:00Z",
    subscription_status: "cancel_requested",
    starts_on: "2026-10-01",
    ends_on: "2027-10-01",
    organization_uuid: "dealer-1",
    organization_name: "Bayi 1",
    item_name: "Reklam paketi",
    ...over,
  } as CancelQueueItem;
}

describe("AssignSubscriptionDialog (TEC-311)", () => {
  it("keeps Ata disabled while the end date is empty", async () => {
    await render(
      createElement(AssignSubscriptionDialog, {
        open: true,
        onOpenChange: () => {},
        orgType: "center",
        initial: {
          organizationUuid: "org-1",
          itemUuid: "item-1",
          startsOn: "2026-10-07",
        },
      }),
    );
    expect(byTestId<HTMLButtonElement>("assign-submit")?.disabled).toBe(true);

    act(() => root.unmount());
    root = createRoot(container);
    await render(
      createElement(AssignSubscriptionDialog, {
        open: true,
        onOpenChange: () => {},
        orgType: "center",
        initial: {
          organizationUuid: "org-1",
          itemUuid: "item-1",
          startsOn: "2026-10-07",
          endsOn: "2027-10-07",
        },
      }),
    );
    expect(byTestId<HTMLButtonElement>("assign-submit")?.disabled).toBe(false);
    // The center sees the effective price of the target as a preview.
    expect(api.previewPrice).toHaveBeenCalledWith("item-1", "org-1");
    expect(byTestId<HTMLInputElement>("assign-price")?.value).toBe("80.00 TRY");
  });

  it("shows the distributor price read-only (center price)", async () => {
    await render(
      createElement(AssignSubscriptionDialog, {
        open: true,
        onOpenChange: () => {},
        orgType: "distributor",
        initial: { organizationUuid: "dealer-1", itemUuid: "item-1" },
      }),
    );
    const price = byTestId<HTMLInputElement>("assign-price");
    expect(price?.readOnly).toBe(true);
    expect(price?.value).toBe("80.00 TRY");
    expect(document.body.textContent).toContain(
      "catalog.subscriptions.assign.price_locked",
    );
  });
});

describe("SubscriptionDetailPage early cancellation (TEC-311)", () => {
  it("moves the status badge to cancel_requested after the request", async () => {
    api.get.mockResolvedValue(subscription());
    api.requestCancel.mockResolvedValue({
      uuid: "req-1",
      subscription_uuid: "sub-1",
      reason: "budget",
      status: "pending",
      cancellation_fee: "25.00",
      currency: "TRY",
      created_at: "2026-10-07T08:00:00Z",
    });
    await render(
      createElement(SubscriptionDetailPage, { slug: "acme", uuid: "sub-1" }),
    );
    expect(byTestId("subscription-status")?.dataset.status).toBe("active");

    await act(async () => byTestId("open-cancel-request")?.click());
    await flush();
    expect(byTestId("cancel-fee-info")?.textContent).toContain("25.00 TRY");
    const submit = byTestId<HTMLButtonElement>("cancel-submit");
    expect(submit?.disabled).toBe(true);

    // The refetch after the request may still be in flight; the badge must
    // already show the new state.
    api.get.mockReturnValue(new Promise(() => {}));
    await act(async () =>
      typeInto(byTestId<HTMLTextAreaElement>("cancel-reason")!, "budget"),
    );
    expect(byTestId<HTMLButtonElement>("cancel-submit")?.disabled).toBe(false);
    await act(async () => byTestId("cancel-submit")?.click());
    await flush();

    expect(api.requestCancel).toHaveBeenCalledWith("sub-1", "budget");
    expect(byTestId("subscription-status")?.dataset.status).toBe(
      "cancel_requested",
    );
    expect(byTestId("open-cancel-request")).toBeNull();
  });

  it("shows the contract status and the PDF action when ready", async () => {
    state.grants.add("contracts.read");
    api.get.mockResolvedValue(
      subscription({
        contract: {
          uuid: "c1",
          contract_no: 7,
          status: "executed",
          pdf_ready: true,
        },
      }),
    );
    await render(
      createElement(SubscriptionDetailPage, { slug: "acme", uuid: "sub-1" }),
    );
    expect(byTestId("subscription-contract")?.textContent).toContain("#7");
    expect(byTestId("subscription-contract-pdf")).not.toBeNull();
  });
});

type QueueTable = {
  columns: ColumnDef<CancelQueueItem, unknown>[];
};

function queueActions(row: CancelQueueItem) {
  const table = captured.tables.at(-1) as unknown as QueueTable;
  const col = table.columns.find((c) => c.id === "actions");
  const cell = col?.cell as (ctx: unknown) => ReactElement<{
    actions: { id: string; onSelect: () => void }[];
  }>;
  return cell({ row: { original: row } }).props.actions;
}

describe("CancelRequestsPage (TEC-311)", () => {
  beforeEach(() => {
    state.org = { uuid: "center-1", slug: "acme", type: "center" };
    state.grants = new Set(["service_subscriptions.cancel_approve"]);
    api.listCancelRequests.mockResolvedValue({
      items: [queueItem()],
      total: 1,
      limit: 20,
      offset: 0,
    });
    api.approveCancel.mockResolvedValue({});
    api.rejectCancel.mockResolvedValue({});
  });

  it("opens on pending requests and approves through the approve endpoint", async () => {
    await render(createElement(CancelRequestsPage, { slug: "acme" }));
    expect(api.listCancelRequests).toHaveBeenLastCalledWith({
      limit: 20,
      offset: 0,
      sort: "-created_at",
      status: "pending",
    });
    const actions = queueActions(queueItem());
    expect(actions.map((a) => a.id)).toEqual(["view", "approve", "reject"]);
    await act(async () => actions.find((a) => a.id === "approve")!.onSelect());
    await flush();
    await act(async () => byTestId("decision-submit")?.click());
    await flush();
    expect(api.approveCancel).toHaveBeenCalledWith("req-1", "");
    expect(api.rejectCancel).not.toHaveBeenCalled();
  });

  it("rejects with a required reason through the reject endpoint", async () => {
    await render(createElement(CancelRequestsPage, { slug: "acme" }));
    const reject = queueActions(queueItem()).find((a) => a.id === "reject")!;
    await act(async () => reject.onSelect());
    await flush();
    expect(byTestId<HTMLButtonElement>("decision-submit")?.disabled).toBe(true);
    await act(async () =>
      typeInto(byTestId<HTMLTextAreaElement>("decision-note")!, "contract"),
    );
    await act(async () => byTestId("decision-submit")?.click());
    await flush();
    expect(api.rejectCancel).toHaveBeenCalledWith("req-1", "contract");
    expect(api.approveCancel).not.toHaveBeenCalled();
  });

  it("offers no decision on decided requests and is center only", async () => {
    await render(createElement(CancelRequestsPage, { slug: "acme" }));
    expect(
      queueActions(queueItem({ status: "approved" })).map((a) => a.id),
    ).toEqual(["view"]);

    act(() => root.unmount());
    root = createRoot(container);
    api.listCancelRequests.mockClear();
    state.org = { uuid: "dist-1", slug: "acme", type: "distributor" };
    await render(createElement(CancelRequestsPage, { slug: "acme" }));
    expect(api.listCancelRequests).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain(
      "catalog.subscriptions.queue.forbidden",
    );
  });
});
