// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  dashboard: vi.fn(),
  ranking: vi.fn(),
  benchmark: vi.fn(),
  regionMap: vi.fn(),
  dealerMap: vi.fn(),
  createTarget: vi.fn(),
  reportCatalog: vi.fn(),
  report: vi.fn(),
  reportLayout: vi.fn(),
  saveReportLayout: vi.fn(),
}));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "dealer" as string | null,
  features: ["performance"] as string[] | null,
}));
const captured = vi.hoisted(() => ({
  tables: [] as Record<string, unknown>[],
  maps: [] as Record<string, unknown>[],
}));

vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: unknown }) =>
    createElement("a", { href }, children as never),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "en",
    format: {
      number: (v: number) => String(v),
      percent: (v: number) => `${Math.round(v * 1000) / 10}%`,
      currency: (v: number, c: string) => `${v} ${c}`,
      dateParts: (v: string) => v.slice(0, 7),
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
vi.mock("@/features/modules/hooks/use-features", () => ({
  useEnabledFeatures: () => state.features,
}));
vi.mock("@/features/geo/hooks/use-geo", () => ({
  useProvinces: () => ({ data: [{ id: 34, name: "İstanbul" }] }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock(
  "@/features/performance/services/performance.service",
  async (orig) => ({
    ...(await orig<object>()),
    performanceService: api,
  }),
);
vi.mock("@/components/charts", () => ({
  AppChart: () => createElement("div", { "data-testid": "chart" }),
}));
vi.mock("@/components/common/leaflet-map", () => ({
  LeafletMap: (props: Record<string, unknown>) => {
    captured.maps.push(props);
    return createElement("div", { "data-testid": "leaflet" });
  },
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: ({ children }: { children?: ReactNode }) => children,
  EntityRowActions: () => null,
  EntityTable: (props: Record<string, unknown>) => {
    captured.tables.push(props);
    return createElement(
      "div",
      { "data-testid": "entity-table" },
      props.toolbarExtra as ReactNode,
    );
  },
}));

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

import { DashboardPanel } from "./dashboard-panel";
import { PerformancePage, performanceTabs } from "./performance-page";
import { RegionMap } from "./region-map";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root | null;

const metricDelta = (value: string) => ({
  current: { value },
  previous: { value: "1" },
  delta_pct: "10.00",
});

beforeEach(() => {
  window.localStorage?.clear();
  captured.tables = [];
  captured.maps = [];
  state.grants = new Set([Permission.PerformanceRead]);
  state.orgType = "dealer";
  state.features = ["performance"];
  for (const fn of Object.values(api)) fn.mockReset();
  api.dashboard.mockResolvedValue({
    period: "2026-09",
    organization_uuid: "org-1",
    organization_name: "Acme",
    metrics: {
      services_count: metricDelta("12"),
      // A stale value must not leak into a disabled module's card.
      review_avg: metricDelta("4.5"),
    },
    trend: [],
    targets: [],
    subtree: [],
  });
  api.ranking.mockResolvedValue({ items: [], total: 0, limit: 20, offset: 0 });
  api.benchmark.mockResolvedValue({
    period: "2026-09",
    own: { metrics: { services_count: { value: "12" } }, currency: "TRY" },
    network_average: { services_count: { value: "10" } },
  });
  api.regionMap.mockResolvedValue({
    level: "province",
    period: "2026-09",
    metric: "services_count",
    items: [],
    empty_regions: [],
    missing_coordinates: 0,
  });
  api.dealerMap.mockResolvedValue({
    period: "2026-09",
    metric: "services_count",
    items: [],
    missing_coordinates: 0,
  });
  api.reportCatalog.mockResolvedValue({ items: [] });
  api.reportLayout.mockResolvedValue({
    version: 1,
    updated_at: null,
    widgets: [],
  });
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
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

const q = (sel: string) => container.querySelector(sel);

/** Radix tabs activate on a primary-button mousedown. */
async function pickTab(id: string) {
  const trigger = q(`[data-testid="performance-tab-${id}"]`)!;
  await act(async () => {
    trigger.dispatchEvent(
      new MouseEvent("mousedown", { bubbles: true, button: 0 }),
    );
  });
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

describe("DashboardPanel", () => {
  it("greys out a metric whose module is off and shows no value", async () => {
    state.features = ["performance", "measurements"];
    await render(
      createElement(DashboardPanel, { slug: "acme", period: "2026-09" }),
    );

    const off = q('[data-testid="metric-card-review_avg"]')!;
    expect(off.getAttribute("data-module-off")).toBe("true");
    expect(off.textContent).toContain("performance.module_off");
    expect(off.querySelector('[data-testid="metric-value"]')).toBeNull();
    expect(off.textContent).not.toContain("4.5");

    // Module on (measurements) and metrics without a module keep a value.
    const on = q('[data-testid="metric-card-services_count"]')!;
    expect(on.getAttribute("data-module-off")).toBeNull();
    expect(on.querySelector('[data-testid="metric-value"]')?.textContent).toBe(
      "12",
    );
    expect(
      q('[data-testid="metric-card-measurement_rate"]')!.getAttribute(
        "data-module-off",
      ),
    ).toBeNull();
    for (const metric of [
      "certificate_coverage",
      "lead_conversion_rate",
      "waste_ratio",
    ]) {
      expect(
        q(`[data-testid="metric-card-${metric}"]`)!.getAttribute(
          "data-module-off",
        ),
      ).toBe("true");
    }
  });

  it("shows the target achievement ring on target metrics", async () => {
    api.dashboard.mockResolvedValue({
      period: "2026-09",
      metrics: { services_count: metricDelta("12") },
      trend: [],
      targets: [
        {
          metric: "services_count",
          period_start: "2026-09-01",
          period_end: "2026-09-30",
          achievement_pct: "75.00",
        },
      ],
    });
    await render(
      createElement(DashboardPanel, { slug: "acme", period: "2026-09" }),
    );
    const ring = q(
      '[data-testid="metric-card-services_count"] [data-testid="metric-target-ring"]',
    );
    expect(ring?.textContent).toBe("75%");
    expect(
      q(
        '[data-testid="metric-card-order_volume"] [data-testid="metric-target-ring"]',
      ),
    ).toBeNull();
  });
});

describe("PerformancePage", () => {
  it("shows the position card instead of the ranking table to a dealer", async () => {
    state.orgType = "dealer";
    await render(createElement(PerformancePage, { slug: "acme" }));
    expect(q('[data-testid="performance-tab-ranking"]')?.textContent).toBe(
      "performance.tabs.position",
    );
    expect(q('[data-testid="performance-tab-map"]')).toBeNull();

    await pickTab("ranking");
    expect(q('[data-testid="performance-position-card"]')).not.toBeNull();
    expect(q('[data-testid="performance-ranking"]')).toBeNull();
    expect(api.benchmark).toHaveBeenCalled();
    expect(api.ranking).not.toHaveBeenCalled();
    expect(q('[data-testid="position-services_count"]')?.textContent).toContain(
      "performance.position.better",
    );
  });

  it("shows the ranking table to a center with distributor and province facets", async () => {
    state.orgType = "center";
    state.grants.add(Permission.TasksWrite);
    await render(createElement(PerformancePage, { slug: "acme" }));
    await pickTab("ranking");

    expect(q('[data-testid="performance-ranking"]')).not.toBeNull();
    expect(q('[data-testid="performance-position-card"]')).toBeNull();
    const call = api.ranking.mock.calls
      .map((c) => c[0] as Record<string, unknown>)
      .find((p) => p.type !== "distributor")!;
    expect(call).toMatchObject({ sort: "-services_count" });
    expect(typeof call.period).toBe("string");
    const table = captured.tables.at(-1)!;
    const ids = (table.columns as { id?: string; accessorKey?: string }[]).map(
      (c) => c.id ?? c.accessorKey,
    );
    expect(ids).toEqual(
      expect.arrayContaining([
        "name",
        "distributor",
        "province",
        "services_count",
        "waste_ratio",
        "actions",
      ]),
    );
  });

  it("is forbidden without performance.read", async () => {
    state.grants = new Set();
    await render(createElement(PerformancePage, { slug: "acme" }));
    expect(container.textContent).toContain("performance.page.forbidden");
    expect(api.dashboard).not.toHaveBeenCalled();
  });

  it("offers the map to center and distributor only", () => {
    expect(performanceTabs("center")).toContain("map");
    expect(performanceTabs("distributor")).toContain("map");
    expect(performanceTabs("dealer")).not.toContain("map");
  });
});

describe("RegionMap", () => {
  it("sends the picked level as the map query parameter", async () => {
    state.orgType = "center";
    await render(createElement(RegionMap, { slug: "acme", period: "2026-09" }));
    expect(api.regionMap).toHaveBeenLastCalledWith(
      expect.objectContaining({ level: "province", country: "TR" }),
    );

    await act(async () => {
      (q('[data-testid="map-level-district"]') as HTMLButtonElement).click();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(api.regionMap).toHaveBeenLastCalledWith(
      expect.objectContaining({ level: "district", period: "2026-09" }),
    );

    await act(async () => {
      (q('[data-testid="map-level-country"]') as HTMLButtonElement).click();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    const last = api.regionMap.mock.calls.at(-1)![0];
    expect(last.level).toBe("country");
    expect(last.country).toBeUndefined();
  });

  it("draws region circles with the territory owner and lists empty regions", async () => {
    api.regionMap.mockResolvedValue({
      level: "province",
      period: "2026-09",
      metric: "services_count",
      items: [
        {
          level: "province",
          id: 34,
          code: "34",
          name: "İstanbul",
          country_iso2: "TR",
          country_name: "Türkiye",
          dealer_count: 4,
          missing_coordinates: 0,
          latitude: 41,
          longitude: 29,
          metric_avg: 8,
          distributor: { uuid: "d1", name: "Marmara Dağıtım" },
        },
      ],
      empty_regions: [
        {
          level: "province",
          id: 6,
          code: "06",
          name: "Ankara",
          country_iso2: "TR",
          empty_reason: "unassigned_territory",
        },
      ],
      missing_coordinates: 0,
    });
    await render(createElement(RegionMap, { slug: "acme", period: "2026-09" }));
    const markers = captured.maps.at(-1)!.markers as {
      label: string;
      radius: number;
    }[];
    expect(markers).toHaveLength(1);
    expect(markers[0].label).toContain("Marmara Dağıtım");
    expect(markers[0].radius).toBe(30);
    expect(captured.tables.at(-1)!.data).toHaveLength(1);
  });
});

describe("performance navigation", () => {
  function visible(granted: string[], features: string[]) {
    const access = {
      can: (p: string | string[]) =>
        (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
      canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
      org: { type: "dealer", role: "owner", features },
    };
    return tenantNav("acme").groups.flatMap((group) =>
      visibleNavItems(group, access).map((item) => item.id),
    );
  }

  it("shows the menu only with the performance module and permission", () => {
    expect(visible([Permission.PerformanceRead], ["performance"])).toContain(
      "performance",
    );
    expect(visible([Permission.PerformanceRead], [])).not.toContain(
      "performance",
    );
    expect(visible([], ["performance"])).not.toContain("performance");
  });
});
