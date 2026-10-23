// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  grants: new Set<string>(),
  push: vi.fn(),
  getVehicle: vi.fn(),
  listServices: vi.fn(),
  listWarranties: vi.fn(),
  listClaims: vi.fn(),
  getCustomer: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: api.push, back: vi.fn() }),
}));
vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: { href: string; children: ReactNode } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "tr",
    format: {
      number: (v: number) => String(v),
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => api.grants.has(p) }),
}));
vi.mock("@/hooks/use-mobile", () => ({ useIsMobile: () => false }));
vi.mock("@/features/vehicles/components/vehicle-transfer-card", () => ({
  VehicleTransferCard: () => createElement("div", null, "transfer-card"),
}));
vi.mock(
  "@/features/vehicles/services/vehicle-transfer.service",
  async (orig) => ({
    ...(await orig<object>()),
    vehicleTransferService: { getVehicle: api.getVehicle },
  }),
);
vi.mock(
  "@/features/services/services/service-wizard.service",
  async (orig) => ({
    ...(await orig<object>()),
    serviceWizardService: { listServices: api.listServices },
  }),
);
vi.mock("@/features/warranty/services/warranty.service", async (orig) => ({
  ...(await orig<object>()),
  warrantyService: { list: api.listWarranties },
}));
vi.mock("@/features/warranty-claims/services/claims.service", async (orig) => ({
  ...(await orig<object>()),
  claimsService: { list: api.listClaims },
}));
vi.mock("@/features/customers/services/customers.service", async (orig) => ({
  ...(await orig<object>()),
  customersService: { get: api.getCustomer },
}));

import { Permission } from "@/config/permissions";
import { installRadixPolyfills } from "@/test/form-controls";

import { VehicleDetailPage } from "./vehicle-detail-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
installRadixPolyfills();

let container: HTMLDivElement;
let root: Root;

const vehicle = {
  uuid: "veh-1",
  customer_uuid: "cus-1",
  organization_uuid: "org-1",
  plate: "34ABC123",
  plate_normalized: "34ABC123",
  plate_country: "TR",
  vin: "WBA00000000000001",
  model_year: 2024,
  car_brand: { uuid: "b-1", name: "BMW" },
  car_model: { uuid: "m-1", name: "X5" },
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  warnings: [],
};

const service = {
  uuid: "srv-1",
  service_no: "DS12345678",
  status: "completed",
  status_label: "Tamamlandı",
  package: "Full PPF",
  created_at: "2026-02-01T00:00:00Z",
  completed_at: "2026-02-03T00:00:00Z",
  updated_at: "2026-02-03T00:00:00Z",
  organization: { uuid: "org-1", name: "Bayi A" },
  customer: {
    uuid: "cus-1",
    name: "Ayşe",
    surname: "Kaya",
    phone: "+905551112233",
    anonymized: false,
  },
};

const warranty = {
  uuid: "war-1",
  public_code: "PUB123",
  status: "active",
  item_kind: "full",
  start_at: "2026-02-03T00:00:00Z",
  end_at: "2031-02-03T00:00:00Z",
  created_at: "2026-02-03T00:00:00Z",
  product: { uuid: "p-1", sku: "PPF", name: "Film PPF" },
  service: { uuid: "srv-1", service_no: "DS12345678" },
  organization: { uuid: "org-1", name: "Bayi A", type: "dealer" },
  vehicle: {
    uuid: "veh-1",
    brand_name: "BMW",
    model_name: "X5",
    model_year: 2024,
    plate: "34ABC123",
  },
  holder: { uuid: "cus-1", name: "Ayşe", surname: "Kaya", anonymized: false },
  can_void: false,
};

