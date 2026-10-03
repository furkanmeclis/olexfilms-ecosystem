// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  getService: vi.fn(),
  listStockUnits: vi.fn(),
  addItem: vi.fn(),
  removeItem: vi.fn(),
  transition: vi.fn(),
}));
const catalog = vi.hoisted(() => ({ listCategories: vi.fn() }));
const granted = vi.hoisted(() => ({ set: new Set<string>() }));
const router = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
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
  }),
}));
vi.mock("next/navigation", () => ({ useRouter: () => router }));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => granted.set.has(p) }),
}));
vi.mock("@/providers/toast-provider", () => ({ appToast: toast }));
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
vi.mock("@/features/catalog/services/catalog.service", () => ({
  catalogService: catalog,
}));

import type {
  Service,
  ServiceItem,
  ServiceStockUnit,
} from "@/features/services/services/service-wizard.service";
import { ApiError } from "@/lib/api/errors";

import { CarPartPicker } from "./car-part-picker";
import { PartsStep } from "./parts-step";
import { ServiceWizardPage } from "./service-wizard-page";
import { StockStep } from "./stock-step";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
// Radix checkboxes inside a form measure themselves.
(globalThis as { ResizeObserver?: unknown }).ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  granted.set = new Set(["services.write", "catalog.read"]);
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
  if (!el) throw new Error("element not found");
  await act(async () => {
    el.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  await flush();
}

async function key(el: Element | null, k: string) {
  if (!el) throw new Error("element not found");
  await act(async () => {
    el.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true }));
  });
  await flush();
}

