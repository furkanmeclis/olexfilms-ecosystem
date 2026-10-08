// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type {
  EfficiencySummaryRow,
  PartConsumptionExpectation,
} from "@/features/efficiency/services/efficiency.service";

const api = vi.hoisted(() => ({
  summary: vi.fn(),
  trend: vi.fn(),
  compare: vi.fn(),
  settings: vi.fn(),
  rolls: vi.fn(),
  roll: vi.fn(),
  expectations: vi.fn(),
  createExpectation: vi.fn(),
  updateExpectation: vi.fn(),
  deleteExpectation: vi.fn(),
}));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "dealer" as string | null,
}));
const captured = vi.hoisted(() => ({
  tables: [] as Record<string, unknown>[],
  exports: [] as Record<string, unknown>[],
}));

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
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "en",
    format: {
      number: (v: number) => String(v),
      percent: (v: number) => `${Math.round(v * 1000) / 10}%`,
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => state.grants.has(x)),
    canAny: (ps: string[]) => ps.some((x) => state.grants.has(x)),
    canScope: () => false,
  }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () =>
    state.orgType ? { slug: "acme", uuid: "org-1", type: state.orgType } : null,
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/features/efficiency/services/efficiency.service", async (orig) => ({
  ...(await orig<object>()),
  efficiencyService: api,
}));
vi.mock("@/features/io/components/export-menu", () => ({
  ExportMenu: (props: Record<string, unknown>) => {
    captured.exports.push(props);
    return null;
  },
}));
vi.mock("@/features/io/components/import-wizard", () => ({
  ImportWizard: () => null,
}));
vi.mock("@/components/charts", () => ({
  AppChart: () => createElement("div", { "data-testid": "chart" }),
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: ({ children }: { children?: ReactNode }) => children,
  EntityRowActions: () => null,
  // Renders every row with its column cells and row class.
  EntityTable: (props: Record<string, unknown>) => {
    captured.tables.push(props);
    const rows = (props.data as Record<string, unknown>[]) ?? [];
    const columns = props.columns as {
      id?: string;
      accessorKey?: string;
      cell?: unknown;
    }[];
    const rowClass = props.getRowClassName as
      ((row: unknown) => string | undefined) | undefined;
    return createElement(
      "div",
      { "data-testid": "entity-table" },
      props.toolbarExtra as ReactNode,
      rows.map((row, index) =>
        createElement(
          "div",
          {
            key: index,
            "data-testid": "entity-row",
            className: rowClass?.(row) ?? "",
          },
          columns.map((column) =>
            createElement(
              "span",
              {
                key: column.id ?? column.accessorKey,
                "data-col": column.id ?? column.accessorKey,
              },
              typeof column.cell === "function"
                ? (column.cell as (ctx: unknown) => ReactNode)({
                    row: { original: row },
                  })
                : String(row[column.accessorKey ?? ""] ?? ""),
            ),
          ),
        ),
      ),
    );
  },
}));

import { tenantNav, platformNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import { EfficiencyPage } from "./efficiency-page";
import { PartConsumptionExpectationsPage } from "./part-consumption-expectations-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

function summaryRow(
  partial: Partial<EfficiencySummaryRow>,
): EfficiencySummaryRow {
  return {
    dimension_key: partial.dimension_key ?? "p1",
    dimension_label: partial.dimension_label ?? "Film A",
    services: partial.services ?? 3,
    meters: partial.meters ?? "12.00",
    expected_meters:
      partial.expected_meters === undefined ? "10.00" : partial.expected_meters,
    waste_ratio:
      partial.waste_ratio === undefined ? "0.20" : partial.waste_ratio,
  };
}

function expectation(
  partial: Partial<PartConsumptionExpectation>,
): PartConsumptionExpectation {
  return {
    uuid: partial.uuid ?? "e1",
    product_uuid: partial.product_uuid ?? "p1",
    category_uuid: partial.category_uuid ?? null,
    product_name: partial.product_name ?? "Film A",
    category_name: partial.category_name ?? null,
    body_type: partial.body_type ?? "sedan",
    part_key: partial.part_key ?? "body_kaput",
    expected_meters: partial.expected_meters ?? "2.00",
    source: partial.source ?? "network",
    sample_size: partial.sample_size ?? 24,
  };
}

let container: HTMLDivElement;
let root: Root | null;

beforeEach(() => {
  window.localStorage?.clear();
  captured.tables = [];
  captured.exports = [];
  state.grants = new Set([
    Permission.EfficiencyRead,
    Permission.EfficiencyExpectationsManage,
  ]);
  state.orgType = "dealer";
  for (const fn of Object.values(api)) fn.mockReset();
  api.settings.mockResolvedValue({ warning_waste_ratio: "0.15" });
  api.trend.mockResolvedValue([]);
  api.compare.mockResolvedValue([]);
  api.rolls.mockResolvedValue({ items: [], total: 0, limit: 20, offset: 0 });
  api.summary.mockResolvedValue({ items: [], total: 0, limit: 20, offset: 0 });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root?.unmount());
  root = null;
  container.remove();
});

