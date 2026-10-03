// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const portal = vi.hoisted(() => ({
  listVehicles: vi.fn(),
  getVehicle: vi.fn(),
  getService: vi.fn(),
}));
const lang = vi.hoisted(() => ({
  locale: "en" as string,
  dir: "ltr" as "ltr" | "rtl",
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
  usePathname: () => "/portal/vehicles",
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
}));
vi.mock("qrcode", () => ({
  default: { toDataURL: vi.fn(async () => "data:image/png;base64,QR") },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: lang.locale,
    dir: lang.dir,
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/features/portal/lib/portal-client", async (orig) => ({
  ...(await orig<object>()),
  portalApi: portal,
}));

import type {
  PortalActiveWarranty,
  PortalService,
  PortalVehicle,
  PortalVehicleDetail as VehicleDetail,
} from "@/features/portal/lib/portal-client";
import {
  highlightedParts,
  portalVehicleTitle,
  whatsappUrl,
} from "@/features/portal/lib/portal-vehicles";

import { isPortalNavActive, PortalNav } from "./portal-nav";
import { PortalServiceView } from "./portal-service-detail";
import {
  PortalActiveWarrantyCard,
  PortalVehicleDetail,
} from "./portal-vehicle-detail";
import { PortalVehicles } from "./portal-vehicles";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  lang.locale = "en";
  lang.dir = "ltr";
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
});

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render(node: ReturnType<typeof createElement>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await flush();
}

async function click(el: Element | null) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
  await flush();
}

function vehicle(over: Partial<PortalVehicle> = {}): PortalVehicle {
  return {
    uuid: "v-1",
    car_brand: { uuid: "b-1", name: "BMW" },
    car_model: { uuid: "m-1", name: "320i" },
    model_year: 2021,
    plate: "34 ABC 123",
    plate_country: "TR",
    vin: "WBA00000000000001",
    service_count: 3,
    active_warranty_count: 1,
    last_service_at: "2026-05-01T10:00:00Z",
    created_at: "2025-01-01T10:00:00Z",
    ...over,
  };
}

function activeWarranty(
  over: Partial<PortalActiveWarranty> = {},
): PortalActiveWarranty {
  return {
    uuid: "w-1",
    public_code: "PUBCODE123456",
    start_at: "2026-01-01T00:00:00Z",
    end_at: "2027-01-01T00:00:00Z",
    days_left: 183,
    percent_left: 50,
    product: { uuid: "p-1", sku: "PPF-1", name: "Film PPF" },
    service: { uuid: "s-1", service_no: "SRV-0001" },
    organization: { uuid: "o-1", name: "Bayi Kadıköy", type: "dealer" },
    ...over,
  };
}

function service(over: Partial<PortalService> = {}): PortalService {
  return {
    uuid: "s-1",
    service_no: "SRV-0001",
    status: "completed",
    status_label: "Completed",
    vehicle_uuid: "v-1",
    car_brand: { uuid: "b-1", name: "BMW" },
    car_model: { uuid: "m-1", name: "320i" },
    model_year: 2021,
    plate: "34 ABC 123",
    created_at: "2026-05-01T10:00:00Z",
    completed_at: "2026-05-02T10:00:00Z",
    applied_parts: ["body_kaput", "body_tavan", "window_on_cam"],
    products: [
      {
        service_item_uuid: "i-1",
        name: "PPF Gloss",
        category: "PPF",
        applied_parts: ["body_kaput", "body_tavan"],
      },
      {
        service_item_uuid: "i-2",
        name: "Cam filmi",
        category: "Window film",
        applied_parts: ["window_on_cam"],
      },
    ],
    dealer: {
      uuid: "o-1",
      name: "Bayi Kadıköy",
      city: "İstanbul",
      district: "Kadıköy",
      address: "Moda Cad. 1",
      whatsapp: "+905321234567",
    },
    warranties: [],
    ...over,
  };
}

