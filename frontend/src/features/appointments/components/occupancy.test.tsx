// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const http = vi.hoisted(() => ({ platformRequest: vi.fn() }));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "center" as string | null,
  feature: true,
  push: vi.fn(),
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
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: state.push }),
  usePathname: () => "/t/acme/appointments/occupancy",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    dir: "ltr",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      timeZone: "UTC",
      number: (v: number) => String(v),
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () =>
    state.orgType
      ? { uuid: "org-1", slug: "acme", name: "Acme", type: state.orgType }
      : null,
}));
vi.mock("@/features/modules/hooks/use-features", () => ({
  useFeature: () => ({
    enabled: state.feature,
    isLoading: false,
    isError: false,
  }),
}));
vi.mock("@/lib/api/platform-request", () => http);
// The table renders each row's cells, the empty title and row clicks.
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityTable: (props: {
    columns: {
      id?: string;
      accessorKey?: string;
      cell?: (ctx: unknown) => ReactNode;
    }[];
    data: { organization_uuid: string }[];
    isLoading?: boolean;
    emptyTitle?: string;
    emptyDescription?: string;
    onRowClick?: (row: unknown) => void;
  }) => {
    if (props.isLoading) return createElement("div", null, "loading");
    if (props.data.length === 0) {
      return createElement(
        "div",
        { "data-testid": "table-empty" },
        `${props.emptyTitle} / ${props.emptyDescription}`,
      );
    }
    return createElement(
      "div",
      null,
      props.data.map((row) =>
        createElement(
          "div",
          {
            key: row.organization_uuid,
            "data-testid": "table-row",
            onClick: () => props.onRowClick?.(row),
          },
          props.columns.map((c, i) =>
            createElement(
              "span",
              { key: c.id ?? c.accessorKey ?? i },
              c.cell?.({
                row: { original: row },
                getValue: () => undefined,
              }) ?? null,
            ),
          ),
        ),
      ),
    );
  },
}));

import { Permission } from "@/config/permissions";
import {
  aggregateOccupancy,
  OCCUPANCY_THRESHOLDS,
  occupancyLevel,
  occupancyPercent,
  occupancyRate,
  summarizeOccupancy,
} from "@/features/appointments/lib/occupancy";
import type { AppointmentOccupancy } from "@/features/appointments/services/appointments.service";

import { NetworkOccupancyWidget } from "./network-occupancy-widget";
import { OccupancyPage } from "./occupancy-page";
import { OrganizationCalendarPage } from "./organization-calendar-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

const occ = (
  uuid: string,
  name: string,
  capacity: number,
  occupied: number,
): AppointmentOccupancy => ({
  organization_id: 1,
  organization_uuid: uuid,
  organization_name: name,
  capacity,
  occupied,
  remaining: Math.max(capacity - occupied, 0),
});

let container: HTMLDivElement;
let root: Root;

async function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  }
}

function mockOccupancy(rows: AppointmentOccupancy[]) {
  http.platformRequest.mockImplementation(
    async (method: string, path: string) => {
      if (method === "GET" && path === "/v1/appointments/occupancy") {
        return rows;
      }
      throw new Error(`unexpected ${method} ${path}`);
    },
  );
}

const badges = () =>
  [
    ...document.querySelectorAll<HTMLElement>(
      '[data-testid="table-row"] [data-testid="occupancy-badge"]',
    ),
  ].map((b) => [b.dataset.level, b.textContent]);

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  http.platformRequest.mockReset();
  state.push.mockReset();
  state.grants = new Set([Permission.AppointmentsRead]);
  state.orgType = "center";
  state.feature = true;
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

