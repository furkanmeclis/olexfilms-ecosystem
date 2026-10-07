// @vitest-environment jsdom
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: {
      number: (v: number) => v.toFixed(2),
      date: (v: string) => v,
      dateTime: (v: string) => v,
    },
  }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));

import { resolveServiceIncomeAccess } from "@/features/services/lib/access";
import type { FinanceAccount } from "@/features/accounting/services/accounting.service";
import { ServiceIncomeForm, ServiceProfitCard } from "./service-income";

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
});

async function render(node: ReactNode) {
  await act(async () => {
    root.render(node);
  });
}

async function choose(el: Element | null, value: string) {
  if (!(el instanceof HTMLSelectElement)) throw new Error("select not found");
  const setter = Object.getOwnPropertyDescriptor(
    HTMLSelectElement.prototype,
    "value",
  )?.set;
  await act(async () => {
    setter?.call(el, value);
    el.dispatchEvent(new Event("change", { bubbles: true }));
  });
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

async function submit() {
  await act(async () => {
    (
      container.querySelector(
        '[data-testid="service-income-form"]',
      ) as HTMLFormElement
    ).requestSubmit();
  });
}

const accounts = [
  { uuid: "cash-1", type: "cash", name: "Kasa", currency: "TRY", active: true },
  {
    uuid: "bank-1",
    type: "bank",
    name: "Banka",
    currency: "TRY",
    active: true,
  },
  {
    uuid: "bank-2",
    type: "bank",
    name: "Eski",
    currency: "TRY",
    active: false,
  },
] as FinanceAccount[];

describe("ServiceIncomeForm (TEC-347)", () => {
  it("cash and card list their account type; cari hides the account and books the customer cari", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    await render(
      createElement(ServiceIncomeForm, {
        service: { is_warranty_reapply: false },
        accounts,
        onCancel: () => {},
        onSubmit,
      }),
    );
    const account = () => container.querySelector("#service-income-account");
    // Cash: only the cash account, preselected.
    expect(account()?.textContent).toContain("Kasa");
    expect(account()?.textContent).not.toContain("Banka");
    expect((account() as HTMLSelectElement).value).toBe("cash-1");
    // Card: the active bank account only.
    await choose(container.querySelector("#service-income-method"), "card");
    expect(account()?.textContent).toContain("Banka");
    expect(account()?.textContent).not.toContain("Eski");

    await choose(container.querySelector("#service-income-method"), "cari");
    expect(account()).toBeNull();
    expect(
      container.querySelector('[data-testid="service-income-cari-hint"]'),
    ).not.toBeNull();
    await type(container.querySelector("#service-income-amount"), "15.000");
    await submit();
    expect(onSubmit).toHaveBeenCalledWith({
      amount: "15000.00",
      payment_method: "cari",
    });
  });

  it("cash sends the account; a bad amount is refused", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    await render(
      createElement(ServiceIncomeForm, {
        service: { is_warranty_reapply: false },
        accounts,
        onCancel: () => {},
        onSubmit,
      }),
    );
    await type(container.querySelector("#service-income-amount"), "abc");
    await submit();
    expect(onSubmit).not.toHaveBeenCalled();
    expect(container.textContent).toContain("services.income.amount_invalid");
    await type(container.querySelector("#service-income-amount"), "250,5");
    await submit();
    expect(onSubmit).toHaveBeenCalledWith({
      amount: "250.50",
      payment_method: "cash",
      account_uuid: "cash-1",
    });
  });

  it("warns on a warranty re-apply service", async () => {
    await render(
      createElement(ServiceIncomeForm, {
        service: { is_warranty_reapply: true },
        accounts,
        onCancel: () => {},
        onSubmit: vi.fn(),
      }),
    );
    expect(
      container.querySelector('[data-testid="service-income-warranty"]')
        ?.textContent,
    ).toContain("services.income.warranty_warning");
  });
});

describe("ServiceProfitCard (TEC-347)", () => {
  const rows = () =>
    Array.from(container.querySelectorAll("[data-testid^='profit-']")).map(
      (el) => el.getAttribute("data-testid"),
    );

  it("shows revenue, cost, profit and margin with pricing.purchase.read", async () => {
    await render(
      createElement(ServiceProfitCard, {
        profit: {
          revenue: "15000.00",
          cost: "4500.00",
          gross_profit: "10500.00",
          margin_pct: "70.00",
        },
      }),
    );
    expect(rows()).toEqual([
      "profit-revenue",
      "profit-cost",
      "profit-gross_profit",
      "profit-margin",
    ]);
    expect(
      container.querySelector('[data-testid="profit-cost"]')?.textContent,
    ).toContain("4500.00");
  });

  it("hides the cost row (and profit / margin) without the cost permission", async () => {
    await render(
      createElement(ServiceProfitCard, {
        profit: {
          revenue: "15000.00",
          cost: null,
          gross_profit: null,
          margin_pct: null,
        },
      }),
    );
    expect(rows()).toEqual(["profit-revenue"]);
    expect(container.querySelector('[data-testid="profit-cost"]')).toBeNull();
    expect(container.textContent).not.toContain("services.profit.cost");
  });
});

describe("resolveServiceIncomeAccess", () => {
  const own = { canWrite: true, orgUuid: "dealer-1" };
  const svc = (over: Record<string, unknown> = {}) => ({
    status: "completed",
    organization: { uuid: "dealer-1" },
    income_amount: null,
    ...over,
  });

  it("records on an own completed service without income", () => {
    expect(resolveServiceIncomeAccess(svc(), own)).toEqual({
      canRecord: true,
      canReverse: false,
    });
    expect(
      resolveServiceIncomeAccess(svc({ income_amount: "100.00" }), own),
    ).toEqual({ canRecord: false, canReverse: true });
  });

  it("never on a draft, another organization's service or without write", () => {
    expect(
      resolveServiceIncomeAccess(svc({ status: "in_progress" }), own).canRecord,
    ).toBe(false);
    expect(
      resolveServiceIncomeAccess(svc({ organization: { uuid: "other" } }), own),
    ).toEqual({ canRecord: false, canReverse: false });
    // A dealer without dealer_accounting resolves canWrite=false.
    expect(
      resolveServiceIncomeAccess(svc(), {
        canWrite: false,
        orgUuid: "dealer-1",
      }),
    ).toEqual({ canRecord: false, canReverse: false });
  });
});
