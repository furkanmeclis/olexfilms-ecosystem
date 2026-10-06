// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  getService: vi.fn(),
}));
const state = vi.hoisted(() => ({ grants: new Set<string>() }));
const cert = vi.hoisted(() => ({
  request: vi.fn(),
  get: vi.fn(),
  download: vi.fn(),
  forService: vi.fn(),
  trigger: vi.fn(),
  toast: { success: vi.fn(), error: vi.fn() },
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
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), back: vi.fn() }),
}));
vi.mock("@/hooks/use-mobile", () => ({
  useIsMobile: () => false,
  useIsXl: () => true,
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/features/warranty/services/certificate.service", () => ({
  panelCertificateClient: (uuid: string) => {
    cert.forService(uuid);
    return { request: cert.request, get: cert.get, download: cert.download };
  },
}));
const pdf = vi.hoisted(() => ({
  request: vi.fn(),
  get: vi.fn(),
  download: vi.fn(),
  forService: vi.fn(),
}));
vi.mock("@/features/services/services/service-pdf.service", () => ({
  servicePdfClient: (uuid: string) => {
    pdf.forService(uuid);
    return { request: pdf.request, get: pdf.get, download: pdf.download };
  },
}));
vi.mock("@/lib/api/platform-form-request", async (orig) => ({
  ...(await orig<object>()),
  triggerBrowserDownload: cert.trigger,
}));
vi.mock("@/providers/toast-provider", () => ({ appToast: cert.toast }));
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

import type { Service } from "@/features/services/services/service-wizard.service";
import { ApiError } from "@/lib/api/errors";

import { ServiceDetailPage } from "./service-detail-page";

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
  state.grants = new Set();
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

const $ = (sel: string) => container.querySelector(sel);
const $$ = (sel: string) => Array.from(container.querySelectorAll(sel));

const wizardGrants = ["services.write", "customers.read", "vehicles.read"];

function service(over: Partial<Service> = {}): Service {
  return {
    uuid: "s1",
    service_no: "DSABCD1234",
    status: "completed",
    status_label: "Completed",
    organization: { uuid: "o1", name: "Acme Bayi", type: "dealer" },
    customer: {
      uuid: "c1",
      name: "Ayşe",
      surname: "Yılmaz",
      phone: "+905551234567",
      anonymized: false,
    },
    vehicle_uuid: "v1",
    car_brand: { uuid: "b1", name: "BMW" },
    car_model: { uuid: "m1", name: "320i" },
    model_year: 2022,
    plate: "34ABC123",
    plate_country: "TR",
    vin: "WVWZZZ1JZ3W386752",
    km: 12000,
    package: null,
    notes: null,
    has_measurement: false,
    contract: null,
    contract_required: false,
    cancel_reason: null,
    completed_at: "2026-10-01T10:00:00Z",
    cancelled_at: null,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T10:00:00Z",
    editable: false,
    items_editable: false,
    available_transitions: [],
    items: [
      {
        uuid: "i1",
        product: {
          uuid: "p1",
          sku: "PPF-190",
          name: "Olex PPF 190",
          unit_type: "roll_meter",
        },
        barcode: "OLX-ROLL-1",
        unit_kind: "roll",
        kind: "partial",
        quantity: null,
        meters: "4.50",
        applied_parts: ["body_kaput", "custom_part"],
        notes: null,
        created_at: "2026-10-01T09:00:00Z",
      },
    ],
    images: [
      {
        uuid: "im1",
        title: null,
        sort_order: 0,
        url: "/v1/services/s1/images/im1",
        created_at: "2026-10-01T09:00:00Z",
      },
    ],
    status_logs: [
      {
        from_status: null,
        to_status: "draft",
        note: null,
        by_other_organization: false,
        created_at: "2026-10-01T08:00:00Z",
      },
      {
        from_status: "draft",
        to_status: "completed",
        note: "Teslim edildi",
        by_other_organization: true,
        created_at: "2026-10-01T10:00:00Z",
      },
    ],
    warranties: [
      {
        uuid: "w1",
        public_code: "W-ABC123",
        service_item_uuid: "i1",
        product_name: "Olex PPF 190",
        item_kind: "partial",
        status: "active",
        start_at: "2026-10-01T10:00:00Z",
        end_at: "2036-10-01T20:59:59Z",
        expired_at: null,
        voided_at: null,
      },
    ],
    ...over,
  } as Service;
}

