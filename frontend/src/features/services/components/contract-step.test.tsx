// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const wizardApi = vi.hoisted(() => ({ getService: vi.fn() }));
const contractApi = vi.hoisted(() => ({
  createForService: vi.fn(),
  get: vi.fn(),
  requestCustomerOtp: vi.fn(),
  signCustomer: vi.fn(),
  signStaff: vi.fn(),
  addMedia: vi.fn(),
  deleteMedia: vi.fn(),
  downloadPdf: vi.fn(),
}));
const granted = vi.hoisted(() => ({ set: new Set<string>() }));
const feature = vi.hoisted(() => ({ enabled: true, photos: false }));
const photoApi = vi.hoisted(() => ({ intake: vi.fn(), uploadIntake: vi.fn() }));
const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "tr",
    format: { dateTime: (v: string) => v },
  }),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => granted.set.has(p) }),
}));
vi.mock("@/providers/toast-provider", () => ({ appToast: toast }));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/features/modules/hooks/use-features", () => ({
  useFeature: (_slug: string, key: string) => ({
    enabled: key === "photo_standard" ? feature.photos : feature.enabled,
    isLoading: false,
    isError: false,
  }),
}));
vi.mock(
  "@/features/photo-standard/services/photo-standard.service",
  async (orig) => ({
    ...(await orig<object>()),
    photoStandardService: photoApi,
  }),
);
vi.mock(
  "@/features/services/services/service-wizard.service",
  async (orig) => ({
    ...(await orig<object>()),
    serviceWizardService: wizardApi,
  }),
);
vi.mock(
  "@/features/contracts/services/contract-signing.service",
  async (orig) => ({
    ...(await orig<object>()),
    contractSigningService: contractApi,
  }),
);
vi.mock("@/features/catalog/services/catalog.service", () => ({
  catalogService: {
    listCategories: () =>
      Promise.resolve({ items: [], total: 0, limit: 100, offset: 0 }),
  },
}));

import type { Contract } from "@/features/contracts/services/contract-signing.service";
import type { Service } from "@/features/services/services/service-wizard.service";
import { ApiError } from "@/lib/api/errors";

import { ContractStep } from "./contract-step";
import { ServiceContractCard } from "./service-contract-card";
import { ServiceWizardPage } from "./service-wizard-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const PNG = "data:image/png;base64,iVBORw0KGgoSIG";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  granted.set = new Set([
    "services.write",
    "customers.read",
    "vehicles.read",
    "contracts.read",
    "contracts.write",
  ]);
  feature.enabled = true;
  feature.photos = false;
  // jsdom has no canvas backend: a drawing context stub and a fixed PNG.
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockImplementation(
    () =>
      ({
        scale: vi.fn(),
        beginPath: vi.fn(),
        moveTo: vi.fn(),
        lineTo: vi.fn(),
        stroke: vi.fn(),
        clearRect: vi.fn(),
      }) as unknown as CanvasRenderingContext2D,
  );
  vi.spyOn(HTMLCanvasElement.prototype, "toDataURL").mockReturnValue(PNG);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
  vi.restoreAllMocks();
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

