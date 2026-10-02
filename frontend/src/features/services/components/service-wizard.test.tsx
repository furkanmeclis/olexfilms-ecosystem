// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  listCustomers: vi.fn(),
  createCustomer: vi.fn(),
  listVehicles: vi.fn(),
  createVehicle: vi.fn(),
  createService: vi.fn(),
  updateService: vi.fn(),
  getService: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
  }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  },
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock(
  "@/features/services/services/service-wizard.service",
  async (orig) => ({
    ...(await orig<object>()),
    serviceWizardService: api,
  }),
);
vi.mock("@/features/vehicle-catalog/services/vehicle-catalog.service", () => ({
  vehicleCatalogService: {
    listBrands: () =>
      Promise.resolve({ items: [], total: 0, limit: 20, offset: 0 }),
    listModels: () =>
      Promise.resolve({ items: [], total: 0, limit: 50, offset: 0 }),
  },
}));

import type {
  Service,
  Vehicle,
} from "@/features/services/services/service-wizard.service";
import { ApiError } from "@/lib/api/errors";

import { CustomerVehicleStep } from "./customer-vehicle-step";
import { MeasurementStep } from "./measurement-step";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
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

async function type(el: Element | null, value: string) {
  if (!(el instanceof HTMLInputElement)) throw new Error("input not found");
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  await act(async () => {
    setter?.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await flush();
}

async function click(el: Element | null | undefined) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
  await flush();
}

const $ = (sel: string) => container.querySelector(sel);
const $$ = (sel: string) => Array.from(container.querySelectorAll(sel));

const fullAccess = {
  canStart: true,
  canCreateCustomer: true,
  canCreateVehicle: true,
};

const customer = {
  uuid: "c1",
  name: "Ayşe",
  surname: "Yılmaz",
  phone: "+905551234567",
};

const vehicle = (over: Partial<Vehicle> = {}): Vehicle => ({
  uuid: "v1",
  customer_uuid: "c1",
  organization_uuid: "o1",
  plate: "34 ABC 123",
  plate_normalized: "34ABC123",
  plate_country: "TR",
  vin: null,
  model_year: 2022,
  car_brand: { uuid: "b1", name: "BMW" },
  car_model: { uuid: "m1", name: "X5" },
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
  warnings: [],
  ...over,
});

const service = (over: Partial<Service> = {}): Service => ({
  uuid: "s1",
  service_no: "DSAB12CD34",
  status: "draft",
  status_label: "Draft",
  organization: { uuid: "o1", name: "Bayi", type: "dealer" },
  customer: { ...customer, anonymized: false },
  vehicle_uuid: "v1",
  car_brand: { uuid: "b1", name: "BMW" },
  car_model: { uuid: "m1", name: "X5" },
  model_year: 2022,
  plate: "34 ABC 123",
  plate_country: "TR",
  vin: null,
  km: null,
  package: null,
  notes: null,
  has_measurement: false,
  cancel_reason: null,
  completed_at: null,
  cancelled_at: null,
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
  editable: true,
  items_editable: true,
  available_transitions: [],
  ...over,
});

describe("Step 1: customer and vehicle", () => {
  it("picks an existing customer and vehicle and opens the draft", async () => {
    api.listCustomers.mockResolvedValue({
      items: [customer],
      total: 1,
      limit: 10,
      offset: 0,
    });
    api.listVehicles.mockResolvedValue({
      items: [
        vehicle(),
        vehicle({ uuid: "v2", car_brand: null, car_model: null }),
      ],
      total: 2,
      limit: 50,
      offset: 0,
    });
    const created = service({ km: 12500 });
    api.createService.mockResolvedValue(created);
    const onDone = vi.fn();

    await render(
      createElement(CustomerVehicleStep, { access: fullAccess, onDone }),
    );
    const continueBtn = () =>
      $("[data-testid=step1-continue]") as HTMLButtonElement;
    expect(continueBtn().disabled).toBe(true);

    await type($("input[name=customer-search]"), "Ay");
    expect(api.listCustomers).toHaveBeenCalledWith({ q: "Ay" });
    await click($("[data-testid=customer-option]"));
    expect($("[data-testid=picked-customer]")?.textContent).toContain(
      "Ayşe Yılmaz",
    );
    expect(api.listVehicles).toHaveBeenCalledWith("c1");

    const options = $$("[data-testid=vehicle-option]") as HTMLButtonElement[];
    expect(options).toHaveLength(2);
    // The brand logo comes from the fixed public URL (TEC-150).
    const logo = options[0].querySelector("img[data-slot=vehicle-brand-logo]");
    expect(logo?.getAttribute("src")).toContain("/brand-logos/b1");
    // A vehicle without brand/model cannot carry a service.
    expect(options[1].disabled).toBe(true);
    expect(options[1].textContent).toContain("services.vehicle.needs_model");

    await click(options[0]);
    expect(options[0].getAttribute("aria-checked")).toBe("true");
    await type($("input[name=km]"), "12500");
    expect(continueBtn().disabled).toBe(false);
    await click(continueBtn());

    expect(api.createService).toHaveBeenCalledWith({
      customer_uuid: "c1",
      vehicle_uuid: "v1",
      has_measurement: false,
      km: 12500,
    });
    expect(onDone).toHaveBeenCalledWith(created);
  });

  it("rejects an invalid km before calling the API", async () => {
    api.listCustomers.mockResolvedValue({
      items: [customer],
      total: 1,
      limit: 10,
      offset: 0,
    });
    api.listVehicles.mockResolvedValue({
      items: [vehicle()],
      total: 1,
      limit: 50,
      offset: 0,
    });
    await render(
      createElement(CustomerVehicleStep, {
        access: fullAccess,
        onDone: vi.fn(),
      }),
    );
    await type($("input[name=customer-search]"), "Ayşe");
    await click($("[data-testid=customer-option]"));
    await click($("[data-testid=vehicle-option]"));
    await type($("input[name=km]"), "-5");
    await click($("[data-testid=step1-continue]"));
    expect(container.textContent).toContain("services.wizard.km_invalid");
    expect(api.createService).not.toHaveBeenCalled();
  });

  it("creates a new customer and continues with the vehicle list", async () => {
    api.createCustomer.mockResolvedValue({
      ...customer,
      uuid: "c9",
      existing_user: false,
    });
    api.listVehicles.mockResolvedValue({
      items: [],
      total: 0,
      limit: 50,
      offset: 0,
    });
    await render(
      createElement(CustomerVehicleStep, {
        access: fullAccess,
        onDone: vi.fn(),
      }),
    );
    await click($("[data-testid=new-customer-toggle]"));
    await click($("[data-testid=new-customer-submit]"));
    expect(container.textContent).toContain("services.customer.phone_required");
    expect(api.createCustomer).not.toHaveBeenCalled();

    await type($("#new-customer-phone"), "0555 123 45 67");
    await type($("#new-customer-name"), "Ayşe");
    await click($("[data-testid=new-customer-submit]"));
    expect(api.createCustomer).toHaveBeenCalledWith({
      phone: "0555 123 45 67",
      name: "Ayşe",
    });
    expect($("[data-testid=picked-customer]")).not.toBeNull();
    expect(api.listVehicles).toHaveBeenCalledWith("c9");
    expect(container.textContent).toContain("services.vehicle.none");
  });

  it("hides the create buttons without the write grants", async () => {
    await render(
      createElement(CustomerVehicleStep, {
        access: {
          canStart: true,
          canCreateCustomer: false,
          canCreateVehicle: false,
        },
        onDone: vi.fn(),
      }),
    );
    expect($("[data-testid=new-customer-toggle]")).toBeNull();
  });

  it("new vehicle form rejects an invalid VIN", async () => {
    api.listCustomers.mockResolvedValue({
      items: [customer],
      total: 1,
      limit: 10,
      offset: 0,
    });
    api.listVehicles.mockResolvedValue({
      items: [],
      total: 0,
      limit: 50,
      offset: 0,
    });
    await render(
      createElement(CustomerVehicleStep, {
        access: fullAccess,
        onDone: vi.fn(),
      }),
    );
    await type($("input[name=customer-search]"), "Ayşe");
    await click($("[data-testid=customer-option]"));
    await click($("[data-testid=new-vehicle-toggle]"));
    const form = $("[data-testid=new-vehicle-form]");
    await type(
      form?.querySelector("input[name=vin]") ?? null,
      "WVWZZZ1JZ3W38675O",
    );
    expect(form?.textContent).toContain(
      "services.vin.errors.forbidden_letters",
    );
    await click($("[data-testid=new-vehicle-submit]"));
    expect(container.textContent).toContain("services.vehicle.brand_required");
    expect(api.createVehicle).not.toHaveBeenCalled();
  });

  it("shows the draft's customer and vehicle read-only", async () => {
    const draft = service({ km: 100 });
    api.updateService.mockResolvedValue(draft);
    const onDone = vi.fn();
    await render(
      createElement(CustomerVehicleStep, {
        access: fullAccess,
        service: draft,
        onDone,
      }),
    );
    expect($("[data-testid=locked-customer]")?.textContent).toContain("Ayşe");
    expect($("input[name=customer-search]")).toBeNull();
    expect(
      $("img[data-slot=vehicle-brand-logo]")?.getAttribute("src"),
    ).toContain("/brand-logos/b1");
    await type($("input[name=km]"), "250");
    await click($("[data-testid=step1-continue]"));
    expect(api.updateService).toHaveBeenCalledWith("s1", { km: 250 });
    expect(onDone).toHaveBeenCalled();
  });
});

describe("Step 3: measurement and VIN", () => {
  it("requires a valid VIN when there is a measurement", async () => {
    const onSaved = vi.fn();
    await render(
      createElement(MeasurementStep, {
        service: service(),
        onSaved,
        onBack: vi.fn(),
      }),
    );
    await click($("[data-testid=measurement-yes]"));
    expect(
      $("[data-testid=measurement-yes]")?.getAttribute("aria-pressed"),
    ).toBe("true");

    await click($("[data-testid=measurement-save]"));
    expect($("[data-testid=vin-error]")?.textContent).toBe(
      "services.vin.errors.required",
    );

    await type($("input[name=vin]"), "WVWZZZ1JZ3W3867");
    expect($("[data-testid=vin-error]")?.textContent).toBe(
      "services.vin.errors.length",
    );
    await type($("input[name=vin]"), "WVWZZZ1JZ3W38675Q");
    expect($("[data-testid=vin-error]")?.textContent).toBe(
      "services.vin.errors.forbidden_letters",
    );
    await click($("[data-testid=measurement-save]"));
    expect(api.updateService).not.toHaveBeenCalled();
  });

  it("saves a valid VIN normalized with the measurement answer", async () => {
    const saved = service({ has_measurement: true, vin: "WVWZZZ1JZ3W386752" });
    api.updateService.mockResolvedValue(saved);
    const onSaved = vi.fn();
    await render(
      createElement(MeasurementStep, {
        service: service(),
        onSaved,
        onBack: vi.fn(),
      }),
    );
    await click($("[data-testid=measurement-yes]"));
    await type($("input[name=vin]"), "wvw-zzz 1jz3w386752");
    expect($("[data-testid=vin-error]")).toBeNull();
    await click($("[data-testid=measurement-save]"));
    expect(api.updateService).toHaveBeenCalledWith("s1", {
      vin: "WVWZZZ1JZ3W386752",
      has_measurement: true,
    });
    expect(onSaved).toHaveBeenCalledWith(saved);
  });

  it("allows no measurement without a VIN", async () => {
    api.updateService.mockResolvedValue(service());
    await render(
      createElement(MeasurementStep, {
        service: service({ has_measurement: true, vin: "WVWZZZ1JZ3W386752" }),
        onSaved: vi.fn(),
        onBack: vi.fn(),
      }),
    );
    await click($("[data-testid=measurement-no]"));
    await type($("input[name=vin]"), "");
    await click($("[data-testid=measurement-save]"));
    expect(api.updateService).toHaveBeenCalledWith("s1", {
      vin: null,
      has_measurement: false,
    });
  });

  it("shows the server rejection of the VIN", async () => {
    api.updateService.mockRejectedValue(
      new ApiError({
        status: 400,
        code: "VALIDATION_ERROR",
        message: "invalid",
        details: [{ field: "vin", message: "must be 17 letters" }],
      }),
    );
    const onSaved = vi.fn();
    await render(
      createElement(MeasurementStep, {
        service: service(),
        onSaved,
        onBack: vi.fn(),
      }),
    );
    await type($("input[name=vin]"), "WVWZZZ1JZ3W386752");
    await click($("[data-testid=measurement-save]"));
    expect($("[data-testid=vin-error]")?.textContent).toBe(
      "services.vin.errors.server",
    );
    expect(onSaved).not.toHaveBeenCalled();
  });
});