async function render(node: ReactNode) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  await act(async () => {
    root?.render(createElement(QueryClientProvider, { client: qc }, node));
  });
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function rowsOf(testid = "entity-row") {
  return Array.from(container.querySelectorAll(`[data-testid="${testid}"]`));
}

function cell(row: Element, col: string) {
  return row.querySelector(`[data-col="${col}"]`)?.textContent ?? "";
}

describe("EfficiencyPage", () => {
  it('shows "—" in the waste cell when nothing is expected', async () => {
    api.summary.mockImplementation((_dim: string, params: { limit: number }) =>
      Promise.resolve({
        items:
          params.limit === 1
            ? []
            : [
                summaryRow({
                  dimension_key: "p1",
                  expected_meters: null,
                  waste_ratio: null,
                }),
                summaryRow({ dimension_key: "p2", waste_ratio: "0.10" }),
              ],
        total: 2,
        limit: 20,
        offset: 0,
      }),
    );
    await render(createElement(EfficiencyPage, { slug: "acme" }));

    const rows = rowsOf();
    expect(rows).toHaveLength(2);
    expect(cell(rows[0], "waste_ratio")).toBe("—");
    expect(cell(rows[0], "expected_meters")).toBe("—");
    expect(cell(rows[1], "waste_ratio")).toBe("10%");
  });

  it("highlights rows above the warning threshold", async () => {
    api.settings.mockResolvedValue({ warning_waste_ratio: "0.25" });
    api.summary.mockImplementation((_dim: string, params: { limit: number }) =>
      Promise.resolve({
        items:
          params.limit === 1
            ? []
            : [
                summaryRow({ dimension_key: "high", waste_ratio: "0.30" }),
                summaryRow({ dimension_key: "low", waste_ratio: "0.20" }),
                summaryRow({ dimension_key: "none", waste_ratio: null }),
              ],
        total: 3,
        limit: 20,
        offset: 0,
      }),
    );
    await render(createElement(EfficiencyPage, { slug: "acme" }));

    const rows = rowsOf();
    expect(rows).toHaveLength(3);
    expect(rows[0].className).toContain("bg-destructive/5");
    expect(rows[1].className).not.toContain("bg-destructive/5");
    expect(rows[2].className).not.toContain("bg-destructive/5");
  });

  it("has no dealer tab for a dealer user", async () => {
    await render(createElement(EfficiencyPage, { slug: "acme" }));
    const tab = (id: string) =>
      container.querySelector(`[data-testid="efficiency-tab-${id}"]`);
    expect(tab("dealer")).toBeNull();
    expect(tab("staff")).toBeNull();
    expect(tab("product")).not.toBeNull();
    expect(tab("rolls")).not.toBeNull();
    expect(api.summary).not.toHaveBeenCalledWith("dealer", expect.anything());
  });

  it("opens on the dealer tab for a distributor", async () => {
    state.orgType = "distributor";
    await render(createElement(EfficiencyPage, { slug: "acme" }));
    expect(
      container.querySelector('[data-testid="efficiency-tab-dealer"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[data-testid="efficiency-tab-staff"]'),
    ).not.toBeNull();
    expect(api.summary).toHaveBeenCalledWith(
      "dealer",
      expect.objectContaining({ sort: "-waste_ratio" }),
    );
  });

  it("sends the summary export with the period and dimension", async () => {
    await render(createElement(EfficiencyPage, { slug: "acme" }));
    const exp = captured.exports.at(-1);
    expect(exp?.exportPath).toBe("/v1/efficiency/summary/export");
    expect(exp?.query).toMatchObject({ dimension: "product" });
    expect((exp?.query as Record<string, string>).period_from).toMatch(
      /^\d{4}-\d{2}-\d{2}$/,
    );
  });

  it("is forbidden without efficiency.read", async () => {
    state.grants = new Set();
    await render(createElement(EfficiencyPage, { slug: "acme" }));
    expect(container.textContent).toContain("efficiency.page.forbidden");
    expect(api.summary).not.toHaveBeenCalled();
  });
});

