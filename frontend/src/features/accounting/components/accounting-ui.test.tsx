// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number | null) => (v === null ? "—" : String(v)),
      date: (v: string) => `d:${v}`,
      dateTime: (v: string) => `dt:${v}`,
      currency: (v: number | null, c: string) =>
        v === null ? "—" : `${v.toFixed(2)} ${c}`,
    },
  }),
}));

// The shared DatePicker is a Radix popover; a plain input keeps the test on
// the filter wiring (value in, yyyy-MM-dd out).
vi.mock("@/components/ui/date-picker", async () => {
  const { createElement: h } = await import("react");
  return {
    DatePicker: ({
      id,
      value,
      onChange,
    }: {
      id?: string;
      value?: string;
      onChange?: (v: string) => void;
    }) =>
      h("input", {
        id,
        value: value ?? "",
        onChange: (e: { target: { value: string } }) =>
          onChange?.(e.target.value),
      }),
  };
});

import { AccountForm } from "./account-form-dialog";
import { AccountOpeningForm } from "./account-opening-dialog";
import { EntryFilters } from "./entry-filters";
import { SettlementForm } from "./settlement-dialog";
import { EntryAmount, EntryStatus } from "./shared";
import { EMPTY_ENTRY_FILTERS } from "@/features/accounting/lib/form";
import type {
  CariAccount,
  FinanceAccount,
  FinanceEntry,
} from "@/features/accounting/services/accounting.service";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
// Radix Switch measures itself; jsdom has no ResizeObserver.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

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

async function render(node: ReturnType<typeof createElement>) {
  await act(async () => {
    root.render(node);
  });
}

async function flush() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
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

async function submit(form: Element | null) {
  if (!(form instanceof HTMLFormElement)) throw new Error("form not found");
  await act(async () => {
    form.requestSubmit();
  });
  await flush();
}

const $ = (sel: string) => container.querySelector(sel);

const account = (over: Partial<FinanceAccount> = {}): FinanceAccount => ({
  uuid: "a1",
  type: "bank",
  name: "Ziraat",
  currency: "TRY",
  iban: null,
  active: true,
  balance: "0.00",
  entry_count: 0,
  last_entry_at: null,
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
  ...over,
});

const cari: CariAccount = {
  uuid: "c1",
  counterparty: {
    type: "organization",
    uuid: "o2",
    name: "Bayi A",
    org_type: "dealer",
  },
  currency: "TRY",
  active: true,
  balance: "1500.00",
  entry_count: 3,
  last_entry_at: null,
  created_at: "2026-10-01T00:00:00Z",
};

const entry = (over: Partial<FinanceEntry> = {}): FinanceEntry => ({
  uuid: "e1",
  direction: "collection",
  category: "collection",
  category_label_key: "accounting.category.collection",
  account: { uuid: "a1", name: "Ziraat" },
  cari_uuid: "c1",
  counterparty_organization: { uuid: "o2", name: "Bayi A" },
  orig_currency: "TRY",
  orig_amount: "100.00",
  currency: "TRY",
  amount: "100.00",
  rate: "1.00000000",
  rate_date: "2026-10-01",
  source_type: "manual",
  source_uuid: "s1",
  revision: 1,
  reversal_of_uuid: null,
  reversed_by_uuid: null,
  voided: false,
  description: null,
  created_at: "2026-10-01T10:00:00Z",
  ...over,
});

