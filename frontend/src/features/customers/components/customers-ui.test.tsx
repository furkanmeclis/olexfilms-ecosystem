// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  upgrade: vi.fn(),
  anonymize: vi.fn(),
  requestDataExport: vi.fn(),
  getDataExport: vi.fn(),
  downloadDataExport: vi.fn(),
  requestListExport: vi.fn(),
  getListExport: vi.fn(),
  downloadListExport: vi.fn(),
  listDealers: vi.fn(),
  listVehicles: vi.fn(),
  createVehicle: vi.fn(),
  updateVehicle: vi.fn(),
}));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "center" as string | null,
}));
const nav = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
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
vi.mock("next/navigation", () => ({ useRouter: () => nav }));
vi.mock("sonner", () => ({ toast }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "en",
    format: {
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
    state.orgType ? { slug: "acme", type: state.orgType } : null,
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/features/geo/hooks/use-geo", () => ({
  usePlateFormats: () => ({
    data: [
      {
        country_iso2: "TR",
        country_name_en: "Turkey",
        regex: "^(0[1-9]|[1-7][0-9]|8[01])[A-Z]{1,3}[0-9]{2,5}$",
        input_mask: "99 AAA 9999",
        example: "34 ABC 123",
        country_label: "TR",
        strip_color: "#003399",
        background_color: "#FFFFFF",
        text_color: "#000000",
      },
    ],
  }),
}));
vi.mock("@/features/vehicle-catalog/services/vehicle-catalog.service", () => ({
  vehicleCatalogService: {
    listBrands: vi.fn(async () => ({ items: [], total: 0 })),
    listModels: vi.fn(async () => ({ items: [], total: 0 })),
  },
}));
vi.mock("@/features/customers/services/customers.service", async (orig) => ({
  ...(await orig<object>()),
  customersService: api,
}));

import { Permission } from "@/config/permissions";
import type {
  CustomerDetail,
  Vehicle,
} from "@/features/customers/services/customers.service";

import { CustomerDetailPage } from "./customer-detail-page";
import { CustomerForm } from "./customer-form-page";
import { CustomersListPage } from "./customers-list-page";
import { VehicleForm } from "./vehicle-form-dialog";

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
  document.body.innerHTML = "";
  vi.clearAllMocks();
  state.grants = new Set();
  state.orgType = "center";
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

const $ = (sel: string) => document.querySelector(sel);
const actions = () =>
  Array.from(document.querySelectorAll("[data-action]")).map((b) =>
    b.getAttribute("data-action"),
  );

function customer(over: Partial<CustomerDetail> = {}): CustomerDetail {
  return {
    uuid: "c1",
    name: "Ayşe",
    surname: "Yılmaz",
    email: "ayse@example.com",
    phone: "+905551234567",
    status: "active",
    anonymized: false,
    type: "individual",
    company_name: null,
    locale: "tr",
    created_at: "2026-09-01T09:00:00Z",
    linked_at: "2026-09-01T09:00:00Z",
    first_service_at: null,
    tax_office: null,
    national_id_last4: "8901",
    tax_no_last4: null,
    address: {},
    notification_prefs: {},
    editable: true,
    identity_editable: true,
    organizations: [
      {
        uuid: "o1",
        name: "Olex Bayi",
        type: "dealer",
        linked_at: "2026-09-01T09:00:00Z",
        first_service_at: null,
      },
    ],
    ...over,
  };
}

const vehicle: Vehicle = {
  uuid: "v1",
  customer_uuid: "c1",
  organization_uuid: "o1",
  plate: "34 ABC 123",
  plate_normalized: "34ABC123",
  plate_country: "TR",
  vin: null,
  model_year: 2022,
  car_brand: { uuid: "b1", name: "BMW" },
  car_model: { uuid: "m1", name: "320i" },
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
  warnings: [],
};