async function click(el: Element | null | undefined) {
  if (!el) throw new Error("element not found");
  await act(async () => {
    el.dispatchEvent(new MouseEvent("click", { bubbles: true }));
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

/** One stroke on a signature canvas (pointer events). */
async function draw(canvas: Element | null) {
  if (!canvas) throw new Error("canvas not found");
  for (const type of ["pointerdown", "pointermove", "pointerup"]) {
    await act(async () => {
      canvas.dispatchEvent(
        new MouseEvent(type, { bubbles: true, clientX: 10, clientY: 12 }),
      );
    });
  }
  await flush();
}

const $ = (sel: string) => container.querySelector(sel);
const $$ = (sel: string) => Array.from(container.querySelectorAll(sel));
const disabled = (sel: string) =>
  ($(sel) as HTMLButtonElement | null)?.disabled;

const service = (over: Partial<Service> = {}): Service =>
  ({
    uuid: "s1",
    service_no: "SRV-1",
    status: "draft",
    status_label: "Draft",
    available_transitions: ["completed"],
    items: [],
    items_editable: true,
    contract_required: false,
    contract: null,
    updated_at: "2026-10-07T10:00:00Z",
    ...over,
  }) as unknown as Service;

const contract = (over: Partial<Contract> = {}): Contract => ({
  uuid: "k1",
  contract_no: 7,
  subject_type: "service",
  subject_id: 1,
  kind: "vehicle_intake",
  locale: "tr",
  template_version: 1,
  otp_required: true,
  signature_required: true,
  status: "pending",
  pdf_ready: false,
  rendered_html: "<p>Sözleşme metni</p>",
  signers: [
    {
      uuid: "sg1",
      role: "customer",
      name: "Ayşe Yılmaz",
      phone_e164: "+905551234567",
    },
    { uuid: "sg2", role: "staff", name: "Ali Usta" },
  ],
  media: [],
  created_at: "2026-10-07T10:00:00Z",
  updated_at: "2026-10-07T10:00:00Z",
  ...over,
});

const signed = (c: Contract, role: "customer" | "staff"): Contract => ({
  ...c,
  signers: c.signers.map((s) =>
    s.role === role ? { ...s, signed_at: "2026-10-07T10:05:00Z" } : s,
  ),
});

const summary = (c: Contract) => ({
  uuid: c.uuid,
  status: c.status,
  contract_no: c.contract_no,
  pdf_ready: c.pdf_ready,
});

describe("ContractStep (TEC-291)", () => {
  it("keeps İleri disabled in required mode until the contract is executed", async () => {
    const pending = contract();
    const customerDone = signed(pending, "customer");
    const executed: Contract = {
      ...signed(customerDone, "staff"),
      status: "executed",
    };
    contractApi.createForService.mockResolvedValue(pending);
    contractApi.get.mockResolvedValue(pending);
    contractApi.requestCustomerOtp.mockResolvedValue({
      channel: "whatsapp",
      expires_at: "2026-10-07T10:30:00Z",
      resend_at: "2026-10-07T10:01:00Z",
    });
    contractApi.signCustomer.mockResolvedValue(customerDone);
    contractApi.signStaff.mockResolvedValue(executed);
    const onNext = vi.fn();

    await render(
      createElement(ContractStep, {
        service: service({ contract_required: true }),
        onBack: vi.fn(),
        onNext,
      }),
    );
    expect($('[data-testid="contract-required"]')).not.toBeNull();
    expect(disabled('[data-testid="contract-next"]')).toBe(true);

    await click($('[data-testid="contract-create"]'));
    expect(contractApi.createForService).toHaveBeenCalledWith("s1");
    expect(
      $('[data-testid="contract-preview"]')?.getAttribute("srcdoc"),
    ).toContain("<p>Sözleşme metni</p>");
    expect(disabled('[data-testid="contract-next"]')).toBe(true);

    // Customer: KVKK notice, OTP, countdown, code and signature.
    expect($('[data-testid="kvkk-notice"]')).not.toBeNull();
    await click($('[data-testid="otp-send"]'));
    expect(contractApi.requestCustomerOtp).toHaveBeenCalledWith("k1");
    expect($('[data-testid="otp-countdown"]')?.textContent).toContain("30:00");
    await type($('input[name="code"]'), "123456");
    await draw($('[data-testid="customer-canvas"] canvas'));
    await click($('[data-testid="customer-sign-submit"]'));
    expect(contractApi.signCustomer).toHaveBeenCalledWith("k1", {
      code: "123456",
      signature_png: "iVBORw0KGgoSIG",
    });
    expect($('[data-testid="customer-signed"]')).not.toBeNull();
    expect(disabled('[data-testid="contract-next"]')).toBe(true);
    await click($('[data-testid="contract-next"]'));
    expect(onNext).not.toHaveBeenCalled();

    // Staff signs last: the contract executes and İleri opens.
    await draw($('[data-testid="staff-canvas"] canvas'));
    await click($('[data-testid="staff-sign-submit"]'));
    expect(contractApi.signStaff).toHaveBeenCalledWith("k1", {
      signature_png: "iVBORw0KGgoSIG",
    });
    expect($('[data-testid="contract-executed"]')).not.toBeNull();
    expect(disabled('[data-testid="contract-next"]')).toBe(false);
    await click($('[data-testid="contract-next"]'));
    expect(onNext).toHaveBeenCalledTimes(1);
  });

  it("lets an optional contract be skipped", async () => {
    const onNext = vi.fn();
    await render(
      createElement(ContractStep, {
        service: service(),
        onBack: vi.fn(),
        onNext,
      }),
    );
    expect($('[data-testid="contract-required"]')).toBeNull();
    expect(disabled('[data-testid="contract-next"]')).toBe(false);
    await click($('[data-testid="contract-next"]'));
    expect(onNext).toHaveBeenCalled();
  });

  it("disables İmzala while the canvas is empty and enables it after drawing", async () => {
    const pending = contract({ otp_required: false });
    contractApi.get.mockResolvedValue(pending);
    await render(
      createElement(ContractStep, {
        service: service({ contract: summary(pending) }),
        onBack: vi.fn(),
        onNext: vi.fn(),
      }),
    );
    expect(disabled('[data-testid="customer-sign-submit"]')).toBe(true);
    expect(disabled('[data-testid="staff-sign-submit"]')).toBe(true);

    await draw($('[data-testid="customer-canvas"] canvas'));
    expect(disabled('[data-testid="customer-sign-submit"]')).toBe(false);
    expect(disabled('[data-testid="staff-sign-submit"]')).toBe(true);

    // Clearing empties the canvas again.
    await click(
      $('[data-testid="customer-canvas"] [data-testid="signature-clear"]'),
    );
    expect(disabled('[data-testid="customer-sign-submit"]')).toBe(true);
  });

  it("shows OTP errors at the field and keeps 500s generic", async () => {
    const pending = contract();
    contractApi.get.mockResolvedValue(pending);
    contractApi.requestCustomerOtp
      .mockRejectedValueOnce(
        new ApiError({ status: 500, code: "INTERNAL", message: "boom" }),
      )
      .mockRejectedValueOnce(
        new ApiError({
          status: 400,
          code: "VALIDATION_ERROR",
          message: "customer phone is required",
        }),
      )
      .mockResolvedValue({
        channel: "sms",
        expires_at: "2026-10-07T10:30:00Z",
        resend_at: "2026-10-07T10:01:00Z",
      });
    contractApi.signCustomer.mockRejectedValueOnce(
      new ApiError({
        status: 400,
        code: "VALIDATION_ERROR",
        message: "invalid OTP code",
      }),
    );
    await render(
      createElement(ContractStep, {
        service: service({ contract: summary(pending) }),
        onBack: vi.fn(),
        onNext: vi.fn(),
      }),
    );

    await click($('[data-testid="otp-send"]'));
    expect(toast.error).toHaveBeenCalledWith(
      "services.contract.errors.otp_failed",
    );
    expect($('[data-testid="customer-sign"]')?.textContent).not.toContain(
      "services.contract.errors.no_phone",
    );

    await click($('[data-testid="otp-send"]'));
    const otpError = $('[data-testid="otp-send"]')?.getAttribute(
      "aria-describedby",
    );
    expect(otpError).toBeTruthy();
    expect(container.querySelector(`[id="${otpError}"]`)?.textContent).toBe(
      "services.contract.errors.no_phone",
    );

    await click($('[data-testid="otp-send"]'));
    await type($('input[name="code"]'), "000000");
    await draw($('[data-testid="customer-canvas"] canvas'));
    await click($('[data-testid="customer-sign-submit"]'));
    const code = $('input[name="code"]');
    expect(code?.getAttribute("aria-invalid")).toBe("true");
    const codeError = code?.getAttribute("aria-describedby");
    expect(container.querySelector(`[id="${codeError}"]`)?.textContent).toBe(
      "services.contract.errors.invalid_code",
    );
  });

  it("removes media only before the contract is executed", async () => {
    const media = {
      uuid: "m1",
      storage_key: "k",
      mime_type: "image/png" as const,
      size_bytes: 1024,
      sha256: "a".repeat(64),
      sort_order: 0,
      created_at: "2026-10-07T10:00:00Z",
    };
    const pending = contract({ media: [media] });
    contractApi.get.mockResolvedValue(pending);
    await render(
      createElement(ContractStep, {
        service: service({ contract: summary(pending) }),
        onBack: vi.fn(),
        onNext: vi.fn(),
      }),
    );
    expect($$('[data-testid="contract-media-item"]')).toHaveLength(1);
    expect($('[data-testid="contract-media-remove"]')).not.toBeNull();

    // A file over 12 MB is rejected before the upload.
    const input = $('[data-testid="contract-media-input"]') as HTMLInputElement;
    const big = new File(["x"], "big.png", { type: "image/png" });
    Object.defineProperty(big, "size", { value: 13 * 1024 * 1024 });
    Object.defineProperty(input, "files", { value: [big], configurable: true });
    await act(async () => {
      input.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await flush();
    expect(contractApi.addMedia).not.toHaveBeenCalled();
    expect($('[data-testid="contract-media"]')?.textContent).toContain(
      "services.contract.errors.media_size",
    );

    act(() => root.unmount());
    root = createRoot(container);
    const executed = contract({ media: [media], status: "executed" });
    contractApi.get.mockResolvedValue(executed);
    await render(
      createElement(ContractStep, {
        service: service({ contract: summary(executed) }),
        onBack: vi.fn(),
        onNext: vi.fn(),
      }),
    );
    expect($$('[data-testid="contract-media-item"]')).toHaveLength(1);
    expect($('[data-testid="contract-media-remove"]')).toBeNull();
    expect($('[data-testid="contract-media-input"]')).toBeNull();
  });
});

describe("ServiceWizardPage contract step (TEC-291)", () => {
  it("lists the contract step only while intake_contracts is on", async () => {
    wizardApi.getService.mockResolvedValue(service());
    feature.enabled = false;
    await render(
      createElement(ServiceWizardPage, { slug: "acme", uuid: "s1" }),
    );
    const off = $$('[data-testid="wizard-stepper"] [data-step]').map((b) =>
      b.getAttribute("data-step"),
    );
    expect(off).toEqual(["customer_vehicle", "parts", "measurement", "stock"]);

    act(() => root.unmount());
    root = createRoot(container);
    feature.enabled = true;
    await render(
      createElement(ServiceWizardPage, { slug: "acme", uuid: "s1" }),
    );
    const on = $$('[data-testid="wizard-stepper"] [data-step]').map((b) =>
      b.getAttribute("data-step"),
    );
    expect(on).toEqual([
      "customer_vehicle",
      "parts",
      "measurement",
      "contract",
      "stock",
    ]);
  });

  it("keeps the stock step closed while a required contract is not executed", async () => {
    const pending = contract();
    contractApi.get.mockResolvedValue(pending);
    wizardApi.getService.mockResolvedValue(
      service({ contract_required: true, contract: summary(pending) }),
    );
    await render(
      createElement(ServiceWizardPage, { slug: "acme", uuid: "s1" }),
    );
    expect(disabled('[data-step="stock"]')).toBe(true);
    await click($('[data-step="contract"]'));
    expect($('[data-testid="contract-step"]')).not.toBeNull();
    expect(disabled('[data-testid="contract-next"]')).toBe(true);
    expect($('[data-testid="stock-step"]')).toBeNull();
  });
});

const angle = (key: string, over: Record<string, unknown> = {}) => ({
  angle: {
    uuid: `a-${key}`,
    key,
    name: { tr: key },
    hint: {},
    required: true,
    sort_order: 10,
    active: true,
  },
  missing: false,
  required: true,
  photo: {
    uuid: `p-${key}`,
    angle_key: key,
    url: `/v1/services/s1/intake-photos/${key}/file`,
    mime: "image/jpeg",
    size: 10,
    sha256: "0".repeat(64),
    created_at: "2026-10-07T10:00:00Z",
  },
  ...over,
});

describe("ServiceWizardPage photos step (TEC-500)", () => {
  const steps = () =>
    $$('[data-testid="wizard-stepper"] [data-step]').map((b) =>
      b.getAttribute("data-step"),
    );

  it("lists photos after customer_vehicle only while photo_standard is on", async () => {
    wizardApi.getService.mockResolvedValue(service());
    await render(
      createElement(ServiceWizardPage, { slug: "acme", uuid: "s1" }),
    );
    expect(steps()).not.toContain("photos");
    expect(photoApi.intake).not.toHaveBeenCalled();

    act(() => root.unmount());
    root = createRoot(container);
    feature.photos = true;
    photoApi.intake.mockResolvedValue({
      service_uuid: "s1",
      angles: [angle("front")],
      missing: [],
    });
    await render(
      createElement(ServiceWizardPage, { slug: "acme", uuid: "s1" }),
    );
    expect(steps().slice(0, 3)).toEqual([
      "customer_vehicle",
      "photos",
      "parts",
    ]);
  });

  it("keeps İleri and the contract closed while a required angle is missing", async () => {
    feature.photos = true;
    wizardApi.getService.mockResolvedValue(service());
    photoApi.intake.mockResolvedValue({
      service_uuid: "s1",
      angles: [
        angle("front"),
        angle("rear", { photo: undefined, missing: true }),
      ],
      missing: ["rear"],
    });
    await render(
      createElement(ServiceWizardPage, { slug: "acme", uuid: "s1" }),
    );
    expect($('[data-testid="photos-step"]')).not.toBeNull();
    expect(disabled('[data-testid="photos-next"]')).toBe(true);
    expect(
      $('[data-testid="photos-next"]')?.getAttribute("aria-describedby"),
    ).toBe("intake-blocked-reason");
    expect($('[data-testid="intake-counter"]')?.textContent).toContain(
      '"count":1',
    );
    expect(disabled('[data-step="contract"]')).toBe(true);
    expect($('[data-step="contract"]')?.getAttribute("data-blocked")).toBe(
      "photos",
    );
    expect(disabled('[data-step="parts"]')).toBe(true);
  });

  it("marks the missing angles on the cards after a 422 PHOTO_STANDARD_INCOMPLETE", async () => {
    feature.photos = true;
    wizardApi.getService.mockResolvedValue(service());
    photoApi.intake
      .mockResolvedValueOnce({
        service_uuid: "s1",
        angles: [angle("front"), angle("rear")],
        missing: [],
      })
      .mockResolvedValue({
        service_uuid: "s1",
        angles: [
          angle("front"),
          angle("rear", { photo: undefined, missing: true }),
        ],
        missing: ["rear"],
      });
    contractApi.createForService.mockRejectedValue(
      new ApiError({
        status: 422,
        code: "PHOTO_STANDARD_INCOMPLETE",
        message: "missing",
        details: [
          { field: "intake_photos.rear", code: "missing", message: "x" },
        ],
      }),
    );
    await render(
      createElement(ServiceWizardPage, { slug: "acme", uuid: "s1" }),
    );
    expect(disabled('[data-testid="photos-next"]')).toBe(false);
    await click($('[data-step="contract"]'));
    await click($('[data-testid="contract-create"]'));

    expect($('[data-testid="photos-step"]')).not.toBeNull();
    const rear = $('[data-testid="intake-angle"][data-angle="rear"]');
    const front = $('[data-testid="intake-angle"][data-angle="front"]');
    expect(rear?.getAttribute("data-missing")).toBe("true");
    expect(
      rear?.querySelector('[data-testid="intake-missing"]'),
    ).not.toBeNull();
    expect(front?.getAttribute("data-missing")).toBe("false");
    expect(toast.error).toHaveBeenCalledWith(
      "services.stock.errors.PHOTO_STANDARD_INCOMPLETE",
    );
  });
});

describe("ServiceContractCard (TEC-291)", () => {
  it("spins while pdf_ready=false and polls until the PDF is ready", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const executed = contract({ status: "executed", pdf_ready: false });
      contractApi.get
        .mockResolvedValueOnce(executed)
        .mockResolvedValue({ ...executed, pdf_ready: true });
      await render(
        createElement(ServiceContractCard, { contract: summary(executed) }),
      );
      expect($('[data-testid="service-contract-no"]')?.textContent).toBe("#7");
      expect(disabled('[data-testid="service-contract-pdf"]')).toBe(true);
      expect($('[data-testid="service-contract-pdf-spinner"]')).not.toBeNull();
      expect($('[data-testid="service-contract-pdf"]')?.textContent).toContain(
        "services.contract.pdf_preparing",
      );

      await act(async () => {
        await vi.advanceTimersByTimeAsync(5000);
      });
      await flush();
      expect(contractApi.get).toHaveBeenCalledTimes(2);
      expect($('[data-testid="service-contract-pdf-spinner"]')).toBeNull();
      expect(disabled('[data-testid="service-contract-pdf"]')).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });

  it("downloads a ready PDF without polling", async () => {
    contractApi.downloadPdf.mockResolvedValue({
      blob: new Blob(["%PDF"]),
      filename: "c.pdf",
    });
    const createUrl = vi.fn(() => "blob:x");
    Object.assign(URL, {
      createObjectURL: createUrl,
      revokeObjectURL: vi.fn(),
    });
    const ready = contract({ status: "executed", pdf_ready: true });
    await render(
      createElement(ServiceContractCard, { contract: summary(ready) }),
    );
    expect($('[data-testid="service-contract-pdf-spinner"]')).toBeNull();
    await click($('[data-testid="service-contract-pdf"]'));
    expect(contractApi.get).not.toHaveBeenCalled();
    expect(contractApi.downloadPdf).toHaveBeenCalledWith("k1");
    expect(createUrl).toHaveBeenCalled();
  });
});