describe("SettlementForm validation", () => {
  it("blocks an empty submit and shows field errors", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    await render(
      createElement(SettlementForm, {
        kind: "collection",
        accounts: [account(), account({ uuid: "a2", name: "Kasa" })],
        cari: [cari],
        currencies: ["TRY", "EUR"],
        idempotencyKey: "k1",
        onCancel: vi.fn(),
        onSubmit,
      }),
    );
    await submit($("[data-testid=settlement-form]"));
    expect(onSubmit).not.toHaveBeenCalled();
    const text = container.textContent ?? "";
    expect(text).toContain("accounting.validation.account");
    expect(text).toContain("accounting.validation.cari");
    expect(text).toContain("accounting.validation.required");

    await type($("#settlement-amount"), "-3");
    await submit($("[data-testid=settlement-form]"));
    expect(container.textContent).toContain("accounting.validation.amount");
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("posts the normalized body with the idempotency key", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    await render(
      createElement(SettlementForm, {
        kind: "payment",
        accounts: [account(), account({ uuid: "a2", name: "Kasa" })],
        cari: [cari],
        currencies: ["TRY", "EUR"],
        idempotencyKey: "k1",
        onCancel: vi.fn(),
        onSubmit,
      }),
    );
    await choose($("#settlement-account"), "a2");
    await choose($("#settlement-cari"), "c1");
    await type($("#settlement-amount"), "1.250,5");
    await submit($("[data-testid=settlement-form]"));
    expect(onSubmit).toHaveBeenCalledWith({
      account_uuid: "a2",
      cari_uuid: "c1",
      amount: "1250.50",
      currency: "TRY",
      idempotency_key: "k1",
    });
  });

  it("locks a fixed cari and hints the conversion of another currency", async () => {
    await render(
      createElement(SettlementForm, {
        kind: "collection",
        accounts: [account()],
        cari: [],
        currencies: ["TRY", "EUR"],
        fixedCari: cari,
        idempotencyKey: "k1",
        onCancel: vi.fn(),
        onSubmit: vi.fn(),
      }),
    );
    const select = $("#settlement-cari") as HTMLSelectElement;
    expect(select.disabled).toBe(true);
    expect(select.value).toBe("c1");
    // A single active account is preselected with its currency.
    expect(($("#settlement-account") as HTMLSelectElement).value).toBe("a1");
    expect($("[data-testid=settlement-fx-hint]")).toBeNull();
    await choose($("#settlement-currency"), "EUR");
    expect($("[data-testid=settlement-fx-hint]")).not.toBeNull();
  });

  it("does not offer an inactive account", async () => {
    await render(
      createElement(SettlementForm, {
        kind: "collection",
        accounts: [account({ active: false }), account({ uuid: "a2" })],
        cari: [cari],
        currencies: [],
        idempotencyKey: "k1",
        onCancel: vi.fn(),
        onSubmit: vi.fn(),
      }),
    );
    const values = Array.from(
      ($("#settlement-account") as HTMLSelectElement).options,
    ).map((o) => o.value);
    expect(values).toEqual(["", "a2"]);
  });
});

describe("AccountForm validation", () => {
  it("requires a name and a valid IBAN for a bank account", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    await render(
      createElement(AccountForm, {
        account: null,
        onCancel: vi.fn(),
        onSubmit,
      }),
    );
    expect($("#account-iban")).toBeNull();
    await submit($("[data-testid=account-form]"));
    expect(container.textContent).toContain("accounting.validation.required");
    expect(onSubmit).not.toHaveBeenCalled();

    await choose($("#account-type"), "bank");
    await type($("#account-name"), "Ziraat");
    await type($("#account-iban"), "TR00 1111");
    await submit($("[data-testid=account-form]"));
    expect(container.textContent).toContain("accounting.validation.iban");
    expect(onSubmit).not.toHaveBeenCalled();

    await type($("#account-iban"), "TR33 0006 1005 1978 6457 8413 26");
    await submit($("[data-testid=account-form]"));
    expect(onSubmit).toHaveBeenCalledWith({
      type: "bank",
      name: "Ziraat",
      iban: "TR33 0006 1005 1978 6457 8413 26",
      active: true,
    });
  });

  it("keeps the type of an existing account", async () => {
    await render(
      createElement(AccountForm, {
        account: account({ type: "cash", name: "Kasa" }),
        onCancel: vi.fn(),
        onSubmit: vi.fn(),
      }),
    );
    expect(($("#account-type") as HTMLSelectElement).disabled).toBe(true);
    expect(($("#account-name") as HTMLInputElement).value).toBe("Kasa");
  });
});

