// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type { StoredExchangeRate } from "@/features/exchange-rates/services/rates.service";

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<StoredExchangeRate>>,
  rates: {
    day: vi.fn(),
    currencies: vi.fn(),
    setOverride: vi.fn(),
    clearOverride: vi.fn(),
    fetchNow: vi.fn(),
  },
  toastError: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key, locale: "en" }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: captured.toastError },
}));
vi.mock("@/components/forms", () => ({
  AppForm: () => null,
  AppCombobox: () => null,
  AppDatePicker: () => null,
  AppInput: () => null,
  FormSection: () => null,
}));
vi.mock("@/components/ui/date-picker", () => ({ DatePicker: () => null }));
vi.mock("@/features/exchange-rates/services/rates.service", () => ({
  ratesService: captured.rates,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (props: Partial<DataTableProps<StoredExchangeRate>>) => {
    captured.table = props;
    return null;
  },
}));

import { ExchangeRatesPage } from "./exchange-rates-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const tcmb: StoredExchangeRate = {
  rate_date: "2026-10-06",
  base: "EUR",
  quote: "TRY",
  rate: "48.75",
  source: "tcmb",
  effective: false,
  fetched_at: "2026-10-06T08:00:00Z",
};
const manual: StoredExchangeRate = {
  ...tcmb,
  rate: "49.10",
  source: "manual",
  note: "fixed",
  effective: true,
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  for (const fn of Object.values(captured.rates)) fn.mockReset();
  captured.toastError.mockReset();
  captured.rates.day.mockResolvedValue({
    date: "2026-10-06",
    items: [manual, tcmb],
  });
  captured.rates.currencies.mockResolvedValue({ items: [] });
  captured.rates.setOverride.mockResolvedValue({});
  captured.rates.clearOverride.mockResolvedValue({ deleted: true });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function flush() {
  for (let i = 0; i < 4; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(ExchangeRatesPage),
      ),
    );
  });
  await flush();
}

const column = (id: string) =>
  (captured.table?.columns as ColumnDef<StoredExchangeRate, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("ExchangeRatesPage", () => {
  it("is client-side with a source facet and inline rate edit", async () => {
    await render();
    expect(captured.table?.manual).toMatchObject({
      sorting: false,
      filtering: false,
      pagination: false,
    });
    expect(captured.table?.features).toMatchObject({
      persistKey: "platform-exchange-rates-v1",
      inlineEdit: true,
    });
    expect(column("source")?.meta?.filterOptions?.map((o) => o.value)).toEqual([
      "manual",
      "tcmb",
      "ecb",
    ]);
    expect(column("rate")?.meta?.editVariant).toBe("text");
  });

  it("saves an inline rate edit as a manual override", async () => {
    await render();
    await act(async () => {
      captured.table?.onCellEdit?.({
        rowId: "EUR-TRY-tcmb",
        columnId: "rate",
        value: " 48.90 ",
        row: tcmb,
      });
    });
    await flush();
    expect(captured.rates.setOverride).toHaveBeenCalledWith({
      date: "2026-10-06",
      base: "EUR",
      quote: "TRY",
      rate: "48.90",
      note: undefined,
    });
  });

  it("rejects an invalid inline rate", async () => {
    await render();
    await act(async () => {
      captured.table?.onCellEdit?.({
        rowId: "EUR-TRY-tcmb",
        columnId: "rate",
        value: "abc",
        row: tcmb,
      });
    });
    await flush();
    expect(captured.rates.setOverride).not.toHaveBeenCalled();
    expect(captured.toastError).toHaveBeenCalledWith("rates.validation.rate");
  });

  it("clears an override from the row action", async () => {
    await render();
    const cell = column("actions")?.cell as (ctx: unknown) => ReactNode;
    const host = document.createElement("div");
    document.body.appendChild(host);
    const cellRoot = createRoot(host);
    await act(async () => {
      cellRoot.render(cell({ row: { original: manual } }) as never);
    });
    await act(async () => {
      host.querySelector<HTMLElement>("button")?.click();
    });
    await flush();
    expect(captured.rates.clearOverride).toHaveBeenCalledWith(
      "2026-10-06",
      "EUR",
      "TRY",
    );
    act(() => cellRoot.unmount());
    host.remove();
  });
});
