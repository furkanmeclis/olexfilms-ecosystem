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
const stockApi = vi.hoisted(() => ({ listUnits: vi.fn() }));

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
vi.mock("@/features/stock/services/stock.service", async (orig) => ({
  ...(await orig<object>()),
  stockService: stockApi,
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ uuid: "me-uuid", slug: "s", type: "dealer" }),
}));

import { Permission } from "@/config/permissions";
import type { StockTransfer } from "@/features/transfers/services/transfers.service";

import { TransferDetailPage } from "./transfer-detail-page";
import { TransferFormPage } from "./transfer-form-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
// DataTable reads the mobile breakpoint (jsdom has no matchMedia).
window.matchMedia ??= ((query: string) => ({
  matches: false,
  media: query,
  onchange: null,
  addEventListener: () => {},
  removeEventListener: () => {},
  addListener: () => {},
  removeListener: () => {},
  dispatchEvent: () => false,
})) as typeof window.matchMedia;
(
  globalThis as typeof globalThis & { ResizeObserver?: typeof ResizeObserver }
).ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as typeof ResizeObserver;

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
    // Units are a nested DataTable (TEC-374).
    expect(
      container.querySelectorAll('table [data-testid="transfer-item"]'),
    ).toHaveLength(1);
    expect(container.textContent).toContain("OLX-1");
    expect(container.textContent).toContain("transfers.item.pending");

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

describe("Returns (TEC-223)", () => {
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

const unitRow = (patch: Record<string, unknown>) => ({
  uuid: `u-${String(patch.barcode)}`,
  barcode: "B",
  unit_kind: "serial",
  status: "available",
  quantity: 1,
  initial_meters: null,
  remaining_meters: null,
  product: {
    uuid: "p-1",
    sku: "SKU-1",
    name: "Film",
    unit_type: "piece",
    uses_fixed_barcode: false,
  },
  category: { uuid: "c-1", name: "PPF" },
  location: null,
  purchase_price: null,
  updated_at: "2026-10-02T10:00:00Z",
  ...patch,
});

function pickRow(barcode: string): HTMLElement {
  return [
    ...container.querySelectorAll('[data-testid="transfer-stock-row"]'),
  ].find((r) => r.textContent?.includes(barcode)) as HTMLElement;
}

async function click(el: Element | null) {
  await act(async () => {
    (el as HTMLElement).click();
  });
}

async function type(el: Element | null, value: string) {
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )?.set;
    setter?.call(el, value);
    el!.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

describe("Stock picker (TEC-870)", () => {
  beforeEach(() => {
    api.targets.mockResolvedValue({
      items: [{ ...org("Dist"), type: "distributor" }],
    });
    stockApi.listUnits.mockResolvedValue({
      items: [
        unitRow({
          barcode: "ROLL-1",
          remaining_meters: "12.50",
          product: {
            uuid: "p-2",
            sku: "R-1",
            name: "Roll",
            unit_type: "roll_meter",
            uses_fixed_barcode: false,
          },
        }),
        unitRow({ barcode: "PC-1" }),
        unitRow({ barcode: "FIX-1", unit_kind: "fixed", quantity: 3 }),
      ],
      total: 3,
      limit: 20,
      offset: 0,
    });
  });

  it("lists the units on hand and picks them into the request", async () => {
    state.grants = new Set([Permission.TransfersRequest, Permission.StockRead]);
    api.create.mockResolvedValue(transfer({ kind: "return", role: "sender" }));
    await render(
      createElement(TransferFormPage, { slug: "s", kind: "return" }),
    );

    expect(stockApi.listUnits).toHaveBeenCalledWith(
      "me-uuid",
      expect.objectContaining({ status: "available,placed", offset: 0 }),
    );
    expect(
      container.querySelectorAll('[data-testid="transfer-stock-row"]'),
    ).toHaveLength(3);
    expect(pickRow("ROLL-1").textContent).toContain("12.50");
    expect(pickRow("PC-1").textContent).toContain("PPF");
    expect(pickRow("FIX-1").textContent).toContain('{"count":"3"}');

    const pick = (barcode: string) =>
      pickRow(barcode).querySelector('[data-testid="transfer-stock-pick"]');
    await click(pick("ROLL-1"));
    await click(pick("FIX-1"));
    expect(pickRow("ROLL-1").dataset.picked).toBe("true");
    expect(
      pickRow("ROLL-1").querySelector('[data-testid="transfer-stock-added"]'),
    ).not.toBeNull();
    expect(pickRow("PC-1").dataset.picked).toBe("false");
    const lines = container.querySelectorAll('[data-testid="transfer-line"]');
    expect(lines).toHaveLength(2);
    // The roll moves whole: no quantity input.
    expect(lines[0]!.querySelector("input")).toBeNull();
    expect(
      lines[0]!.querySelector('[data-testid="transfer-line-whole"]'),
    ).not.toBeNull();

    // A fixed barcode starts at 1 and may not exceed its stock.
    const qty = container.querySelector("#qty-1") as HTMLInputElement;
    expect(qty.value).toBe("1");
    await type(qty, "4");
    expect(
      container.querySelector('[data-testid="transfer-line-over"]'),
    ).not.toBeNull();
    const submit = container.querySelector(
      '[data-testid="transfer-submit"]',
    ) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    await type(qty, "3");
    expect(
      container.querySelector('[data-testid="transfer-line-over"]'),
    ).toBeNull();

    // Unticking removes the line again.
    await click(pick("ROLL-1"));
    expect(
      container.querySelectorAll('[data-testid="transfer-line"]'),
    ).toHaveLength(1);

    await click(container.querySelector('[data-testid="transfer-submit"]'));
    await flush();
    expect(api.create).toHaveBeenCalledWith({
      kind: "return",
      to_org_uuid: "Dist-uuid",
      items: [{ barcode: "FIX-1", quantity: 3 }],
    });
  });

  it("marks a scanned barcode as added and never adds it twice", async () => {
    state.grants = new Set([Permission.TransfersRequest, Permission.StockRead]);
    await render(
      createElement(TransferFormPage, { slug: "s", kind: "return" }),
    );
    const scan = () => container.querySelector('[data-testid="transfer-scan"]');
    await type(scan(), "PC-1");
    await click(container.querySelector('[data-testid="transfer-add"]'));
    expect(pickRow("PC-1").dataset.picked).toBe("true");
    await type(scan(), "PC-1");
    await click(container.querySelector('[data-testid="transfer-add"]'));
    expect(
      container.querySelectorAll('[data-testid="transfer-line"]'),
    ).toHaveLength(1);
  });

  it("searches the stock on the server", async () => {
    state.grants = new Set([Permission.TransfersRequest, Permission.StockRead]);
    await render(
      createElement(TransferFormPage, { slug: "s", kind: "return" }),
    );
    await type(
      container.querySelector('[data-testid="transfer-stock-search"]'),
      "FIX",
    );
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 350));
    });
    await flush();
    expect(stockApi.listUnits).toHaveBeenLastCalledWith(
      "me-uuid",
      expect.objectContaining({ q: "FIX", offset: 0 }),
    );
  });

  it("hides the picker without stock.read; scanning still works", async () => {
    state.grants = new Set([Permission.TransfersRequest]);
    await render(
      createElement(TransferFormPage, { slug: "s", kind: "return" }),
    );
    expect(
      container.querySelector('[data-testid="transfer-stock-picker"]'),
    ).toBeNull();
    expect(stockApi.listUnits).not.toHaveBeenCalled();
    expect(
      container.querySelector('[data-testid="transfer-scan"]'),
    ).not.toBeNull();
  });
});