describe("ServiceDetailPage (TEC-183)", () => {
  it("is forbidden without services.read", async () => {
    await render(
      createElement(ServiceDetailPage, { slug: "acme", uuid: "s1" }),
    );
    expect(container.textContent).toContain("common.error_forbidden");
    expect(api.getService).not.toHaveBeenCalled();
  });

  it("shows vehicle, customer, items, images, warranties and history", async () => {
    state.grants = new Set(["services.read", ...wizardGrants]);
    api.getService.mockResolvedValue(service());
    await render(
      createElement(ServiceDetailPage, { slug: "acme", uuid: "s1" }),
    );
    expect(api.getService).toHaveBeenCalledWith("s1");
    expect($("h1")?.textContent).toContain("DSABCD1234");
    expect($("[data-testid=detail-status]")?.textContent).toBe("Completed");

    const vehicle = $("[data-testid=detail-vehicle]")?.textContent ?? "";
    expect(vehicle).toContain("BMW 320i 2022");
    expect(vehicle).toContain("34ABC123 (TR)");
    expect(vehicle).toContain("WVWZZZ1JZ3W386752");
    expect(vehicle).toContain("12000");
    const customer = $("[data-testid=detail-customer]")?.textContent ?? "";
    expect(customer).toContain("Ayşe Yılmaz");
    expect(customer).toContain("+905551234567");
    expect(customer).toContain("Acme Bayi");

    // TEC-378: items and warranties are nested client-side DataTables.
    expect($$("[data-testid=detail-item]")).toHaveLength(1);
    const item = $("[data-testid=detail-items] tbody tr")?.textContent ?? "";
    expect(item).toContain("Olex PPF 190");
    expect(item).toContain("OLX-ROLL-1");
    expect(item).toContain('services.stock.amount_meters {"meters":"4.50"}');
    expect(item).toContain("services.parts.names.body_kaput");
    expect(item).toContain("custom_part");

    expect($("[data-testid=detail-image]")?.getAttribute("src")).toBe(
      "/api/v1/services/s1/images/im1",
    );

    expect($("[data-testid=detail-warranty]")?.textContent).toBe("W-ABC123");
    const warranty =
      $("[data-testid=detail-warranties] tbody tr")?.textContent ?? "";
    expect(warranty).toContain("W-ABC123");
    expect(warranty).toContain("services.detail.warranty_status.active");
    expect(warranty).toContain("date(2036-10-01T20:59:59Z)");

    const logs = $$("[data-testid=detail-log]");
    expect(logs.map((l) => l.getAttribute("data-status"))).toEqual([
      "completed",
      "draft",
    ]);
    expect(logs[0].textContent).toContain("Teslim edildi");
    expect(logs[0].textContent).toContain("services.detail.history_other_org");

    // Completed: no wizard link; the service PDF (TEC-196) is available;
    // the vehicle page link needs vehicles.read.
    expect($("[data-testid=continue-wizard]")).toBeNull();
    expect($("[data-testid=service-pdf]")?.hasAttribute("disabled")).toBe(
      false,
    );
    expect($("[data-testid=detail-vehicle-link]")?.getAttribute("href")).toBe(
      "/t/acme/vehicles/v1",
    );
  });

  it("downloads the service PDF (TEC-196)", async () => {
    state.grants = new Set(["services.read"]);
    const svc = service();
    api.getService.mockResolvedValue(svc);
    const done = {
      uuid: "j2",
      resource: "tenant.services.pdf",
      format: "pdf",
      status: "completed",
      row_count: 1,
      created_at: "2026-10-02T09:00:00Z",
    };
    pdf.request.mockResolvedValue(done);
    pdf.download.mockResolvedValue({
      blob: new Blob(["%PDF"]),
      filename: null,
    });
    await render(
      createElement(ServiceDetailPage, { slug: "acme", uuid: "s1" }),
    );
    // Without vehicles.read there is no vehicle page link.
    expect($("[data-testid=detail-vehicle-link]")).toBeNull();

    const button = $("[data-testid=service-pdf]") as HTMLButtonElement;
    expect(button.textContent).toBe("services.detail.pdf");
    await act(async () => {
      button.click();
    });
    await flush();
    expect(pdf.forService).toHaveBeenCalledWith("s1");
    expect(pdf.request).toHaveBeenCalled();
    expect(pdf.download).toHaveBeenCalledWith(done);
    expect(cert.trigger).toHaveBeenCalledWith(
      expect.any(Blob),
      `${svc.service_no}.pdf`,
    );
    expect(cert.toast.success).toHaveBeenCalledWith(
      "services.detail.pdf_ready",
    );
  });

  it("reports a failed service PDF job", async () => {
    state.grants = new Set(["services.read"]);
    api.getService.mockResolvedValue(service());
    pdf.request.mockResolvedValue({
      uuid: "j3",
      resource: "tenant.services.pdf",
      format: "pdf",
      status: "failed",
      error: "boom",
      row_count: 0,
      created_at: "2026-10-02T09:00:00Z",
    });
    await render(
      createElement(ServiceDetailPage, { slug: "acme", uuid: "s1" }),
    );
    await act(async () => {
      ($("[data-testid=service-pdf]") as HTMLButtonElement).click();
    });
    await flush();
    expect(pdf.download).not.toHaveBeenCalled();
    expect(cert.toast.error).toHaveBeenCalledWith("services.detail.pdf_failed");
  });

  it("downloads the warranty PDF with warranties.read (TEC-188)", async () => {
    state.grants = new Set(["services.read"]);
    api.getService.mockResolvedValue(service());
    await render(
      createElement(ServiceDetailPage, { slug: "acme", uuid: "s1" }),
    );
    expect($("[data-testid=warranty-pdf]")).toBeNull();
    act(() => root.unmount());

    root = createRoot(container);
    state.grants = new Set(["services.read", "warranties.read"]);
    const done = {
      uuid: "j1",
      resource: "tenant.warranty.certificate",
      format: "pdf",
      status: "completed",
      row_count: 1,
      created_at: "2026-10-02T09:00:00Z",
    };
    cert.request.mockResolvedValue(done);
    cert.download.mockResolvedValue({
      blob: new Blob(["%PDF"]),
      filename: "garanti.pdf",
    });
    await render(
      createElement(ServiceDetailPage, { slug: "acme", uuid: "s1" }),
    );
    const button = $("[data-testid=warranty-pdf]") as HTMLButtonElement;
    expect(button.textContent).toBe("warranty.certificate.download");
    await act(async () => {
      button.click();
    });
    await flush();
    expect(cert.forService).toHaveBeenCalledWith("s1");
    expect(cert.request).toHaveBeenCalled();
    expect(cert.download).toHaveBeenCalledWith(done);
    expect(cert.trigger).toHaveBeenCalledWith(expect.any(Blob), "garanti.pdf");
    expect(cert.toast.success).toHaveBeenCalledWith(
      "warranty.certificate.ready",
    );
  });

  it("links a draft back to the wizard", async () => {
    state.grants = new Set(["services.read", ...wizardGrants]);
    api.getService.mockResolvedValue(
      service({
        status: "draft",
        status_label: "Draft",
        items_editable: true,
        editable: true,
        images: [],
        warranties: [],
        items: [],
      }),
    );
    await render(
      createElement(ServiceDetailPage, { slug: "acme", uuid: "s1" }),
    );
    expect($("[data-testid=continue-wizard]")?.getAttribute("href")).toBe(
      "/t/acme/services/s1/wizard",
    );
    expect($("[data-testid=detail-items-empty]")).not.toBeNull();
    expect($("[data-testid=detail-images-empty]")).not.toBeNull();
    expect($("[data-testid=detail-warranties-empty]")?.textContent).toBe(
      "services.detail.warranties_pending",
    );
  });

  it("hides the wizard link without services.write", async () => {
    state.grants = new Set(["services.read"]);
    api.getService.mockResolvedValue(
      service({ status: "draft", items_editable: true }),
    );
    await render(
      createElement(ServiceDetailPage, { slug: "acme", uuid: "s1" }),
    );
    expect($("[data-testid=service-detail]")).not.toBeNull();
    expect($("[data-testid=continue-wizard]")).toBeNull();
  });

  it("shows not found on 404", async () => {
    state.grants = new Set(["services.read"]);
    api.getService.mockRejectedValue(
      new ApiError({ status: 404, code: "NOT_FOUND", message: "nope" }),
    );
    await render(
      createElement(ServiceDetailPage, { slug: "acme", uuid: "s1" }),
    );
    expect(container.textContent).toContain("services.detail.not_found");
  });
});