describe("customers list (TEC-163)", () => {
  it("is forbidden without customers.read", async () => {
    await render(createElement(CustomersListPage, { slug: "acme" }));
    expect(api.list).not.toHaveBeenCalled();
    expect(container.textContent).toContain("customers.list.forbidden");
  });

  it("searches with q, filters status and pages", async () => {
    state.grants = new Set([Permission.CustomersRead]);
    api.list.mockResolvedValue({
      items: [customer()],
      total: 45,
      limit: 20,
      offset: 0,
    });
    await render(createElement(CustomersListPage, { slug: "acme" }));
    expect(api.list).toHaveBeenLastCalledWith({ limit: 20, offset: 0 });
    expect($("[data-testid=new-customer]")).toBeNull();
    expect(
      container.querySelectorAll("[data-testid=customer-row]"),
    ).toHaveLength(1);
    expect(container.textContent).toContain("Ayşe Yılmaz");

    await type($("#customer-search"), "ayşe");
    expect(api.list).toHaveBeenLastCalledWith({
      q: "ayşe",
      limit: 20,
      offset: 0,
    });
    await click($("[data-status=anonymized]"));
    expect(api.list).toHaveBeenLastCalledWith({
      q: "ayşe",
      status: "anonymized",
      limit: 20,
      offset: 0,
    });
    await click($("[data-testid=page-next]"));
    expect(api.list).toHaveBeenLastCalledWith({
      q: "ayşe",
      status: "anonymized",
      limit: 20,
      offset: 20,
    });
  });

  it("shows the new customer button with customers.write", async () => {
    state.grants = new Set([
      Permission.CustomersRead,
      Permission.CustomersWrite,
    ]);
    api.list.mockResolvedValue({ items: [], total: 0, limit: 20, offset: 0 });
    await render(createElement(CustomersListPage, { slug: "acme" }));
    expect($("[data-testid=new-customer]")?.getAttribute("href")).toBe(
      "/t/acme/customers/new",
    );
    expect($("[data-testid=customers-empty]")).not.toBeNull();
  });
});