const page = <T,>(items: T[], total = items.length) => ({
  items,
  total,
  limit: 100,
  offset: 0,
});

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  api.getVehicle.mockResolvedValue(vehicle);
  api.listServices.mockResolvedValue(page([service], 1));
  api.listWarranties.mockResolvedValue(page([warranty]));
  api.listClaims.mockResolvedValue(page([]));
  api.getCustomer.mockResolvedValue({
    uuid: "cus-1",
    name: "Ayşe",
    surname: "Kaya",
    company_name: null,
    phone: "+905551112233",
    email: "ayse@example.com",
  });
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
  api.grants = new Set();
});

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(VehicleDetailPage, { slug: "acme", uuid: "veh-1" }),
      ),
    );
  });
  for (let i = 0; i < 6; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

const q = (sel: string) => container.querySelector(sel);

describe("VehicleDetailPage (TEC-863 vehicle 360)", () => {
  it("shows the owner, KPIs and the vehicle's services", async () => {
    api.grants = new Set([
      Permission.VehiclesRead,
      Permission.ServicesRead,
      Permission.WarrantiesRead,
      Permission.WarrantyClaimsRead,
      Permission.CustomersRead,
    ]);
    await render();

    expect(q('[data-testid="vehicle-detail"]')).not.toBeNull();
    expect(api.listServices).toHaveBeenCalledWith(
      expect.objectContaining({ vehicle_uuid: "veh-1", limit: 100 }),
    );
    expect(api.listWarranties).toHaveBeenCalledWith(
      expect.objectContaining({ vehicle_uuid: "veh-1" }),
    );
    expect(api.listClaims).toHaveBeenCalledWith(
      expect.objectContaining({ vehicle_uuid: "veh-1" }),
    );
    expect(api.getCustomer).toHaveBeenCalledWith("cus-1");

    const owner = q('[data-testid="vehicle-owner"]')?.textContent ?? "";
    expect(owner).toContain("Ayşe Kaya");
    expect(owner).toContain("ayse@example.com");
    expect(q('[data-testid="vehicle-owner-link"]')?.getAttribute("href")).toBe(
      "/t/acme/customers/cus-1",
    );

    const kpis = q('[data-testid="vehicle-kpis"]')?.textContent ?? "";
    expect(kpis).toContain("vehicles.detail.kpi_services1");
    expect(kpis).toContain("date(2026-02-03T00:00:00Z)");
    expect(kpis).toContain("vehicles.detail.kpi_active_warranties1");
    expect(kpis).toContain("vehicles.detail.kpi_open_claims0");

    const row = q('[data-testid="vehicle-service-row"]');
    expect(row?.getAttribute("href")).toBe("/t/acme/services/srv-1");
    expect(q('[data-testid="vehicle-tab-ownership"]')).toBeNull();
  });

  it("falls back to the service customer without customers.read", async () => {
    api.grants = new Set([Permission.VehiclesRead, Permission.ServicesRead]);
    await render();

    expect(api.getCustomer).not.toHaveBeenCalled();
    expect(api.listWarranties).not.toHaveBeenCalled();
    expect(api.listClaims).not.toHaveBeenCalled();
    const owner = q('[data-testid="vehicle-owner"]')?.textContent ?? "";
    expect(owner).toContain("Ayşe Kaya");
    expect(owner).toContain("+905551112233");
    expect(q('[data-testid="vehicle-owner-link"]')).toBeNull();
    expect(q('[data-testid="vehicle-tab-warranties"]')).toBeNull();
  });

  it("hides the sections the user cannot read", async () => {
    api.grants = new Set([Permission.VehiclesRead]);
    await render();

    expect(q('[data-testid="vehicle-detail"]')).not.toBeNull();
    expect(api.listServices).not.toHaveBeenCalled();
    expect(q('[data-testid="vehicle-kpis"]')).toBeNull();
    expect(container.textContent).toContain("vehicles.detail.owner_unknown");
  });

  it("notes when only the first page of services is shown", async () => {
    api.grants = new Set([Permission.VehiclesRead, Permission.ServicesRead]);
    api.listServices.mockResolvedValue(page([service], 250));
    await render();

    expect(container.textContent).toContain(
      'vehicles.detail.truncated {"shown":1,"total":250}',
    );
  });
});