async function submit(el: Element | null) {
  if (!(el instanceof HTMLFormElement)) throw new Error("form not found");
  await act(async () => {
    el.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  await flush();
}

const $ = (sel: string) => container.querySelector(sel);
const $$ = (sel: string) => Array.from(container.querySelectorAll(sel));

const service = (over: Partial<Service> = {}): Service => ({
  uuid: "s1",
  service_no: "DSAB12CD34",
  status: "draft",
  status_label: "Draft",
  organization: { uuid: "o1", name: "Bayi", type: "dealer" },
  customer: {
    uuid: "c1",
    name: "Ayşe",
    surname: "Yılmaz",
    phone: "+905551234567",
    anonymized: false,
  },
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
  available_transitions: ["pending", "completed"],
  items: [],
  ...over,
});

const product = (
  uuid: string,
  name: string,
  unit_type: string,
  available_parts: string[],
) => ({ uuid, sku: `SKU-${uuid}`, name, unit_type, available_parts });

const rollUnit: ServiceStockUnit = {
  uuid: "u-roll",
  barcode: "R-001",
  unit_kind: "serial",
  product: product("p-roll", "PPF Roll", "roll_meter", [
    "body_kaput",
    "body_tavan",
  ]),
  quantity_on_hand: 1,
  initial_meters: "50.00",
  remaining_meters: "20.00",
};

const fixedUnit: ServiceStockUnit = {
  uuid: "u-fixed",
  barcode: "F-001",
  unit_kind: "fixed",
  product: product("p-fixed", "Cleaner", "piece", []),
  quantity_on_hand: 4,
  initial_meters: null,
  remaining_meters: null,
};

const item = (over: Partial<ServiceItem> = {}): ServiceItem => ({
  uuid: "i1",
  product: {
    uuid: "p-roll",
    sku: "SKU-p-roll",
    name: "PPF Roll",
    unit_type: "roll_meter",
  },
  barcode: "R-001",
  unit_kind: "serial",
  kind: "partial",
  quantity: null,
  meters: "12.50",
  applied_parts: ["body_kaput"],
  notes: null,
  created_at: "2026-10-01T00:00:00Z",
  correction: null,
  ...over,
});

describe("Step 2: SVG part picker", () => {
  it("toggles parts by click and keyboard, only available ones", async () => {
    const onChange = vi.fn();
    await render(
      createElement(CarPartPicker, {
        available: ["body_kaput", "window_on_cam"],
        selected: ["window_on_cam"],
        onChange,
      }),
    );
    const hood = $('path[data-part="body_kaput"]');
    expect(hood?.getAttribute("role")).toBe("checkbox");
    expect(hood?.getAttribute("aria-checked")).toBe("false");
    expect(hood?.getAttribute("aria-label")).toBe(
      "services.parts.names.body_kaput",
    );
    expect(hood?.getAttribute("tabindex")).toBe("0");
    expect(
      $('path[data-part="window_on_cam"]')?.getAttribute("aria-checked"),
    ).toBe("true");

    await click(hood);
    expect(onChange).toHaveBeenLastCalledWith(["window_on_cam", "body_kaput"]);

    await key($('path[data-part="window_on_cam"]'), " ");
    expect(onChange).toHaveBeenLastCalledWith([]);
    await key(hood, "Enter");
    expect(onChange).toHaveBeenCalledTimes(3);

    // Not in available_parts: disabled, not focusable, ignored.
    const roof = $('path[data-part="body_tavan"]');
    expect(roof?.getAttribute("aria-disabled")).toBe("true");
    expect(roof?.getAttribute("tabindex")).toBe("-1");
    await click(roof);
    expect(onChange).toHaveBeenCalledTimes(3);

    // The checkbox list mirrors the drawing.
    await click($('[data-part-check="body_kaput"]'));
    expect(onChange).toHaveBeenLastCalledWith(["window_on_cam", "body_kaput"]);
  });

  it("applies a preset to its own group", async () => {
    const onChange = vi.fn();
    await render(
      createElement(CarPartPicker, {
        available: ["body_kaput", "body_tavan", "window_on_cam"],
        selected: ["body_tavan", "window_on_cam"],
        onChange,
      }),
    );
    await click($('[data-preset="hood_only"]'));
    expect(onChange).toHaveBeenLastCalledWith(["window_on_cam", "body_kaput"]);
  });

  it("takes the options from the category available_parts", async () => {
    catalog.listCategories.mockResolvedValue({
      items: [
        { uuid: "k1", name: "PPF", available_parts: ["body_kaput"] },
        { uuid: "k2", name: "Glass", available_parts: ["window_on_cam"] },
        { uuid: "k3", name: "Care", available_parts: [] },
      ],
      total: 3,
      limit: 100,
      offset: 0,
    });
    const onChange = vi.fn();
    await render(
      createElement(PartsStep, {
        service: service(),
        selected: [],
        onChange,
        onBack: vi.fn(),
        onNext: vi.fn(),
      }),
    );
    expect(catalog.listCategories).toHaveBeenCalledWith({
      active: true,
      limit: 100,
    });
    const enabled = () =>
      $$('path[role="checkbox"]:not([aria-disabled])').map((p) =>
        p.getAttribute("data-part"),
      );
    expect(enabled().sort()).toEqual(["body_kaput", "window_on_cam"]);

    await click($('[data-category="k2"]'));
    expect(enabled()).toEqual(["window_on_cam"]);
    await click($('path[data-part="window_on_cam"]'));
    expect(onChange).toHaveBeenLastCalledWith(["window_on_cam"]);
  });
});

describe("Step 4: products and stock", () => {
  function renderStock(svc: Service, parts: string[] = []) {
    const props = {
      service: svc,
      selectedParts: parts,
      onChanged: vi.fn(),
      onBack: vi.fn(),
      onCompleted: vi.fn(),
    };
    return render(createElement(StockStep, props)).then(() => props);
  }

  it("blocks meters above the rest of the roll and adds a cut", async () => {
    api.listStockUnits.mockResolvedValue({ items: [rollUnit, fixedUnit] });
    const saved = service({ items: [item()] });
    api.addItem.mockResolvedValue(saved);
    const props = await renderStock(service(), ["body_kaput", "window_on_cam"]);
    expect($$("[data-unit]")).toHaveLength(2);

    await click($('[data-unit="R-001"] [data-testid="pick-unit"]'));
    await type($('input[name="meters"]'), "25");
    expect($('[data-testid="meters-error"]')?.textContent).toContain(
      "services.stock.meters_errors.exceeds",
    );
    expect(
      ($('[data-testid="add-unit-submit"]') as HTMLButtonElement).disabled,
    ).toBe(true);
    await submit($('[data-testid="add-unit-form"]'));
    expect(api.addItem).not.toHaveBeenCalled();

    await type($('input[name="meters"]'), "12,5");
    expect($('[data-testid="meters-error"]')).toBeNull();
    await submit($('[data-testid="add-unit-form"]'));
    expect(api.addItem).toHaveBeenCalledWith("s1", {
      barcode: "R-001",
      product_uuid: "p-roll",
      kind: "partial",
      meters: 12.5,
      applied_parts: ["body_kaput"],
    });
    expect(props.onChanged).toHaveBeenCalledWith(saved);
  });

  it("adds pieces of a fixed barcode within the pieces on hand", async () => {
    api.listStockUnits.mockResolvedValue({ items: [fixedUnit] });
    api.addItem.mockResolvedValue(service());
    await renderStock(service(), ["body_kaput"]);
    await click($('[data-testid="pick-unit"]'));
    await type($('input[name="quantity"]'), "5");
    expect($('[data-testid="quantity-error"]')?.textContent).toContain(
      "services.stock.quantity_errors.exceeds",
    );
    await type($('input[name="quantity"]'), "3");
    await submit($('[data-testid="add-unit-form"]'));
    expect(api.addItem).toHaveBeenCalledWith("s1", {
      barcode: "F-001",
      product_uuid: "p-fixed",
      kind: "full",
      quantity: 3,
      applied_parts: [],
    });
  });

  it("opens a scanned barcode and shows server errors translated", async () => {
    api.listStockUnits.mockImplementation(
      (_uuid: string, params: { barcode?: string }) =>
        Promise.resolve({
          items: params.barcode === "R-001" ? [rollUnit] : [],
        }),
    );
    api.addItem.mockRejectedValue(
      new ApiError({
        status: 409,
        code: "SERVICE_UNIT_NOT_AVAILABLE",
        message: "unit not available",
      }),
    );
    await renderStock(service());
    await type($('input[name="barcode"]'), "NOPE");
    await submit($('[data-testid="scan-form"]'));
    expect($('[data-testid="scan-error"]')?.textContent).toBe(
      "services.stock.scan_not_found",
    );

    await type($('input[name="barcode"]'), "R-001");
    await submit($('[data-testid="scan-form"]'));
    expect(api.listStockUnits).toHaveBeenCalledWith("s1", {
      barcode: "R-001",
    });
    expect($('[data-testid="add-unit-form"]')?.textContent).toContain(
      "PPF Roll",
    );
    await click($('[data-testid="whole-roll"]'));
    await submit($('[data-testid="add-unit-form"]'));
    expect(api.addItem).toHaveBeenCalledWith(
      "s1",
      expect.objectContaining({ kind: "full", barcode: "R-001" }),
    );
    expect($('[data-testid="add-error"]')?.textContent).toBe(
      "services.stock.errors.SERVICE_UNIT_NOT_AVAILABLE",
    );
  });

  it("removes an item", async () => {
    api.listStockUnits.mockResolvedValue({ items: [] });
    const after = service({ items: [] });
    api.removeItem.mockResolvedValue(after);
    const props = await renderStock(service({ items: [item()] }));
    expect($$('[data-testid="service-item"]')).toHaveLength(1);
    expect($('[data-testid="service-item"]')?.textContent).toContain(
      "services.parts.names.body_kaput",
    );
    await click($('[data-testid="remove-item"]'));
    expect(api.removeItem).toHaveBeenCalledWith("s1", "i1");
    expect(props.onChanged).toHaveBeenCalledWith(after);
  });

  it("completes the service only with services.complete", async () => {
    api.listStockUnits.mockResolvedValue({ items: [] });
    const done = service({ status: "completed", items_editable: false });
    api.transition.mockResolvedValue(done);

    const noPerm = await renderStock(service({ items: [item()] }));
    expect($('[data-testid="complete-service"]')).toBeNull();
    expect($('[data-testid="complete-hint"]')).not.toBeNull();
    expect(noPerm.onCompleted).not.toHaveBeenCalled();

    act(() => root.unmount());
    root = createRoot(container);
    granted.set.add("services.complete");
    const props = await renderStock(service({ items: [item()] }));
    await click($('[data-testid="complete-service"]'));
    expect(api.transition).toHaveBeenCalledWith("s1", "completed");
    expect(props.onCompleted).toHaveBeenCalledWith(done);
  });

  it("does not complete a service without items", async () => {
    granted.set.add("services.complete");
    api.listStockUnits.mockResolvedValue({ items: [] });
    await renderStock(service({ items: [] }));
    expect(
      ($('[data-testid="complete-service"]') as HTMLButtonElement).disabled,
    ).toBe(true);
  });
});

describe("Wizard flow: parts, roll and pieces, completion", () => {
  it("applies the step 2 parts to the items and completes", async () => {
    window.sessionStorage.clear();
    granted.set = new Set([
      "services.write",
      "services.complete",
      "customers.read",
      "vehicles.read",
      "catalog.read",
    ]);
    catalog.listCategories.mockResolvedValue({
      items: [
        {
          uuid: "k1",
          name: "PPF",
          available_parts: ["body_kaput", "body_tavan"],
        },
      ],
      total: 1,
      limit: 100,
      offset: 0,
    });
    api.getService.mockResolvedValue(service());
    api.listStockUnits.mockResolvedValue({ items: [rollUnit, fixedUnit] });
    const withRoll = service({ items: [item()] });
    const withBoth = service({
      items: [
        item(),
        item({
          uuid: "i2",
          product: {
            uuid: "p-fixed",
            sku: "SKU-p-fixed",
            name: "Cleaner",
            unit_type: "piece",
          },
          barcode: "F-001",
          unit_kind: "fixed",
          kind: "full",
          quantity: 2,
          meters: null,
          applied_parts: [],
        }),
      ],
    });
    api.addItem.mockResolvedValueOnce(withRoll).mockResolvedValueOnce(withBoth);
    api.transition.mockResolvedValue(
      service({
        status: "completed",
        items_editable: false,
        items: withBoth.items,
      }),
    );

    await render(
      createElement(ServiceWizardPage, { slug: "acme", uuid: "s1" }),
    );
    // Step 2 opens first for an existing draft.
    expect($('[data-testid="parts-step"]')).not.toBeNull();
    await click($('path[data-part="body_kaput"]'));
    expect($('[data-testid="parts-count"]')?.textContent).toContain(
      '"count":1',
    );
    expect(window.sessionStorage.getItem("service-wizard-parts:s1")).toBe(
      '["body_kaput"]',
    );

    await click($('[data-step="stock"]'));
    expect($('[data-testid="stock-step"]')).not.toBeNull();

    await click($('[data-unit="R-001"] [data-testid="pick-unit"]'));
    await type($('input[name="meters"]'), "12.5");
    await submit($('[data-testid="add-unit-form"]'));
    expect(api.addItem).toHaveBeenLastCalledWith("s1", {
      barcode: "R-001",
      product_uuid: "p-roll",
      kind: "partial",
      meters: 12.5,
      applied_parts: ["body_kaput"],
    });

    await click($('[data-unit="F-001"] [data-testid="pick-unit"]'));
    await type($('input[name="quantity"]'), "2");
    await submit($('[data-testid="add-unit-form"]'));
    expect(api.addItem).toHaveBeenLastCalledWith("s1", {
      barcode: "F-001",
      product_uuid: "p-fixed",
      kind: "full",
      quantity: 2,
      applied_parts: [],
    });
    expect($$('[data-testid="service-item"]')).toHaveLength(2);

    await click($('[data-testid="complete-service"]'));
    expect(api.transition).toHaveBeenCalledWith("s1", "completed");
    expect(router.push).toHaveBeenCalledWith("/t/acme/services/s1");
    expect(window.sessionStorage.getItem("service-wizard-parts:s1")).toBeNull();
  });
});
