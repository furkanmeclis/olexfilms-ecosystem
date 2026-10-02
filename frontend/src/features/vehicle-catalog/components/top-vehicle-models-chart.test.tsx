// @vitest-environment jsdom
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

let dir: "ltr" | "rtl" = "ltr";
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key, dir }),
  useFormatter: () => ({ number: (n: number) => String(n) }),
}));

import type { TopVehicle } from "../services/vehicle-stats.service";
import {
  canSeeTopVehicles,
  toTopVehicleBars,
  truncateLabel,
} from "../lib/top-vehicles";
import { TopVehicleModelsChart } from "./top-vehicle-models-chart";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

beforeAll(() => {
  // jsdom has no layout: report a fixed size to recharts'
  // ResponsiveContainer so the chart actually draws.
  globalThis.ResizeObserver = class {
    constructor(private cb: ResizeObserverCallback) {}
    observe(target: Element) {
      this.cb(
        [
          {
            target,
            contentRect: { width: 480, height: 200 },
          } as ResizeObserverEntry,
        ],
        this as unknown as ResizeObserver,
      );
    }
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  dir = "ltr";
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

const BMW = "11111111-1111-4111-8111-111111111111";
const AUDI = "22222222-2222-4222-8222-222222222222";

function row(
  count: number,
  brandUuid: string,
  brand: string,
  model: string | null,
  hasLogo = true,
): TopVehicle {
  return {
    service_count: count,
    car_brand: {
      uuid: brandUuid,
      name: brand,
      has_logo: hasLogo,
      logo_url: hasLogo
        ? `/brand-logos/${brandUuid}?v=abc`
        : `/brand-logos/${brandUuid}`,
    },
    car_model: model
      ? {
          uuid: `${brandUuid.slice(0, 30)}${String(count).padStart(6, "0")}`,
          name: model,
        }
      : null,
  };
}

const MODELS: TopVehicle[] = [
  row(12, BMW, "BMW", "3 Series"),
  row(7, AUDI, "Audi", "A4", false),
  row(3, BMW, "BMW", "X5"),
];

function render(
  props: Partial<Parameters<typeof TopVehicleModelsChart>[0]> = {},
) {
  const handlers = { onPeriodChange: vi.fn(), onGroupChange: vi.fn() };
  act(() =>
    root.render(
      createElement(TopVehicleModelsChart, {
        items: MODELS,
        period: "30d",
        group: "model",
        ...handlers,
        ...props,
      }),
    ),
  );
  return handlers;
}

describe("TopVehicleModelsChart", () => {
  it("draws one row per item in rank order with counts and labels", () => {
    render();
    const rows = container.querySelectorAll(
      '[data-slot="top-vehicle-models-table"] tbody tr',
    );
    expect(Array.from(rows, (r) => r.textContent)).toEqual([
      "1BMW 3 Series12",
      "2Audi A47",
      "3BMW X53",
    ]);
    expect(container.querySelector('[data-slot="chart"]')).not.toBeNull();
    expect(
      container.querySelector('[data-slot="top-vehicle-models-empty"]'),
    ).toBeNull();
  });

  it("renders bars and logo ticks", () => {
    render();
    const ticks = container.querySelectorAll('[data-slot="top-vehicle-tick"]');
    expect(ticks.length).toBe(3);
    const hrefs = Array.from(ticks, (t) =>
      t.querySelector("image")?.getAttribute("href"),
    );
    expect(hrefs).toEqual([
      `/brand-logos/${BMW}?v=abc`,
      `/brand-logos/${AUDI}`,
      `/brand-logos/${BMW}?v=abc`,
    ]);
    expect(container.querySelectorAll(".recharts-bar-rectangle").length).toBe(
      3,
    );
  });

  it("shows the empty state without data", () => {
    render({ items: [] });
    const empty = container.querySelector(
      '[data-slot="top-vehicle-models-empty"]',
    );
    expect(empty?.textContent).toContain("dashboard.top_vehicles.empty");
    expect(container.querySelector('[data-slot="chart"]')).toBeNull();
  });

  it("shows the loading state", () => {
    render({ items: undefined, isLoading: true });
    expect(
      container.querySelector('[data-slot="top-vehicle-models-loading"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[data-slot="top-vehicle-models-empty"]'),
    ).toBeNull();
  });

  it("switches period and group", () => {
    const { onPeriodChange, onGroupChange } = render();
    act(() => {
      container
        .querySelector<HTMLButtonElement>('[data-period="12m"]')!
        .click();
    });
    expect(onPeriodChange).toHaveBeenCalledWith("12m");
    act(() => {
      container
        .querySelector<HTMLButtonElement>('[data-group="brand"]')!
        .click();
    });
    expect(onGroupChange).toHaveBeenCalledWith("brand");
  });

  it("puts the category axis on the right in RTL", () => {
    dir = "rtl";
    render();
    const tick = container.querySelector('[data-slot="top-vehicle-tick"] text');
    expect(tick?.getAttribute("direction")).toBe("rtl");
    expect(Number(tick?.getAttribute("x"))).toBeGreaterThan(0);
  });
});

describe("top vehicle helpers", () => {
  it("labels brand rows with the brand only", () => {
    const bars = toTopVehicleBars([row(4, BMW, "BMW", null)], "brand");
    expect(bars[0]).toMatchObject({
      key: BMW,
      label: "BMW",
      count: 4,
      logoVersion: "abc",
    });
  });

  it("truncates long labels", () => {
    expect(truncateLabel("Mercedes-Benz GLE Coupé AMG", 10)).toBe("Mercedes-…");
    expect(truncateLabel("BMW X5")).toBe("BMW X5");
  });

  it("is visible to the center and super_admin only", () => {
    expect(canSeeTopVehicles(() => "brand", "center")).toBe(true);
    expect(canSeeTopVehicles(() => "all", "dealer")).toBe(true);
    expect(canSeeTopVehicles(() => "brand", "dealer")).toBe(false);
    expect(canSeeTopVehicles(() => "subtree", "distributor")).toBe(false);
    expect(canSeeTopVehicles(() => undefined, "center")).toBe(false);
  });
});
