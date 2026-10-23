// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string;
    children: ReactNode;
  } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    dir: "ltr",
    format: { date: (v: string) => `d:${v.slice(0, 10)}` },
  }),
}));
const perm = vi.hoisted(() => ({ can: vi.fn(() => true) }));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: perm.can }),
}));
const org = vi.hoisted(() => ({ type: "dealer" as string | undefined }));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => (org.type ? { type: org.type } : null),
}));
const api = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock("@/features/services/services/home-overview.service", async (orig) => ({
  ...(await orig<object>()),
  homeOverviewService: { get: api.get },
}));

import type { HomeOverview } from "../services/home-overview.service";
import { HomeOverviewCard, HomeOverviewWidget } from "./home-overview";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const brand = (name: string) => ({
  uuid: `b-${name}`,
  name,
  has_logo: false,
  logo_url: `/brand-logos/b-${name}`,
});

const DATA: HomeOverview = {
  period: "90d",
  since: "2026-07-23T00:00:00Z",
  top_vehicles: [
    {
      service_count: 8,
      car_brand: brand("BMW"),
      car_model: { uuid: "m-1", name: "X5" },
    },
    {
      service_count: 2,
      car_brand: brand("Audi"),
      car_model: { uuid: "m-2", name: "A4" },
    },
  ],
  top_products: [
    {
      uuid: "p-1",
      sku: "PPF-1",
      name: "PPF Gloss",
      unit_type: "meter",
      service_count: 5,
      quantity: 0,
      meters: "42.50",
    },
  ],
  recent_services: [
    {
      uuid: "s-1",
      service_no: "DS00000001",
      plate: "34ABC123",
      plate_country: "TR",
      model_year: 2024,
      package: "Full body",
      products: ["PPF Gloss", "Ceramic"],
      completed_at: "2026-10-20T10:00:00Z",
      vehicle_uuid: "v-1",
      car_brand: brand("BMW"),
      car_model: { uuid: "m-1", name: "X5" },
    },
    {
      uuid: "s-2",
      service_no: "DS00000002",
      plate: null,
      plate_country: null,
      model_year: null,
      package: "Front kit",
      products: [],
      completed_at: "2026-10-19T10:00:00Z",
      vehicle_uuid: "v-2",
      car_brand: brand("Audi"),
      car_model: { uuid: "m-2", name: "A4" },
    },
  ],
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  perm.can.mockReturnValue(true);
  org.type = "dealer";
  api.get.mockReset();
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

const q = (sel: string) => container.querySelector(sel);

describe("HomeOverviewCard", () => {
  it("ranks vehicles and products and lists the latest services", async () => {
    await render(
      createElement(HomeOverviewCard, {
        slug: "acme",
        data: DATA,
        period: "90d",
        onPeriodChange: () => {},
      }),
    );
    const vehicles = q('[data-testid="home-overview-vehicles"]');
    expect(vehicles?.querySelectorAll("li")).toHaveLength(2);
    expect(vehicles?.querySelector('[data-rank="1"]')?.textContent).toContain(
      "BMW X5",
    );
    expect(vehicles?.querySelector('[data-rank="1"]')?.textContent).toContain(
      "8",
    );
    expect(q('[data-testid="home-overview-products"]')?.textContent).toContain(
      "PPF Gloss",
    );

    const recent = q('[data-testid="home-overview-recent"]');
    const links = Array.from(recent?.querySelectorAll("li a") ?? []);
    expect(links.map((a) => a.getAttribute("href"))).toEqual([
      "/t/acme/services/s-1",
      "/t/acme/services/s-2",
    ]);
    expect(links[0]?.textContent).toContain("34ABC123");
    expect(links[0]?.textContent).toContain("BMW X5 2024");
    expect(links[0]?.textContent).toContain("PPF Gloss, Ceramic");
    expect(links[0]?.textContent).toContain("d:2026-10-20");
    // No plate and no products: the placeholder and the package name.
    expect(links[1]?.textContent).toContain("dashboard.overview.no_plate");
    expect(links[1]?.textContent).toContain("Front kit");
    expect(recent?.querySelector('a[href="/t/acme/services"]')).not.toBeNull();
  });

  it("shows empty states and the period switch", async () => {
    const onPeriodChange = vi.fn();
    await render(
      createElement(HomeOverviewCard, {
        slug: "acme",
        data: {
          ...DATA,
          top_vehicles: [],
          top_products: [],
          recent_services: [],
        },
        period: "90d",
        onPeriodChange,
      }),
    );
    expect(container.textContent).toContain("dashboard.overview.empty");
    expect(container.textContent).toContain("dashboard.overview.recent_empty");
    const btn = q('[data-period="30d"]') as HTMLElement | null;
    await act(async () => {
      btn?.click();
    });
    expect(onPeriodChange).toHaveBeenCalledWith("30d");
  });

  it("shows the error with a retry", async () => {
    const onRetry = vi.fn();
    await render(
      createElement(HomeOverviewCard, {
        slug: "acme",
        data: undefined,
        period: "90d",
        onPeriodChange: () => {},
        isError: true,
        onRetry,
      }),
    );
    expect(q('[data-testid="home-overview-error"]')).not.toBeNull();
  });
});

describe("HomeOverviewWidget", () => {
  it("loads the 90 day overview in a dealer organization", async () => {
    api.get.mockResolvedValue(DATA);
    await render(createElement(HomeOverviewWidget, { slug: "acme" }));
    expect(api.get).toHaveBeenCalledWith("90d");
    expect(q('[data-testid="home-overview"]')).not.toBeNull();
  });

  it("renders nothing outside a dealer or without services.read", async () => {
    org.type = "center";
    await render(createElement(HomeOverviewWidget, { slug: "acme" }));
    expect(q('[data-testid="home-overview"]')).toBeNull();
    org.type = "dealer";
    perm.can.mockReturnValue(false);
    await render(createElement(HomeOverviewWidget, { slug: "acme" }));
    expect(q('[data-testid="home-overview"]')).toBeNull();
    expect(api.get).not.toHaveBeenCalled();
  });
});