describe("PartConsumptionExpectationsPage", () => {
  it("turns an edited network row into a manual one", async () => {
    const row = expectation({ uuid: "e1", source: "network" });
    api.expectations.mockResolvedValue({
      items: [row],
      total: 1,
      limit: 50,
      offset: 0,
    });
    let resolveUpdate: (value: PartConsumptionExpectation) => void = () => {};
    api.updateExpectation.mockImplementation(
      () =>
        new Promise<PartConsumptionExpectation>((resolve) => {
          resolveUpdate = resolve;
        }),
    );
    await render(createElement(PartConsumptionExpectationsPage));
    expect(cell(rowsOf()[0], "source")).toBe(
      "efficiency.expectations.sources.network",
    );

    const table = captured.tables.at(-1) as {
      onCellEdit: (p: Record<string, unknown>) => void;
      features: { inlineEdit?: boolean };
    };
    expect(table.features.inlineEdit).toBe(true);
    await act(async () => {
      table.onCellEdit({
        rowId: "e1",
        columnId: "expected_meters",
        value: "2.5",
        row,
      });
      // Query cache notifications are batched on a timer.
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(api.updateExpectation).toHaveBeenCalledWith(
      "e1",
      expect.objectContaining({
        expected_meters: "2.50",
        part_key: "body_kaput",
        product_uuid: "p1",
      }),
    );
    // Before the API answers, the row already reads "manual".
    expect(cell(rowsOf()[0], "source")).toBe(
      "efficiency.expectations.sources.manual",
    );
    expect(cell(rowsOf()[0], "expected_meters")).toBe("2.5");
    await act(async () => {
      resolveUpdate({ ...row, expected_meters: "2.50", source: "manual" });
    });
  });

  it("rejects a non-positive meter edit without calling the API", async () => {
    const row = expectation({});
    api.expectations.mockResolvedValue({
      items: [row],
      total: 1,
      limit: 50,
      offset: 0,
    });
    await render(createElement(PartConsumptionExpectationsPage));
    const table = captured.tables.at(-1) as {
      onCellEdit: (p: Record<string, unknown>) => void;
    };
    await act(async () => {
      table.onCellEdit({
        rowId: "e1",
        columnId: "expected_meters",
        value: "0",
        row,
      });
    });
    expect(api.updateExpectation).not.toHaveBeenCalled();
  });
});

describe("efficiency navigation", () => {
  function visible(
    nav: ReturnType<typeof tenantNav>,
    granted: string[],
    features: string[],
    orgType = "dealer",
  ) {
    const access = {
      can: (p: string | string[]) =>
        (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
      canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
      org: { type: orgType, role: "owner", features },
    };
    return nav.groups.flatMap((group) =>
      visibleNavItems(group, access).map((item) => item.id),
    );
  }

  it("shows the menu only with the efficiency module and permission", () => {
    const nav = tenantNav("acme");
    expect(visible(nav, [Permission.EfficiencyRead], ["efficiency"])).toContain(
      "efficiency",
    );
    expect(visible(nav, [Permission.EfficiencyRead], [])).not.toContain(
      "efficiency",
    );
    expect(visible(nav, [], ["efficiency"])).not.toContain("efficiency");
  });

  it("lists expected consumption on the platform with the manage permission", () => {
    expect(
      visible(platformNav, [Permission.EfficiencyExpectationsManage], []),
    ).toContain("part-consumption-expectations");
    expect(visible(platformNav, [], [])).not.toContain(
      "part-consumption-expectations",
    );
  });
});
