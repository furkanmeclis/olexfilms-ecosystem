// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  targets: vi.fn(),
  create: vi.fn(),
  transition: vi.fn(),
}));
const state = vi.hoisted(() => ({ grants: new Set<string>() }));

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
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
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
vi.mock("@/features/transfers/services/transfers.service", async (orig) => ({
  ...(await orig<object>()),
  transfersService: api,
}));

import { Permission } from "@/config/permissions";
import type { StockTransfer } from "@/features/transfers/services/transfers.service";

import { TransferDetailPage } from "./transfer-detail-page";
import { TransferFormPage } from "./transfer-form-page";
import { TransfersListPage } from "./transfers-list-page";

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
  state.grants = new Set();
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

const org = (name: string) => ({ uuid: `${name}-uuid`, name, type: "dealer" });

function transfer(patch: Partial<StockTransfer> = {}): StockTransfer {
  return {
    uuid: "t-1",
    transfer_no: "TRF-00000001",
    kind: "sibling",
    status: "requested",
    role: "receiver",
    sender: org("Bayi A"),
    receiver: org("Bayi B"),
    parent: { ...org("Dist"), type: "distributor" },
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
    items: [
      {
        uuid: "i-1",
        unit_uuid: "u-1",
        barcode: "OLX-1",
        unit_kind: "serial",
        product: { uuid: "p-1", sku: "P1", name: "Film", unit_type: "piece" },
        quantity: null,
        meters: null,
        unit_price: null,
        line_total: null,
        shipped: false,
        received: false,
        restored: false,
      },
    ],
    ...patch,
  } as StockTransfer;
}

describe("TransferDetailPage (TEC-197)", () => {
  it("renders one button per available transition and posts it", async () => {
    state.grants = new Set([Permission.TransfersRequest]);
    api.get.mockResolvedValue(transfer());
    api.transition.mockResolvedValue(
      transfer({ status: "approved", available_transitions: ["cancelled"] }),
    );
    await render(createElement(TransferDetailPage, { slug: "s", uuid: "t-1" }));
    const buttons = [...container.querySelectorAll("[data-transition]")].map(
      (b) => b.getAttribute("data-transition"),
    );
    expect(buttons).toEqual(["approved", "rejected"]);
    expect(container.textContent).toContain("OLX-1");

    // The refetch after the move reads the server's new state.
    api.get.mockResolvedValue(
      transfer({ status: "approved", available_transitions: ["cancelled"] }),
    );
    await act(async () => {
      (
        container.querySelector('[data-transition="approved"]') as HTMLElement
      ).click();
    });
    await flush();
    expect(api.transition).toHaveBeenCalledWith("t-1", "approved", undefined);
    expect(
      [...container.querySelectorAll("[data-transition]")].map((b) =>
        b.getAttribute("data-transition"),
      ),
    ).toEqual(["cancelled"]);
  });

  it("shows no action without available transitions", async () => {
    state.grants = new Set([Permission.TransfersRequest]);
    api.get.mockResolvedValue(
      transfer({ status: "received", available_transitions: [] }),
    );
    await render(createElement(TransferDetailPage, { slug: "s", uuid: "t-1" }));
    expect(container.querySelectorAll("[data-transition]")).toHaveLength(0);
  });

  it("is forbidden without a transfer permission", async () => {
    await render(createElement(TransferDetailPage, { slug: "s", uuid: "t-1" }));
    expect(api.get).not.toHaveBeenCalled();
    expect(container.textContent).toContain("common.error_forbidden");
  });
});

describe("TransfersListPage (TEC-197)", () => {
  it("lists requests and filters by direction", async () => {
    state.grants = new Set([Permission.TransfersApprove]);
    api.list.mockResolvedValue({
      items: [transfer({ role: "parent" })],
      total: 1,
      limit: 20,
      offset: 0,
    });
    await render(createElement(TransfersListPage, { slug: "s" }));
    expect(
      container.querySelectorAll('[data-testid="transfer-row"]'),
    ).toHaveLength(1);
    expect(container.querySelector('[data-testid="transfer-new"]')).toBeNull();
    await act(async () => {
      (
        container.querySelector('[data-direction="approval"]') as HTMLElement
      ).click();
    });
    await flush();
    expect(api.list).toHaveBeenLastCalledWith(
      expect.objectContaining({ direction: "approval", offset: 0 }),
    );
  });
});

describe("Returns (TEC-223)", () => {
  it("filters the list by kind", async () => {
    state.grants = new Set([Permission.TransfersRequest]);
    api.list.mockResolvedValue({ items: [], total: 0, limit: 20, offset: 0 });
    await render(createElement(TransfersListPage, { slug: "s" }));
    expect(
      container.querySelector('[data-testid="transfer-new-return"]'),
    ).not.toBeNull();
    await act(async () => {
      (container.querySelector('[data-kind="return"]') as HTMLElement).click();
    });
    await flush();
    expect(api.list).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: "return", offset: 0 }),
    );
  });

  it("sends a return to the parent the server lists", async () => {
    state.grants = new Set([Permission.TransfersRequest]);
    api.targets.mockResolvedValue({
      items: [{ ...org("Dist"), type: "distributor" }],
    });
    api.create.mockResolvedValue(transfer({ kind: "return", role: "sender" }));
    await render(
      createElement(TransferFormPage, { slug: "s", kind: "return" }),
    );
    expect(api.targets).toHaveBeenCalledWith("return");
    expect(
      container.querySelector('[data-testid="return-parent"]')?.textContent,
    ).toBe("Dist");
    const input = container.querySelector(
      '[data-testid="transfer-scan"]',
    ) as HTMLInputElement;
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value",
      )?.set;
      setter?.call(input, "OLX-9");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      (
        container.querySelector('[data-testid="transfer-add"]') as HTMLElement
      ).click();
    });
    await act(async () => {
      (
        container.querySelector(
          '[data-testid="transfer-submit"]',
        ) as HTMLElement
      ).click();
    });
    await flush();
    expect(api.create).toHaveBeenCalledWith({
      kind: "return",
      to_org_uuid: "Dist-uuid",
      items: [{ barcode: "OLX-9" }],
    });
  });
});