describe("occupancy rate and heat level (TEC-328)", () => {
  it("computes occupied / capacity and the rounded percent", () => {
    expect(occupancyRate(3, 4)).toBe(0.75);
    expect(occupancyPercent(occupancyRate(2, 3))).toBe(67);
    expect(occupancyPercent(occupancyRate(5, 4))).toBe(125);
    expect(occupancyRate(2, 0)).toBeNull();
    expect(occupancyPercent(null)).toBeNull();
  });

  it("colors by the single threshold table", () => {
    expect(OCCUPANCY_THRESHOLDS).toEqual({ medium: 0.5, high: 0.8, full: 1 });
    expect(occupancyLevel(null)).toBe("none");
    expect(occupancyLevel(0)).toBe("low");
    expect(occupancyLevel(0.49)).toBe("low");
    expect(occupancyLevel(0.5)).toBe("medium");
    expect(occupancyLevel(0.79)).toBe("medium");
    expect(occupancyLevel(0.8)).toBe("high");
    expect(occupancyLevel(0.99)).toBe("high");
    expect(occupancyLevel(1)).toBe("full");
    expect(occupancyLevel(1.2)).toBe("full");
  });

  it("sums a week per organization and summarizes the network", () => {
    const rows = aggregateOccupancy([
      [occ("a", "A", 4, 4), occ("b", "B", 0, 0)],
      [occ("a", "A", 4, 2), occ("b", "B", 2, 0)],
    ]);
    expect(rows).toEqual([
      expect.objectContaining({
        organization_uuid: "a",
        capacity: 8,
        occupied: 6,
        rate: 0.75,
        level: "medium",
      }),
      expect.objectContaining({
        organization_uuid: "b",
        capacity: 2,
        occupied: 0,
        rate: 0,
        level: "low",
      }),
    ]);
    const summary = summarizeOccupancy(rows);
    expect(summary).toMatchObject({
      organizations: 2,
      capacity: 10,
      occupied: 6,
      rate: 0.6,
      level: "medium",
    });
    expect(summary.levels).toMatchObject({ medium: 1, low: 1 });
  });
});

describe("OccupancyPage (TEC-328)", () => {
  it("shows percent and heat color per organization", async () => {
    mockOccupancy([
      occ("o-1", "Kadıköy", 10, 3),
      occ("o-2", "Ankara", 10, 5),
      occ("o-3", "İzmir", 5, 4),
      occ("o-4", "Bursa", 4, 4),
      occ("o-5", "Antalya", 0, 0),
    ]);
    await render(createElement(OccupancyPage, { slug: "acme" }));
    expect(http.platformRequest).toHaveBeenCalledWith(
      "GET",
      "/v1/appointments/occupancy",
      { query: { date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/) } },
    );
    expect(badges()).toEqual([
      ["low", 'appointments.occupancy.percent {"value":"30"}'],
      ["medium", 'appointments.occupancy.percent {"value":"50"}'],
      ["high", 'appointments.occupancy.percent {"value":"80"}'],
      ["full", 'appointments.occupancy.percent {"value":"100"}'],
      ["none", "—"],
    ]);
  });

  it("opens the organization's calendar read-only on row click", async () => {
    mockOccupancy([occ("o-1", "Kadıköy", 10, 3)]);
    await render(createElement(OccupancyPage, { slug: "acme" }));
    act(() => {
      (
        document.querySelector('[data-testid="table-row"]') as HTMLElement
      ).click();
    });
    expect(state.push).toHaveBeenCalledWith(
      expect.stringMatching(
        /^\/t\/acme\/appointments\/occupancy\/o-1\?name=Kad%C4%B1k%C3%B6y&date=\d{4}-\d{2}-\d{2}&view=day$/,
      ),
    );
  });

  it("shows the empty state text without data", async () => {
    mockOccupancy([]);
    await render(createElement(OccupancyPage, { slug: "acme" }));
    expect(
      document.querySelector('[data-testid="table-empty"]')?.textContent,
    ).toBe(
      "appointments.occupancy.empty / appointments.occupancy.empty_description",
    );
  });

  it("sums seven day requests in the week view", async () => {
    mockOccupancy([occ("o-1", "Kadıköy", 2, 1)]);
    await render(createElement(OccupancyPage, { slug: "acme" }));
    act(() => {
      (
        document.querySelector(
          '[data-testid="occupancy-view-week"]',
        ) as HTMLElement
      ).click();
    });
    await render(createElement(OccupancyPage, { slug: "acme" }));
    const dates = new Set(
      http.platformRequest.mock.calls.map(
        (c) => (c[2] as { query: { date: string } }).query.date,
      ),
    );
    expect(dates.size).toBeGreaterThanOrEqual(7);
  });

  it("is forbidden for a dealer and does not call the API", async () => {
    state.orgType = "dealer";
    mockOccupancy([]);
    await render(createElement(OccupancyPage, { slug: "acme" }));
    expect(document.body.textContent).toContain(
      "appointments.occupancy.forbidden",
    );
    expect(http.platformRequest).not.toHaveBeenCalled();
  });
});

