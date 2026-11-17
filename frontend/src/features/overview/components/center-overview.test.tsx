// @vitest-environment jsdom
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ReportQuery } from "@/features/reports/services/reports.service";

// TEC-1220: the center overview ranks the most serviced make-models and the
// most used products, both on the overview's one period query.

type PanelCall = { reportKey: string; query: ReportQuery; title?: string };
const rankCalls: PanelCall[] = [];
let shellQuery: ReportQuery = {};

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key, locale: "en" }),
}));
vi.mock("@/features/overview/components/overview-shell", () => ({
  OverviewShell: ({
    children,
  }: {
    children: (query: ReportQuery) => ReactNode;
  }) => children(shellQuery),
  OverviewSection: ({ children }: { children: ReactNode }) => children,
}));
vi.mock("@/features/reports/components/report-panels", () => ({
  ReportKpiStrip: () => null,
  ReportTrendPanel: () => null,
  ReportTablePanel: () => null,
  ReportRankPanel: (props: PanelCall) => {
    rankCalls.push({
      reportKey: props.reportKey,
      query: props.query,
      title: props.title,
    });
    return createElement("div", { "data-rank": props.reportKey });
  },
}));
vi.mock("@/features/announcements", () => ({
  RecentAnnouncementsWidget: () => null,
}));
vi.mock("@/features/stock-forecast", () => ({
  StockForecastWidget: () => null,
}));
vi.mock("@/features/stock", () => ({ StockSummaryWidget: () => null }));

import { CenterOverview } from "./center-overview";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let host: HTMLDivElement;
let root: Root;
beforeEach(() => {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  rankCalls.length = 0;
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

const lastCall = (key: string) =>
  rankCalls.filter((c) => c.reportKey === key).at(-1);

describe("CenterOverview", () => {
  it("shows the make-model and product rankings on the period query", () => {
    shellQuery = { period: "30d", compare: "previous" };
    act(() => root.render(createElement(CenterOverview, { slug: "olex" })));
    const top = host.querySelector('[data-testid="center-top"]');
    expect(
      top?.querySelector('[data-rank="services.top_models"]'),
    ).not.toBeNull();
    expect(
      top?.querySelector('[data-rank="services.top_products"]'),
    ).not.toBeNull();
    expect(lastCall("services.top_models")).toMatchObject({
      query: { period: "30d" },
      title: "dashboard.role.top_models",
    });
    expect(lastCall("services.top_products")).toMatchObject({
      query: { period: "30d" },
      title: "dashboard.role.top_products",
    });

    // A new period (e.g. "Today") reaches both rankings.
    shellQuery = { period: "today", compare: "previous" };
    act(() => root.render(createElement(CenterOverview, { slug: "olex" })));
    expect(lastCall("services.top_models")?.query.period).toBe("today");
    expect(lastCall("services.top_products")?.query.period).toBe("today");
  });
});