describe("portal vehicle helpers", () => {
  it("builds the vehicle title and drops empty parts", () => {
    expect(portalVehicleTitle(vehicle())).toBe("BMW 320i · 2021");
    expect(
      portalVehicleTitle(
        vehicle({ car_model: null, model_year: null, car_brand: null }),
      ),
    ).toBe("");
  });

  it("builds a wa.me link from E.164", () => {
    expect(whatsappUrl("+90 532 123 45 67")).toBe("https://wa.me/905321234567");
    expect(whatsappUrl(null)).toBeNull();
    expect(whatsappUrl("+90")).toBeNull();
  });

  it("highlights the focused product's parts or every applied part", () => {
    const s = service();
    expect([...highlightedParts(s, null)].sort()).toEqual([
      "body_kaput",
      "body_tavan",
      "window_on_cam",
    ]);
    expect([...highlightedParts(s, "i-2")]).toEqual(["window_on_cam"]);
    expect(highlightedParts(s, "missing").size).toBe(3);
  });

  it("marks the current nav section", () => {
    expect(isPortalNavActive("/portal", "/portal")).toBe(true);
    expect(isPortalNavActive("/portal", "/portal/vehicles")).toBe(false);
    expect(isPortalNavActive("/portal/vehicles", "/portal/vehicles/v-1")).toBe(
      true,
    );
    expect(isPortalNavActive("/portal/vehicles", "/portal/services/s-1")).toBe(
      true,
    );
    expect(isPortalNavActive("/portal/warranties", "/portal/vehicles")).toBe(
      false,
    );
  });
});

describe("PortalNav", () => {
  it("links home, vehicles, warranties and dealers and marks the current page", async () => {
    await render(createElement(PortalNav));
    const links = Array.from(container.querySelectorAll("nav a"));
    expect(links.map((a) => a.getAttribute("href"))).toEqual([
      "/portal",
      "/portal/vehicles",
      "/portal/warranties",
      "/portal/dealers",
    ]);
    expect(
      container
        .querySelector('a[href="/portal/vehicles"]')
        ?.getAttribute("aria-current"),
    ).toBe("page");
  });
});

describe("PortalVehicles", () => {
  it("lists the user's vehicles with links to the detail", async () => {
    portal.listVehicles.mockResolvedValue({
      items: [
        vehicle(),
        vehicle({ uuid: "v-2", plate: null, active_warranty_count: 0 }),
      ],
      total: 2,
      limit: 12,
      offset: 0,
    });
    await render(createElement(PortalVehicles));
    expect(portal.listVehicles).toHaveBeenCalledWith(12, 0);
    const rows = container.querySelectorAll('[data-testid="portal-vehicle"]');
    expect(rows).toHaveLength(2);
    expect(rows[0].querySelector("a")?.getAttribute("href")).toBe(
      "/portal/vehicles/v-1",
    );
    expect(rows[0].textContent).toContain("BMW 320i · 2021");
    expect(rows[0].querySelector('[dir="ltr"]')?.textContent).toBe(
      "34 ABC 123",
    );
    expect(
      rows[0].querySelector('[data-testid="active-warranty-count"]')
        ?.textContent,
    ).toContain('{"count":1}');
    expect(
      container.querySelector('[data-testid="portal-vehicles-empty"]'),
    ).toBeNull();
  });

  it("shows the empty state", async () => {
    portal.listVehicles.mockResolvedValue({
      items: [],
      total: 0,
      limit: 12,
      offset: 0,
    });
    await render(createElement(PortalVehicles));
    const empty = container.querySelector(
      '[data-testid="portal-vehicles-empty"]',
    );
    expect(empty?.textContent).toContain("portal.vehicles.empty_title");
    expect(
      container.querySelector('[data-testid="portal-vehicle"]'),
    ).toBeNull();
  });

  it("is right-to-left in Arabic", async () => {
    lang.locale = "ar";
    lang.dir = "rtl";
    portal.listVehicles.mockResolvedValue({
      items: [vehicle()],
      total: 1,
      limit: 12,
      offset: 0,
    });
    await render(createElement(PortalVehicles));
    const page = container.querySelector(
      '[data-testid="portal-vehicles-page"]',
    );
    expect(page?.getAttribute("dir")).toBe("rtl");
    expect(page?.getAttribute("lang")).toBe("ar");
    // The plate stays left-to-right inside the RTL page.
    expect(page?.querySelector('[dir="ltr"]')?.textContent).toBe("34 ABC 123");
  });
});