describe("customer list export (TEC-199)", () => {
  const job = {
    uuid: "le1",
    resource: "customers",
    format: "csv",
    row_count: 3,
    created_at: "2026-10-02T09:00:00Z",
  };

  it("is hidden without customers.read", async () => {
    state.grants = new Set([Permission.CustomersWrite]);
    await render(createElement(CustomersListPage, { slug: "acme" }));
    expect($("[data-testid=customer-list-export]")).toBeNull();
  });

  it("is shown with customers.read", async () => {
    state.grants = new Set([Permission.CustomersRead]);
    api.list.mockResolvedValue({ items: [], total: 0, limit: 20, offset: 0 });
    await render(createElement(CustomersListPage, { slug: "acme" }));
    expect($("[data-testid=customer-list-export]")).not.toBeNull();
  });

  it("queues the job with the list filters and downloads once ready", async () => {
    state.grants = new Set([Permission.CustomersRead]);
    api.list.mockResolvedValue({
      items: [customer()],
      total: 1,
      limit: 20,
      offset: 0,
    });
    api.requestListExport.mockResolvedValue({ ...job, status: "queued" });
    api.getListExport.mockResolvedValue({ ...job, status: "completed" });
    await render(createElement(CustomersListPage, { slug: "acme" }));
    await type($("#customer-search"), "ayşe");
    await click($("[data-status=active]"));

    await click($("[data-testid=customer-list-export]"));
    const select = $("#list-export-format");
    if (!(select instanceof HTMLSelectElement)) throw new Error("no select");
    expect(Array.from(select.options).map((o) => o.value)).toEqual([
      "csv",
      "xlsx",
      "pdf",
    ]);
    await act(async () => {
      select.value = "csv";
      select.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await click($("[data-testid=list-export-confirm]"));
    expect(api.requestListExport).toHaveBeenCalledWith("csv", {
      q: "ayşe",
      status: "active",
    });
    expect(api.getListExport).toHaveBeenCalledWith("le1");
    expect(
      $("[data-testid=list-export-status]")?.getAttribute("data-status"),
    ).toBe("completed");
    await click($("[data-testid=list-export-download]"));
    expect(api.downloadListExport).toHaveBeenCalledWith({
      ...job,
      status: "completed",
    });
  });

  it("keeps polling while the job is processing", async () => {
    state.grants = new Set([Permission.CustomersRead]);
    api.list.mockResolvedValue({ items: [], total: 0, limit: 20, offset: 0 });
    api.requestListExport.mockResolvedValue({ ...job, status: "queued" });
    api.getListExport.mockResolvedValue({ ...job, status: "processing" });
    await render(createElement(CustomersListPage, { slug: "acme" }));
    await click($("[data-testid=customer-list-export]"));
    await click($("[data-testid=list-export-confirm]"));
    expect(api.requestListExport).toHaveBeenCalledWith("xlsx", {});
    expect(
      $("[data-testid=list-export-status]")?.getAttribute("data-status"),
    ).toBe("processing");
    expect($("[data-testid=list-export-download]")).toBeNull();
    expect(
      ($("[data-testid=list-export-confirm]") as HTMLButtonElement).disabled,
    ).toBe(true);
  });
});

describe("customer detail actions by permission", () => {
  beforeEach(() => {
    api.listVehicles.mockResolvedValue({
      items: [vehicle],
      total: 1,
      limit: 100,
      offset: 0,
    });
  });

  it("read-only user sees no actions", async () => {
    state.grants = new Set([Permission.CustomersRead]);
    api.get.mockResolvedValue(customer());
    await render(
      createElement(CustomerDetailPage, { slug: "acme", uuid: "c1" }),
    );
    expect(actions()).toEqual([]);
    expect(api.listVehicles).not.toHaveBeenCalled();
  });

  it("center admin sees edit, upgrade, export, anonymize and vehicle writes", async () => {
    state.grants = new Set([
      Permission.CustomersRead,
      Permission.CustomersWrite,
      Permission.CustomersAnonymize,
      Permission.OrganizationsRead,
      Permission.VehiclesRead,
      Permission.VehiclesWrite,
    ]);
    api.get.mockResolvedValue(customer());
    await render(
      createElement(CustomerDetailPage, { slug: "acme", uuid: "c1" }),
    );
    expect(actions()).toEqual([
      "edit",
      "upgrade",
      "export",
      "anonymize",
      "add-vehicle",
      "edit-vehicle",
    ]);
    expect(container.textContent).toContain("34 ABC 123");
    expect(container.textContent).toContain("•••• 8901");
  });

  it("dealer: no privacy actions and no upgrade", async () => {
    state.orgType = "dealer";
    state.grants = new Set([
      Permission.CustomersRead,
      Permission.CustomersWrite,
      Permission.CustomersAnonymize,
      Permission.OrganizationsRead,
    ]);
    api.get.mockResolvedValue(customer());
    await render(
      createElement(CustomerDetailPage, { slug: "acme", uuid: "c1" }),
    );
    expect(actions()).toEqual(["edit"]);
  });

  it("anonymized customer: no actions, notice shown", async () => {
    state.grants = new Set([
      Permission.CustomersRead,
      Permission.CustomersWrite,
      Permission.CustomersAnonymize,
      Permission.OrganizationsRead,
    ]);
    api.get.mockResolvedValue(
      customer({ anonymized: true, status: "anonymized", editable: false }),
    );
    await render(
      createElement(CustomerDetailPage, { slug: "acme", uuid: "c1" }),
    );
    expect(actions()).toEqual([]);
    expect($("[data-testid=anonymized-notice]")).not.toBeNull();
  });

  it("anonymize needs the typed confirmation word", async () => {
    state.grants = new Set([
      Permission.CustomersRead,
      Permission.CustomersAnonymize,
    ]);
    api.get.mockResolvedValue(customer());
    api.anonymize.mockResolvedValue({
      uuid: "c1",
      status: "anonymized",
      changed: true,
      anonymized_at: "2026-10-02T09:00:00Z",
    });
    await render(
      createElement(CustomerDetailPage, { slug: "acme", uuid: "c1" }),
    );
    await click($("[data-action=anonymize]"));
    const confirm = $("[data-testid=anonymize-confirm-button]");
    expect((confirm as HTMLButtonElement).disabled).toBe(true);
    await type($("#anonymize-confirm"), "anonymize");
    expect((confirm as HTMLButtonElement).disabled).toBe(false);
    await click(confirm);
    expect(api.anonymize).toHaveBeenCalledWith("c1");
    expect(toast.success).toHaveBeenCalledWith(
      "customers.actions.anonymize.success",
    );
  });

  it("data export queues a job and offers the download once completed", async () => {
    state.grants = new Set([
      Permission.CustomersRead,
      Permission.CustomersAnonymize,
    ]);
    api.get.mockResolvedValue(customer());
    const job = {
      uuid: "j1",
      resource: "customer_data",
      format: "pdf",
      row_count: 1,
      created_at: "2026-10-02T09:00:00Z",
    };
    api.requestDataExport.mockResolvedValue({ ...job, status: "queued" });
    api.getDataExport.mockResolvedValue({ ...job, status: "completed" });
    await render(
      createElement(CustomerDetailPage, { slug: "acme", uuid: "c1" }),
    );
    await click($("[data-action=export]"));
    await click($("[data-testid=export-confirm]"));
    expect(api.requestDataExport).toHaveBeenCalledWith("c1", "pdf");
    expect($("[data-testid=export-status]")?.getAttribute("data-status")).toBe(
      "completed",
    );
    await click($("[data-testid=export-download]"));
    expect(api.downloadDataExport).toHaveBeenCalled();
  });
});

describe("customer form validation", () => {
  it("blocks submit without phone and name", async () => {
    await render(createElement(CustomerForm, { slug: "acme", customer: null }));
    await click($("[data-testid=customer-submit]"));
    expect(api.create).not.toHaveBeenCalled();
    expect($("[data-error=phone]")?.textContent).toContain(
      "customers.validation.phone_required",
    );
    expect($("[data-error=name]")?.textContent).toContain(
      "customers.validation.name_required",
    );
  });

  it("corporate needs a company name; valid form creates", async () => {
    api.create.mockResolvedValue({
      ...customer(),
      existing_user: true,
      ignored_fields: [],
    });
    await render(createElement(CustomerForm, { slug: "acme", customer: null }));
    await type($("#customer-phone"), "0555 123 45 67");
    await type($("#customer-name"), "Ayşe");
    await click($("[data-type=corporate]"));
    await click($("[data-testid=customer-submit]"));
    expect($("[data-error=company_name]")).not.toBeNull();
    await type($("#customer-company_name"), "Acme Ltd");
    await click($("[data-testid=customer-submit]"));
    expect(api.create).toHaveBeenCalledWith({
      phone: "0555 123 45 67",
      name: "Ayşe",
      type: "corporate",
      company_name: "Acme Ltd",
    });
    expect(toast.success).toHaveBeenCalledWith(
      "customers.form.linked_existing",
    );
    expect(nav.push).toHaveBeenCalledWith("/t/acme/customers/c1");
  });

  it("edit sends only changed fields and locks the phone", async () => {
    api.update.mockResolvedValue({
      ...customer({ surname: "" }),
      existing_user: false,
      ignored_fields: ["email"],
    });
    await render(
      createElement(CustomerForm, {
        slug: "acme",
        customer: customer({ identity_editable: false }),
      }),
    );
    expect(($("#customer-phone") as HTMLInputElement).disabled).toBe(true);
    expect($("[data-testid=fill-only-notice]")).not.toBeNull();
    await type($("#customer-surname"), "");
    await click($("[data-testid=customer-submit]"));
    expect(api.update).toHaveBeenCalledWith("c1", { surname: null });
    expect(toast.warning).toHaveBeenCalled();
  });
});

describe("vehicle form: plate format", () => {
  it("rejects a plate that does not match the country format", async () => {
    await render(
      createElement(VehicleForm, {
        customerUuid: "c1",
        vehicle: null,
        onCancel: () => {},
        onSaved: () => {},
      }),
    );
    expect(($("#vehicle-plate-country") as HTMLSelectElement).value).toBe("TR");
    await type($("#vehicle-plate"), "99 ABC 123");
    await click($("[data-testid=vehicle-submit]"));
    expect(api.createVehicle).not.toHaveBeenCalled();
    expect($("[data-error=plate]")?.textContent).toContain(
      "customers.validation.plate_invalid",
    );
  });

  it("creates a vehicle with a valid TR plate", async () => {
    const onSaved = vi.fn();
    api.createVehicle.mockResolvedValue(vehicle);
    await render(
      createElement(VehicleForm, {
        customerUuid: "c1",
        vehicle: null,
        onCancel: () => {},
        onSaved,
      }),
    );
    await type($("#vehicle-plate"), "34 abc 123");
    await type($("#vehicle-year"), "2022");
    await click($("[data-testid=vehicle-submit]"));
    expect(api.createVehicle).toHaveBeenCalledWith({
      customer_uuid: "c1",
      plate: "34 abc 123",
      plate_country: "TR",
      car_brand_uuid: null,
      car_model_uuid: null,
      model_year: 2022,
    });
    expect(onSaved).toHaveBeenCalledWith(vehicle);
  });

  it("edits only the changed fields of a stored vehicle", async () => {
    api.updateVehicle.mockResolvedValue(vehicle);
    await render(
      createElement(VehicleForm, {
        customerUuid: "c1",
        vehicle,
        onCancel: () => {},
        onSaved: () => {},
      }),
    );
    await type($("#vehicle-year"), "2023");
    await click($("[data-testid=vehicle-submit]"));
    expect(api.updateVehicle).toHaveBeenCalledWith("v1", { model_year: 2023 });
  });
});