describe("AccountOpeningForm (TEC-198)", () => {
  it("validates, then posts the normalized amount and date", async () => {
    const onSubmit = vi.fn().mockResolvedValue({});
    await render(
      createElement(AccountOpeningForm, {
        account: account({ type: "cash" }),
        onCancel: () => {},
        onSubmit,
      }),
    );
    await submit($("[data-testid=account-opening-form]"));
    expect(onSubmit).not.toHaveBeenCalled();
    expect(container.textContent).toContain("accounting.validation.amount");
    expect(container.textContent).toContain("accounting.validation.date");

    await type($("#opening-amount"), "1.500,5");
    await type($("#opening-date"), "2021-03-01");
    await submit($("[data-testid=account-opening-form]"));
    expect(onSubmit).toHaveBeenCalledWith({
      amount: "1500.50",
      opening_date: "2021-03-01",
      description: undefined,
    });
  });
});

describe("EntryFilters", () => {
  it("reports each filter and clears them", async () => {
    const onChange = vi.fn();
    await render(
      createElement(EntryFilters, {
        value: EMPTY_ENTRY_FILTERS,
        cariOptions: [{ value: "c1", label: "Bayi A" }],
        onChange,
      }),
    );
    const clear = $("[data-testid=entry-filters-clear]") as HTMLButtonElement;
    expect(clear.disabled).toBe(true);

    await choose($("#entry-filter-cari"), "c1");
    expect(onChange).toHaveBeenLastCalledWith({
      ...EMPTY_ENTRY_FILTERS,
      cari_uuid: "c1",
    });
    await choose($("#entry-filter-source"), "order");
    expect(onChange).toHaveBeenLastCalledWith({
      ...EMPTY_ENTRY_FILTERS,
      source_type: "order",
    });
    await choose($("#entry-filter-direction"), "payment");
    expect(onChange).toHaveBeenLastCalledWith({
      ...EMPTY_ENTRY_FILTERS,
      direction: "payment",
    });
    await type($("#entry-filter-from"), "2026-10-01");
    expect(onChange).toHaveBeenLastCalledWith({
      ...EMPTY_ENTRY_FILTERS,
      date_from: "2026-10-01",
    });
  });

  it("enables clear when a filter is set", async () => {
    const onChange = vi.fn();
    await render(
      createElement(EntryFilters, {
        value: { ...EMPTY_ENTRY_FILTERS, source_type: "manual" },
        cariOptions: [],
        onChange,
      }),
    );
    const clear = $("[data-testid=entry-filters-clear]") as HTMLButtonElement;
    expect(clear.disabled).toBe(false);
    await act(async () => clear.click());
    expect(onChange).toHaveBeenCalledWith(EMPTY_ENTRY_FILTERS);
  });
});

describe("entry markers", () => {
  it("marks reversal and reversed rows", async () => {
    await render(
      createElement(
        "div",
        null,
        createElement(EntryStatus, {
          entry: entry({ reversal_of_uuid: "e0", amount: "-100.00" }),
        }),
        createElement(EntryStatus, { entry: entry({ voided: true }) }),
        createElement(EntryStatus, { entry: entry() }),
      ),
    );
    expect(
      container.querySelectorAll("[data-testid=entry-reversal]"),
    ).toHaveLength(1);
    expect(
      container.querySelectorAll("[data-testid=entry-voided]"),
    ).toHaveLength(1);
  });

  it("shows the original amount and frozen rate of a foreign entry", async () => {
    await render(
      createElement(EntryAmount, {
        entry: entry({
          orig_currency: "EUR",
          orig_amount: "10.00",
          amount: "380.00",
          rate: "38.00000000",
        }),
      }),
    );
    expect(container.textContent).toContain("380.00 TRY");
    expect(container.textContent).toContain("10.00 EUR");
    expect($("[data-testid=entry-rate]")?.textContent).toContain('"rate":"38"');
  });
});