describe("PortalVehicleDetail", () => {
  it("shows the warranty progress percent of an active warranty", async () => {
    await render(
      createElement(PortalActiveWarrantyCard, {
        warranty: activeWarranty(),
        now: new Date("2026-07-02T12:00:00Z"),
      }),
    );
    const bar = container.querySelector('[data-testid="warranty-progress"]');
    // 182.5 of 365 days elapsed.
    expect(bar?.getAttribute("data-percent")).toBe("50");
    expect(bar?.getAttribute("data-days-left")).toBe("183");
    expect(
      container
        .querySelector('[role="progressbar"]')
        ?.getAttribute("aria-valuenow"),
    ).toBe("50");
    expect(
      container.querySelector('[data-testid="warranty-pdf-w-1"]'),
    ).not.toBeNull();
    const qr = container.querySelector('[data-testid="warranty-qr"]');
    expect(qr?.getAttribute("href")).toBe("/garanti/PUBCODE123456");
    expect(qr?.querySelector("img")?.getAttribute("src")).toBe(
      "data:image/png;base64,QR",
    );
  });

  it("renders the summary, warranties and service history", async () => {
    const detail: VehicleDetail = {
      ...vehicle(),
      service_summary: {
        total: 2,
        completed: 1,
        organization_count: 1,
        last_service_at: "2026-05-01T10:00:00Z",
      },
      services: [
        {
          uuid: "s-1",
          service_no: "SRV-0001",
          status: "completed",
          package: "Full PPF",
          organization: { uuid: "o-1", name: "Bayi Kadıköy", type: "dealer" },
          vehicle_uuid: "v-1",
          car_brand_name: "BMW",
          car_model_name: "320i",
          model_year: 2021,
          plate: "34 ABC 123",
          plate_country: "TR",
          completed_at: "2026-05-02T10:00:00Z",
          created_at: "2026-05-01T10:00:00Z",
        },
      ],
      active_warranties: [activeWarranty()],
    };
    portal.getVehicle.mockResolvedValue(detail);
    await render(createElement(PortalVehicleDetail, { uuid: "v-1" }));
    expect(portal.getVehicle).toHaveBeenCalledWith("v-1");
    expect(
      container.querySelectorAll('[data-testid="portal-active-warranty"]'),
    ).toHaveLength(1);
    const history = container.querySelector(
      '[data-testid="portal-vehicle-services"]',
    );
    expect(history?.querySelector("a")?.getAttribute("href")).toBe(
      "/portal/services/s-1",
    );
    expect(history?.textContent).toContain("services.status.completed");
    // TEC-243: the owner can start a transfer from the detail.
    expect(
      container.querySelector('[data-testid="portal-transfer-open"]'),
    ).not.toBeNull();
  });
});

describe("PortalServiceView", () => {
  function part(key: string) {
    return container.querySelector(`path[data-part="${key}"]`);
  }

  it("highlights the applied parts and focuses one product's parts", async () => {
    await render(createElement(PortalServiceView, { service: service() }));
    expect(part("body_kaput")?.getAttribute("data-highlighted")).toBe("true");
    expect(part("window_on_cam")?.getAttribute("data-highlighted")).toBe(
      "true",
    );
    expect(part("body_bagaj")?.getAttribute("data-applied")).toBe("false");
    expect(part("body_bagaj")?.getAttribute("data-highlighted")).toBe("false");

    await click(container.querySelector('button[data-product="i-2"]'));
    expect(
      container
        .querySelector('button[data-product="i-2"]')
        ?.getAttribute("aria-pressed"),
    ).toBe("true");
    expect(part("window_on_cam")?.getAttribute("data-highlighted")).toBe(
      "true",
    );
    // Still applied, but no longer highlighted.
    expect(part("body_kaput")?.getAttribute("data-applied")).toBe("true");
    expect(part("body_kaput")?.getAttribute("data-highlighted")).toBe("false");

    // A second tap clears the focus.
    await click(container.querySelector('button[data-product="i-2"]'));
    expect(part("body_kaput")?.getAttribute("data-highlighted")).toBe("true");
  });

  it("shows the dealer card with a WhatsApp link and the PDF button", async () => {
    await render(createElement(PortalServiceView, { service: service() }));
    const dealer = container.querySelector('[data-testid="portal-dealer"]');
    expect(dealer?.textContent).toContain("Bayi Kadıköy");
    expect(
      dealer
        ?.querySelector('[data-testid="dealer-whatsapp"]')
        ?.getAttribute("href"),
    ).toBe("https://wa.me/905321234567");
    expect(
      container.querySelector('[data-testid="service-pdf"]'),
    ).not.toBeNull();
  });

  it("hides WhatsApp when the dealer has no phone", async () => {
    await render(
      createElement(PortalServiceView, {
        service: service({
          dealer: { ...service().dealer, whatsapp: null },
        }),
      }),
    );
    expect(
      container.querySelector('[data-testid="dealer-whatsapp"]'),
    ).toBeNull();
  });
});
