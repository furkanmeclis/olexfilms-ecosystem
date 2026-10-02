// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  void: vi.fn(),
}));
const portal = vi.hoisted(() => ({ listWarranties: vi.fn() }));
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
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
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
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/features/warranty/services/warranty.service", async (orig) => ({
  ...(await orig<object>()),
  warrantyService: api,
}));
vi.mock("@/features/portal/lib/portal-client", async (orig) => ({
  ...(await orig<object>()),
  portalApi: portal,
}));

import { Permission } from "@/config/permissions";
import type { Warranty } from "@/features/warranty/lib/warranty-list";

import { PortalWarranties } from "./portal-warranties";
import { validVoidReason, WarrantyDetailPage } from "./warranty-detail-page";
import { WarrantiesListPage } from "./warranties-list-page";
import { WarrantyProgressBar } from "./warranty-progress";

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

async function click(el: Element | null) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
  await flush();
}

async function type(el: Element | null, value: string) {
  if (!(el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement))
    throw new Error("input not found");
  const proto =
    el instanceof HTMLInputElement
      ? HTMLInputElement.prototype
      : HTMLTextAreaElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
  await act(async () => {
    setter?.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await flush();
}

const DAY = 24 * 60 * 60 * 1000;

function warranty(over: Partial<Warranty> = {}): Warranty {
  const now = Date.now();
  return {
    uuid: "w-1",
    public_code: "PUBCODE123456",
    status: "active",
    item_kind: "full",
    start_at: new Date(now - 100 * DAY).toISOString(),
    end_at: new Date(now + 100 * DAY - 60_000).toISOString(),
    expired_at: null,
    voided_at: null,
    void_reason: null,
    created_at: new Date(now - 100 * DAY).toISOString(),
    product: { uuid: "p-1", sku: "PPF-1", name: "Film PPF" },
    service: { uuid: "s-1", service_no: "DS12345678" },
    organization: { uuid: "o-1", name: "Bayi A", type: "dealer" },
    vehicle: {
      uuid: "v-1",
      brand_name: "BMW",
      model_name: "X5",
      model_year: 2024,
      plate: "34 ABC 123",
    },
    holder: { uuid: "h-1", name: "Ayşe", surname: "Kaya", anonymized: false },
    can_void: false,
    ...over,
  };
}

function page(items: Warranty[], total = items.length) {
  return { items, total, limit: 20, offset: 0 };
}

describe("WarrantyProgressBar (TEC-191)", () => {
  it("shows days left and the elapsed share", async () => {
    await render(
      createElement(WarrantyProgressBar, {
        warranty: {
          status: "active",
          start_at: "2026-01-01T00:00:00Z",
          end_at: "2027-01-01T00:00:00Z",
        },
        now: new Date("2026-07-02T12:00:00Z"),
      }),
    );
    const bar = container.querySelector("[data-testid=warranty-progress]");
    expect(bar?.getAttribute("data-percent")).toBe("50");
    expect(bar?.getAttribute("data-days-left")).toBe("183");
    expect(
      container
        .querySelector("[role=progressbar]")
        ?.getAttribute("aria-valuenow"),
    ).toBe("50");
    expect(container.textContent).toContain(
      'warranty.progress.days_left {"days":183}',
    );
  });

  it("labels void and ended warranties", async () => {
    await render(
      createElement(WarrantyProgressBar, {
        warranty: {
          status: "void",
          start_at: "2026-01-01T00:00:00Z",
          end_at: "2027-01-01T00:00:00Z",
        },
      }),
    );
    expect(container.textContent).toContain("warranty.progress.void");
  });
});

describe("WarrantiesListPage", () => {
  it("is forbidden without warranties.read", async () => {
    await render(createElement(WarrantiesListPage, { slug: "acme" }));
    expect(api.list).not.toHaveBeenCalled();
    expect(container.textContent).toContain("warranty.list.forbidden");
  });

  it("lists rows and sends the status and ends-within filters", async () => {
    state.grants = new Set([Permission.WarrantiesRead]);
    api.list.mockResolvedValue(page([warranty()]));
    await render(createElement(WarrantiesListPage, { slug: "acme" }));
    expect(api.list).toHaveBeenLastCalledWith({ limit: 20, offset: 0 });
    const rows = container.querySelectorAll("[data-testid=warranty-row]");
    expect(rows).toHaveLength(1);
    expect(rows[0].textContent).toContain("Film PPF");
    expect(rows[0].textContent).toContain("Ayşe Kaya");
    expect(rows[0].querySelector("a")?.getAttribute("href")).toBe(
      "/t/acme/warranties/w-1",
    );
    // No catalog.read: no product picker.
    expect(container.querySelector("#warranty-product")).toBeNull();

    await click(container.querySelector("[data-status=expired]"));
    expect(api.list).toHaveBeenLastCalledWith({
      limit: 20,
      offset: 0,
      status: "expired",
    });
    await click(container.querySelector("[data-status=all]"));
    await click(container.querySelector("[data-ends-within='30']"));
    expect(api.list).toHaveBeenLastCalledWith({
      limit: 20,
      offset: 0,
      status: "active",
      days_left_max: 30,
    });
    await type(container.querySelector("#warranty-search"), "34ABC");
    expect(api.list).toHaveBeenLastCalledWith(
      expect.objectContaining({ q: "34ABC", days_left_max: 30 }),
    );
    await click(container.querySelector("[data-testid=clear-filters]"));
    expect(api.list).toHaveBeenLastCalledWith({ limit: 20, offset: 0 });
  });

  it("shows the empty state", async () => {
    state.grants = new Set([Permission.WarrantiesRead]);
    api.list.mockResolvedValue(page([]));
    await render(createElement(WarrantiesListPage, { slug: "acme" }));
    expect(
      container.querySelector("[data-testid=warranties-empty]"),
    ).not.toBeNull();
  });
});

describe("WarrantyDetailPage", () => {
  it("hides the void action without warranties.void", async () => {
    state.grants = new Set([Permission.WarrantiesRead]);
    api.get.mockResolvedValue(warranty({ can_void: true }));
    await render(
      createElement(WarrantyDetailPage, { slug: "acme", uuid: "w-1" }),
    );
    expect(container.textContent).toContain("Film PPF");
    expect(container.querySelector("[data-testid=void-open]")).toBeNull();
    // TEC-188: the certificate PDF of an active warranty's service.
    expect(
      container.querySelector("[data-testid=warranty-pdf]"),
    ).not.toBeNull();
  });

  it("voids with a reason", async () => {
    state.grants = new Set([
      Permission.WarrantiesRead,
      Permission.WarrantiesVoid,
    ]);
    api.get.mockResolvedValue(warranty({ can_void: true }));
    const voided = warranty({
      status: "void",
      can_void: false,
      voided_at: new Date().toISOString(),
      void_reason: "Hatalı kayıt",
    });
    api.void.mockResolvedValue(voided);
    await render(
      createElement(WarrantyDetailPage, { slug: "acme", uuid: "w-1" }),
    );
    await click(container.querySelector("[data-testid=void-open]"));
    const confirm = document.querySelector(
      "[data-testid=void-confirm]",
    ) as HTMLButtonElement | null;
    expect(confirm?.disabled).toBe(true);
    await type(document.querySelector("#void-reason"), "  Hatalı kayıt ");
    expect(confirm?.disabled).toBe(false);
    api.get.mockResolvedValue(voided);
    await click(confirm);
    expect(api.void).toHaveBeenCalledWith("w-1", "Hatalı kayıt");
    expect(
      container.querySelector("[data-testid=void-info]")?.textContent,
    ).toContain("Hatalı kayıt");
  });

  it("validates the reason length", () => {
    expect(validVoidReason("  ab ")).toBe(false);
    expect(validVoidReason("abc")).toBe(true);
    expect(validVoidReason("x".repeat(501))).toBe(false);
  });
});

describe("PortalWarranties", () => {
  it("lists the holder's warranties without the holder column", async () => {
    portal.listWarranties.mockResolvedValue(
      page([
        warranty({ holder: undefined }),
        warranty({ uuid: "w-2", status: "expired", holder: undefined }),
      ]),
    );
    await render(createElement(PortalWarranties));
    expect(portal.listWarranties).toHaveBeenLastCalledWith({
      limit: 10,
      offset: 0,
    });
    const items = container.querySelectorAll("[data-testid=portal-warranty]");
    expect(items).toHaveLength(2);
    expect(items[0].textContent).toContain("Film PPF");
    expect(items[0].textContent).toContain("34 ABC 123");
    expect(container.textContent).not.toContain("Ayşe");
    expect(items[0].querySelector("a")?.getAttribute("href")).toBe(
      "/garanti/PUBCODE123456",
    );
    // PDF only for the active warranty.
    expect(
      container.querySelector("[data-testid=warranty-pdf-w-1]"),
    ).not.toBeNull();
    expect(
      container.querySelector("[data-testid=warranty-pdf-w-2]"),
    ).toBeNull();
    // No product filter in the portal.
    expect(container.querySelector("#warranty-product")).toBeNull();

    await click(container.querySelector("[data-ends-within='7']"));
    expect(portal.listWarranties).toHaveBeenLastCalledWith({
      limit: 10,
      offset: 0,
      status: "active",
      days_left_max: 7,
    });
  });

  it("shows the empty state", async () => {
    portal.listWarranties.mockResolvedValue(page([]));
    await render(createElement(PortalWarranties));
    expect(
      container.querySelector("[data-testid=portal-warranties-empty]"),
    ).not.toBeNull();
  });
});
