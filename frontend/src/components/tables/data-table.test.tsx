// @vitest-environment jsdom
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "en",
    format: { dateTime: (v: string) => v, date: (v: string) => v },
  }),
}));
vi.mock("@/hooks/use-mobile", () => ({
  useIsMobile: () => false,
  useIsXl: () => true,
}));

import { createColumn } from "./create-column";
import { DataTable } from "./data-table";
import type { DataTableProps } from "./types";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

type Row = { id: string; name: string; status: string };

const columns = [
  createColumn<Row>({ accessorKey: "name", labelKey: "users.columns.name" }),
  createColumn<Row>({
    accessorKey: "status",
    labelKey: "users.columns.status",
    filterVariant: "faceted",
    filterOptions: [
      { value: "active", label: "Active" },
      { value: "disabled", label: "Disabled" },
    ],
  }),
] as ColumnDef<Row, unknown>[];

const page2: Row[] = Array.from({ length: 20 }, (_, i) => ({
  id: `r${i + 21}`,
  name: `User ${i + 21}`,
  status: "active",
}));

let container: HTMLDivElement;
let root: Root;

beforeAll(() => {
  class RO {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  (globalThis as { ResizeObserver?: unknown }).ResizeObserver = RO;
  Element.prototype.scrollIntoView = () => {};
});

beforeEach(() => {
  window.localStorage.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

function render(props: Partial<DataTableProps<Row>>) {
  act(() => {
    root.render(
      createElement(DataTable<Row>, {
        columns,
        data: page2,
        ...props,
      } as DataTableProps<Row>),
    );
  });
}

const serverProps: Partial<DataTableProps<Row>> = {
  manual: { pagination: true, sorting: true, filtering: true },
  state: {
    pagination: { pageIndex: 1, pageSize: 20 },
    onPaginationChange: () => {},
  },
};

describe("DataTable server mode", () => {
  it("rowCount drives the x–y of total label and page count", () => {
    render({ ...serverProps, rowCount: 95 });
    const text = document.body.textContent ?? "";
    expect(text).toContain('table.showing {"from":21,"to":40,"total":95}');
    expect(text).toContain("2/5");
  });

  it("without rowCount falls back to the loaded rows (client mode)", () => {
    render({
      data: page2.slice(0, 5),
      initialState: { pagination: { pageIndex: 0, pageSize: 10 } },
    });
    expect(document.body.textContent).toContain(
      'table.showing {"from":1,"to":5,"total":5}',
    );
  });

  it("hides faceted counts in server mode, shows them client-side", async () => {
    const countBadges = async () => {
      const trigger = Array.from(document.querySelectorAll("button")).find(
        (b) => b.textContent?.includes("users.columns.status"),
      );
      await act(async () => {
        trigger?.click();
      });
      return Array.from(document.querySelectorAll("span.font-mono")).map(
        (el) => el.textContent,
      );
    };

    render({ ...serverProps, rowCount: 95 });
    expect(await countBadges()).toEqual([]);

    act(() => root.unmount());
    document.body.innerHTML = "";
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    render({ data: page2.slice(0, 3) });
    expect(await countBadges()).toEqual(["3"]);
  });

  it("persistKey restores layout but not sort/filters in server mode", () => {
    window.localStorage.setItem(
      "t-v1",
      JSON.stringify({
        columnVisibility: { status: false },
        density: "compact",
        sorting: [{ id: "name", desc: true }],
        columnFilters: [{ id: "status", value: ["x"] }],
      }),
    );
    render({ ...serverProps, rowCount: 95, features: { persistKey: "t-v1" } });

    const headers = Array.from(document.querySelectorAll("th")).map(
      (th) => th.textContent,
    );
    expect(headers.some((h) => h?.includes("users.columns.status"))).toBe(
      false,
    );
    expect(headers.some((h) => h?.includes("users.columns.name"))).toBe(true);

    const saved = JSON.parse(window.localStorage.getItem("t-v1") ?? "{}");
    expect(saved).toMatchObject({
      columnVisibility: { status: false },
      density: "compact",
      pagination: { pageIndex: 0, pageSize: 20 },
    });
    expect(saved.sorting).toBeUndefined();
    expect(saved.columnFilters).toBeUndefined();
  });

  it("multiSort is off by default", () => {
    let captured: boolean | undefined;
    render({
      toolbar: (table) => {
        captured = table.options.enableMultiSort;
        return null;
      },
    });
    expect(captured).toBe(false);
  });
});
