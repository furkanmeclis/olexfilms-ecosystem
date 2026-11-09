// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: ReactNode }) =>
    createElement("a", { href }, children),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    dir: "ltr",
    format: {
      number: (v: number) => String(v),
      dateTime: (v: string) => v,
    },
  }),
  useFormatter: () => ({ number: (n: number) => String(n) }),
}));
const perm = vi.hoisted(() => ({ scope: "brand" as string | undefined }));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ scopeFor: () => perm.scope, can: () => true }),
}));
const org = vi.hoisted(() => ({ type: "center" as string | undefined }));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => (org.type ? { type: org.type } : null),
}));
const api = vi.hoisted(() => ({
  overview: vi.fn(),
  topProducts: vi.fn(),
  listServices: vi.fn(),
  topVehicles: vi.fn(),
}));
vi.mock(
  "@/features/center-overview/services/center-overview.service",
  async (orig) => ({
    ...(await orig<object>()),
    centerOverviewService: {
      overview: api.overview,
      topProducts: api.topProducts,
    },
  }),
);
vi.mock(
  "@/features/services/services/service-wizard.service",
  async (orig) => ({
    ...(await orig<object>()),
    serviceWizardService: { listServices: api.listServices },
  }),
);
vi.mock(
  "@/features/vehicle-catalog/services/vehicle-stats.service",
  async (orig) => ({
    ...(await orig<object>()),
    vehicleStatsService: { topVehicleModels: api.topVehicles },
  }),
);

import {
  centerPeriodStart,
  overviewCard,
  type ReportEnvelope,
} from "../services/center-overview.service";
import { CenterOverviewSection } from "./center-overview";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const env = (
  series: ReportEnvelope["series"],
  report = "overview",
): ReportEnvelope => ({
  report,
  kind: "summary",
  title: "",
  locale: "tr",
  timezone: "Europe/Istanbul",
  scope: "brand",
  period: "30d",
  range_from: null,
  range_to: null,
  granularity: null,
  generated_at: "2026-11-09T00:00:00Z",
  series,
  columns: [],
  rows: [],
});

const OVERVIEW = env([
  {
    key: "cards",
    label: "",
    unit: "count",
    points: [
      { key: "dealers_total", label: "", value: 42 },
      { key: "dealers_active", label: "", value: 40 },
      { key: "distributors_total", label: "", value: 3 },
      { key: "services_created", label: "", value: 120 },
      { key: "services_completed", label: "", value: 100 },
      { key: "warranties_started", label: "", value: 90 },
      { key: "warranties_active", label: "", value: 900 },
    ],
  },
]);

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  perm.scope = "brand";
  org.type = "center";
  api.overview.mockReset().mockResolvedValue(OVERVIEW);
  api.topProducts.mockReset().mockResolvedValue(
    env(
      [
        {
          key: "service_count",
          label: "",
          unit: "count",
          points: [
            { key: "p1", label: "PPF Gloss", value: 30 },
            { key: "p2", label: "Ceramic", value: 12 },
          ],
        },
      ],
      "services.top_products",
    ),
  );
  api.listServices.mockReset().mockResolvedValue({
    items: [],
    total: 0,
    limit: 8,
    offset: 0,
  });
  api.topVehicles.mockReset().mockResolvedValue({
    period: "30d",
    group: "model",
    since: null,
    items: [],
  });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

describe("CenterOverviewSection (TEC-1072)", () => {
  it("renders nothing outside the center", async () => {
    org.type = "dealer";
    perm.scope = "managed";
    await render(createElement(CenterOverviewSection, { slug: "olex" }));
    expect(container.textContent).toBe("");
    expect(api.overview).not.toHaveBeenCalled();
  });

  it("shows the network, service and warranty cards and the top products", async () => {
    await render(createElement(CenterOverviewSection, { slug: "olex" }));
    const cards = container.querySelector(
      "[data-testid=center-overview-cards]",
    )?.textContent;
    expect(cards).toContain("dashboard.center.cards.dealers42");
    expect(cards).toContain("dashboard.center.cards.distributors3");
    expect(cards).toContain("dashboard.center.cards.services120");
    expect(cards).toContain("dashboard.center.cards.warranties90");
    const table = container.querySelector(
      '[data-slot="top-products-table"] tbody',
    );
    expect(
      Array.from(table?.querySelectorAll("tr") ?? [], (r) => r.textContent),
    ).toEqual(["1PPF Gloss30", "2Ceramic12"]);
  });

  it("one period drives every card, chart and the recent list", async () => {
    await render(createElement(CenterOverviewSection, { slug: "olex" }));
    expect(api.overview).toHaveBeenLastCalledWith("30d");
    await act(async () => {
      container
        .querySelector<HTMLButtonElement>(
          '[data-testid=center-overview] [data-period="today"]',
        )!
        .click();
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
    expect(api.overview).toHaveBeenLastCalledWith("today");
    expect(api.topProducts).toHaveBeenLastCalledWith("today");
    expect(api.topVehicles).toHaveBeenLastCalledWith({
      period: "today",
      group: "model",
    });
    const last = api.listServices.mock.calls.at(-1)?.[0];
    expect(last).toMatchObject({ sort: "-created_at", limit: 8 });
    const from = new Date(last.created_from);
    const midnight = new Date();
    midnight.setHours(0, 0, 0, 0);
    expect(from.getTime()).toBe(midnight.getTime());
    // The vehicles chart follows the section: no period switch of its own.
    expect(
      container.querySelectorAll(
        '[data-slot="top-vehicle-models"] [data-period]',
      ),
    ).toHaveLength(0);
  });
});

describe("center overview helpers", () => {
  it("period starts", () => {
    const now = new Date(2026, 10, 9, 15, 30);
    expect(centerPeriodStart("today", now)).toEqual(new Date(2026, 10, 9));
    expect(centerPeriodStart("7d", now)).toEqual(new Date(2026, 10, 3));
    expect(centerPeriodStart("30d", now)).toEqual(new Date(2026, 9, 11));
    expect(centerPeriodStart("12m", now)).toEqual(new Date(2025, 11, 1));
  });

  it("reads overview cards", () => {
    expect(overviewCard(OVERVIEW, "distributors_total")).toBe(3);
    expect(overviewCard(OVERVIEW, "missing")).toBeNull();
    expect(overviewCard(undefined, "dealers_total")).toBeNull();
  });
});
