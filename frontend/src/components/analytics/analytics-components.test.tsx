// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createFormatter } from "@/lib/i18n/format";

const formatter = createFormatter({
  locale: "en",
  timeZone: "Europe/Istanbul",
});
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "en",
  }),
  useFormatter: () => formatter,
}));
const push = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, replace: vi.fn() }),
}));

import { withSeriesColors } from "@/components/charts/app-chart";

import { DataGrid } from "./data-grid";
import { KpiTile } from "./kpi-tile";
import type { GridColumn, GridRow } from "./types";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let host: HTMLDivElement;
let root: Root;
beforeEach(() => {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  window.localStorage.clear();
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

const render = (el: React.ReactElement) => act(() => root.render(el));

describe("KpiTile", () => {
  it("shows value, delta with icon + reading, previous and link", () => {
    render(
      createElement(KpiTile, {
        label: "Revenue",
        value: 1200,
        previous: 1000,
        good: "down",
        format: { kind: "money", currency: "EUR" },
        spark: [1, 2, 3],
        href: "/t/olex/orders",
      }),
    );
    const tile = host.querySelector('[data-slot="kpi-tile"]')!;
    expect(tile.tagName).toBe("A");
    expect(tile.textContent).toContain("€1,200.00");
    const badge = host.querySelector('[data-slot="delta-badge"]')!;
    expect(badge.getAttribute("data-tone")).toBe("bad");
    expect(badge.textContent).toContain("+20%");
    expect(badge.textContent).toContain("chart.kpi.worse");
    expect(badge.querySelector("svg")).not.toBeNull();
    expect(host.querySelector('[data-slot="sparkline"]')).not.toBeNull();
  });
  it("has no badge without comparison data", () => {
    render(createElement(KpiTile, { label: "Count", value: 3 }));
    expect(host.querySelector('[data-slot="delta-badge"]')).toBeNull();
  });
});

const columns: GridColumn[] = [
  { key: "name", label: "Name", type: "text" },
  { key: "city", label: "City", type: "text" },
  {
    key: "amount",
    label: "Amount",
    type: "money",
    aggregate: "sum",
    currency: "EUR",
  },
];
const rows: GridRow[] = [
  {
    name: "B",
    city: "Sofia",
    amount: 20,
    _link: { kind: "order", uuid: "o2" },
  },
  {
    name: "A",
    city: "Sofia",
    amount: 10,
    _link: { kind: "order", uuid: "o1" },
  },
  { name: "C", city: "Baku", amount: 5 },
];

describe("DataGrid", () => {
  it("sorts, totals, links rows and searches", async () => {
    render(
      createElement(DataGrid, {
        columns,
        rows,
        slug: "olex",
        defaultSorting: [{ id: "amount", desc: false }],
      }),
    );
    const leafNames = () =>
      [...host.querySelectorAll('tr[data-row="leaf"] td:first-child')].map(
        (td) => td.textContent,
      );
    expect(leafNames()).toEqual(["C", "A", "B"]);
    const total = host.querySelector('tr[data-row="totals"]')!;
    expect(total.textContent).toContain("€35.00");
    expect(host.querySelectorAll('tr[data-row="leaf"] a')).toHaveLength(2);
    expect(
      host.querySelector('tr[data-row="leaf"]')!.querySelector("a"),
    ).toBeNull();
    expect(
      (
        host.querySelectorAll('tr[data-row="leaf"] a')[0] as HTMLAnchorElement
      ).getAttribute("href"),
    ).toBe("/t/olex/orders/o1");

    const search = host.querySelector(
      'input[type="search"]',
    ) as HTMLInputElement;
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value",
      )!.set!;
      setter.call(search, "baku");
      search.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect(leafNames()).toEqual(["C"]);
    expect(host.querySelector('tr[data-row="totals"]')!.textContent).toContain(
      "€5.00",
    );
  });

  it("groups rows with subtotals", () => {
    render(createElement(DataGrid, { columns, rows, defaultGroupBy: "city" }));
    const groups = [...host.querySelectorAll('tr[data-row="group"]')];
    expect(groups).toHaveLength(2);
    const sofia = groups.find((g) => g.textContent?.includes("Sofia"))!;
    expect(sofia.textContent).toContain("€30.00");
  });

  it("keeps layout preferences per user key", () => {
    render(
      createElement(DataGrid, {
        columns,
        rows,
        persistKey: "test.grid",
        defaultSorting: [{ id: "name", desc: true }],
      }),
    );
    const header = [...host.querySelectorAll("thead button")].find((b) =>
      b.textContent?.includes("City"),
    ) as HTMLButtonElement;
    act(() => header.click());
    const key = Object.keys(window.localStorage).find((k) =>
      k.includes("test.grid"),
    );
    expect(key).toContain("anon");
    expect(JSON.parse(window.localStorage.getItem(key!)!).sorting[0].id).toBe(
      "city",
    );
  });

  it("virtualises large data", () => {
    const many = Array.from({ length: 2000 }, (_, i) => ({
      name: `R${i}`,
      city: "X",
      amount: i,
    }));
    render(createElement(DataGrid, { columns, rows: many, height: 360 }));
    const rendered = host.querySelectorAll('tr[data-row="leaf"]').length;
    expect(rendered).toBeGreaterThan(0);
    expect(rendered).toBeLessThan(60);
  });
});

describe("AppChart colour fix", () => {
  it("assigns palette colours to series without one", () => {
    const out = withSeriesColors(
      { a: { label: "A" }, b: { label: "B", color: "red" } },
      ["a", "b", "c"],
    );
    expect(out.a.color).toBe("var(--viz-cat-1)");
    expect(out.b.color).toBe("red");
    expect(out.c.color).toBe("var(--viz-cat-3)");
  });
});
