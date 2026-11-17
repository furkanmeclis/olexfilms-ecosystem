// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  increaseUnitCredits: vi.fn(),
  isSuperAdmin: false,
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, v?: Record<string, unknown>) =>
      v ? `${key} ${JSON.stringify(v)}` : key,
    format: {
      number: (n: number) => String(n),
      dateTime: (d: string) => d,
    },
  }),
}));
vi.mock("@/providers/toast-provider", () => ({ appToast: toast }));
vi.mock("@/lib/auth/session-store", () => ({
  useAuthStore: (
    selector: (state: { user: { isSuperAdmin: boolean } }) => unknown,
  ) => selector({ user: { isSuperAdmin: api.isSuperAdmin } }),
}));
vi.mock("@/features/stock/services/stock-ops.service", () => ({
  stockOpsService: { increaseUnitCredits: api.increaseUnitCredits },
  stockOpsKeys: { unit: (b: string) => ["stock", "unit", b] },
}));

import { UnitCreditsCard, creditIncreaseErrors } from "./unit-credits-card";
import type { StockUnitCredits } from "@/features/stock/services/stock-ops.service";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  api.increaseUnitCredits.mockReset();
  api.isSuperAdmin = false;
  toast.success.mockReset();
  toast.error.mockReset();
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

const credits: StockUnitCredits = {
  unit_uuid: "unit-1",
  credits_total: 5,
  credits_remaining: 3,
  used_count: 2,
  services: [
    {
      uuid: "s1",
      service_no: "SRV-1",
      status: "completed",
      finished_product: false,
      credits_taken: 1,
      created_at: "2026-11-01T00:00:00Z",
      completed_at: "2026-11-02T00:00:00Z",
    },
    {
      uuid: "s2",
      service_no: "SRV-2",
      status: "cancelled",
      finished_product: true,
      credits_taken: 0,
      created_at: "2026-11-03T00:00:00Z",
      completed_at: null,
    },
  ],
  movements: [
    {
      uuid: "m1",
      type: "service_use",
      total_delta: 0,
      remaining_delta: -1,
      reason: null,
      service_uuid: "s1",
      service_no: "SRV-1",
      actor_name: "Ali",
      created_at: "2026-11-01T00:00:00Z",
    },
    {
      uuid: "m2",
      type: "increase",
      total_delta: 2,
      remaining_delta: 2,
      reason: "replacement",
      service_uuid: null,
      service_no: null,
      actor_name: "Admin",
      created_at: "2026-11-04T00:00:00Z",
    },
  ],
};

function render() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  act(() =>
    root.render(
      createElement(
        QueryClientProvider,
        { client: qc },
        createElement(UnitCreditsCard, {
          credits,
          barcode: "P-001",
          slug: "acme",
        }),
      ),
    ),
  );
}

async function flush() {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

function typeInto(el: Element | null, value: string) {
  const proto =
    el instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
  act(() => {
    setter?.call(el, value);
    el?.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

const $ = (sel: string) => document.querySelector(sel);

describe("UnitCreditsCard (TEC-1221)", () => {
  it("shows the credits, the services and the credit ledger", () => {
    render();
    expect($("[data-testid=credits-remaining]")?.textContent).toBe("3 / 5");
    expect($("[data-testid=credits-used]")?.textContent).toBe("2");
    const services = document.querySelectorAll(
      "[data-testid=credits-services] li",
    );
    expect(services).toHaveLength(2);
    expect(services[0].textContent).toContain("SRV-1");
    expect(services[0].textContent).toContain(
      'stock.credits.taken {"credits":"1"}',
    );
    expect(services[0].querySelector("a")?.getAttribute("href")).toBe(
      "/t/acme/services/s1",
    );
    expect(services[1].textContent).toContain("stock.credits.refunded");
    expect(services[1].textContent).toContain("stock.credits.finished");
    const moves = document.querySelectorAll(
      "[data-testid=credits-movements] li",
    );
    expect(moves[0].textContent).toContain("stock.credits.type.service_use");
    expect(moves[0].textContent).toContain(
      'stock.credits.delta_remaining {"delta":"-1"}',
    );
    expect(moves[0].textContent).not.toContain("stock.credits.delta_total");
    expect(moves[1].textContent).toContain(
      'stock.credits.delta_total {"delta":"+2"}',
    );
    expect(moves[1].textContent).toContain("replacement");
    // Only the platform super admin may add credits.
    expect($("[data-testid=credits-increase]")).toBeNull();
  });

  it("lets the super admin add credits with a reason", async () => {
    api.isSuperAdmin = true;
    api.increaseUnitCredits.mockResolvedValue({
      ...credits,
      credits_total: 7,
      credits_remaining: 5,
    });
    render();
    act(() => {
      ($("[data-testid=credits-increase]") as HTMLButtonElement).click();
    });
    expect($("[data-testid=credits-dialog]")).not.toBeNull();
    typeInto($("#credits-amount"), "2");
    act(() => {
      ($("[data-testid=credits-submit]") as HTMLButtonElement).click();
    });
    expect(api.increaseUnitCredits).not.toHaveBeenCalled();
    expect($("[data-testid=credits-dialog]")?.textContent).toContain(
      "stock.credits.reason_required",
    );
    typeInto($("#credits-reason"), " müşteri değişimi ");
    act(() => {
      ($("[data-testid=credits-submit]") as HTMLButtonElement).click();
    });
    await flush();
    expect(api.increaseUnitCredits).toHaveBeenCalledWith("unit-1", {
      amount: 2,
      reason: "müşteri değişimi",
    });
    expect(toast.success).toHaveBeenCalledWith("stock.credits.increased");
  });
});

describe("creditIncreaseErrors", () => {
  it("needs a whole amount 1..1000 and a reason", () => {
    expect(creditIncreaseErrors({ amount: "1", reason: "x" })).toEqual({});
    expect(creditIncreaseErrors({ amount: "1000", reason: "x" })).toEqual({});
    expect(creditIncreaseErrors({ amount: "0", reason: "x" }).amount).toBe(
      "stock.credits.amount_invalid",
    );
    expect(creditIncreaseErrors({ amount: "1001", reason: "x" }).amount).toBe(
      "stock.credits.amount_invalid",
    );
    expect(creditIncreaseErrors({ amount: "1.5", reason: "x" }).amount).toBe(
      "stock.credits.amount_invalid",
    );
    expect(creditIncreaseErrors({ amount: "2", reason: "  " }).reason).toBe(
      "stock.credits.reason_required",
    );
  });
});