describe("OrganizationCalendarPage (TEC-328)", () => {
  it("narrows the calendar to the organization, read-only", async () => {
    http.platformRequest.mockImplementation(
      async (method: string, path: string) => {
        if (path === "/v1/appointments/availability") return [];
        if (path === "/v1/appointments") {
          return { items: [], total: 0, limit: 100, offset: 0 };
        }
        throw new Error(`unexpected ${method} ${path}`);
      },
    );
    state.grants.add(Permission.AppointmentsWrite);
    await render(
      createElement(OrganizationCalendarPage, {
        slug: "acme",
        organizationUuid: "o-1",
        name: "Kadıköy",
        date: "2026-10-07",
        view: "day",
      }),
    );
    const calendar = document.querySelector(
      '[data-testid="appointment-calendar"]',
    );
    expect(calendar?.getAttribute("data-read-only")).toBe("true");
    expect(document.body.textContent).toContain("Kadıköy");
    expect(document.body.textContent).not.toContain("appointments.new");
    for (const path of ["/v1/appointments/availability", "/v1/appointments"]) {
      const call = http.platformRequest.mock.calls.find((c) => c[1] === path);
      expect(call?.[2]).toMatchObject({
        query: { organization_uuid: "o-1" },
      });
    }
  });
});

describe("NetworkOccupancyWidget (TEC-328)", () => {
  it("summarizes today's network occupancy", async () => {
    mockOccupancy([occ("o-1", "Kadıköy", 4, 4), occ("o-2", "Ankara", 6, 2)]);
    await render(createElement(NetworkOccupancyWidget, { slug: "acme" }));
    const rate = document.querySelector(
      '[data-testid="network-occupancy-rate"] [data-testid="occupancy-badge"]',
    ) as HTMLElement;
    expect(rate.dataset.level).toBe("medium");
    expect(rate.textContent).toBe(
      'appointments.occupancy.percent {"value":"60"}',
    );
    expect(
      document.querySelector('[data-testid="network-occupancy-capacity"]')
        ?.textContent,
    ).toBe("10");
    expect(
      document.querySelector('[data-testid="network-occupancy-top"] a')
        ?.textContent,
    ).toBe("Kadıköy");
  });

  it("shows the empty text without organizations", async () => {
    mockOccupancy([]);
    await render(createElement(NetworkOccupancyWidget, { slug: "acme" }));
    expect(
      document.querySelector('[data-testid="network-occupancy-widget-empty"]')
        ?.textContent,
    ).toBe("appointments.occupancy.empty");
  });

  it("is hidden for a dealer", async () => {
    state.orgType = "dealer";
    mockOccupancy([]);
    await render(createElement(NetworkOccupancyWidget, { slug: "acme" }));
    expect(
      document.querySelector('[data-testid="network-occupancy-widget"]'),
    ).toBeNull();
    expect(http.platformRequest).not.toHaveBeenCalled();
  });
});
