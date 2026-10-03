// @vitest-environment jsdom
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  listWarehouses: vi.fn(),
  createEntry: vi.fn(),
}));
const catalog = vi.hoisted(() => ({ listProducts: vi.fn() }));
const nav = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("next/navigation", () => ({ useRouter: () => nav }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    // Error codes resolve to a text (warehouseErrorMessage keeps known codes).
    t: (key: string, params?: Record<string, string | number>) =>
      key.startsWith("warehouse.errors.")
        ? `ERR ${key.slice("warehouse.errors.".length)}`
        : params
          ? `${key} ${JSON.stringify(params)}`
          : key,
  }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/features/warehouse/services/warehouse.service", async (orig) => ({
  ...(await orig<object>()),
  warehouseService: api,
}));
vi.mock("@/features/catalog/services/catalog.service", async (orig) => ({
  ...(await orig<object>()),
  catalogService: catalog,
}));

import { ApiError } from "@/lib/api";

import { GenerateForm } from "./generate-form";
import { NodeForm } from "./node-form";
import { NewEntryForm } from "./stock-entries-page";
import {
  click,
  fill,
  mount,
  render,
  submit,
  unmount,
  type Mounted,
} from "./test-helpers";

// The Radix switch measures itself.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

let m: Mounted;
beforeEach(() => {
  m = mount();
});
afterEach(() => {
  unmount(m);
  vi.clearAllMocks();
});

const $ = <T extends Element = HTMLInputElement>(testId: string) =>
  m.container.querySelector<T>(`[data-testid="${testId}"]`);
const text = () => m.container.textContent ?? "";

describe("NodeForm validation (TEC-231)", () => {
  it("rejects an empty or malformed code without calling the API", async () => {
    const onSubmit = vi.fn();
    await render(
      m,
      createElement(NodeForm, {
        kind: "warehouse",
        mode: "create",
        onSubmit,
        onCancel: vi.fn(),
      }),
    );
    await submit(m.container.querySelector("form"));
    expect(text()).toContain("warehouse.validation.required");
    expect(onSubmit).not.toHaveBeenCalled();

    // The hyphen separates full_code levels; it is not a code character.
    await fill($("node-code"), "A-1");
    await submit(m.container.querySelector("form"));
    expect(text()).toContain("warehouse.validation.code");
    expect(onSubmit).not.toHaveBeenCalled();

    await fill($("node-code"), "x".repeat(33));
    await submit(m.container.querySelector("form"));
    expect(text()).toContain("warehouse.validation.code");
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("upper-cases a valid code and passes name and address", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    await render(
      m,
      createElement(NodeForm, {
        kind: "warehouse",
        mode: "create",
        onSubmit,
        onCancel: vi.fn(),
      }),
    );
    await fill($("node-code"), " wh_1 ");
    await fill($("node-name"), "Main");
    await fill($("node-address"), "Istanbul");
    await submit(m.container.querySelector("form"));
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        code: "WH_1",
        name: "Main",
        address: "Istanbul",
      }),
    );
  });

  it("offers only the types the parent allows and shows a server error", async () => {
    const onSubmit = vi
      .fn()
      .mockRejectedValue(
        new ApiError({
          status: 409,
          code: "WAREHOUSE_CODE_TAKEN",
          message: "taken",
        }),
      );
    await render(
      m,
      createElement(NodeForm, {
        kind: "location",
        mode: "create",
        allowedTypes: ["bin"],
        context: "WH1-R1-A-S1",
        onSubmit,
        onCancel: vi.fn(),
      }),
    );
    const select = $<HTMLSelectElement>("node-type");
    expect(Array.from(select?.options ?? []).map((o) => o.value)).toEqual([
      "bin",
    ]);
    expect(text()).toContain("WH1-R1-A-S1");
    // No address on a location.
    expect($("node-address")).toBeNull();

    await fill($("node-code"), "01");
    await submit(m.container.querySelector("form"));
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ code: "01", type: "bin" }),
    );
    expect($("node-form-error")?.textContent).toBe("ERR WAREHOUSE_CODE_TAKEN");
  });

  it("edit mode shows the active switch and no type choice", async () => {
    await render(
      m,
      createElement(NodeForm, {
        kind: "location",
        mode: "edit",
        initial: { code: "A", name: "Aisle A", active: true },
        onSubmit: vi.fn(),
        onCancel: vi.fn(),
      }),
    );
    expect($("node-type")).toBeNull();
    expect(m.container.querySelector('[role="switch"]')).not.toBeNull();
    expect($("node-code")?.value).toBe("A");
  });
});

describe("NewEntryForm validation (TEC-231)", () => {
  const warehouses = {
    items: [
      { uuid: "w1", code: "WH1", name: "Main", active: true },
      { uuid: "w2", code: "OLD", name: "Old", active: false },
    ],
  };

  it("needs a warehouse; a distributor only takes printed labels", async () => {
    api.listWarehouses.mockResolvedValue(warehouses);
    await render(
      m,
      createElement(NewEntryForm, {
        slug: "acme",
        isCenter: false,
        onCancel: vi.fn(),
      }),
    );
    const wh = $<HTMLSelectElement>("entry-warehouse");
    // Inactive warehouses are not offered.
    expect(Array.from(wh?.options ?? []).map((o) => o.value)).toEqual([
      "",
      "w1",
    ]);
    const mode = $<HTMLSelectElement>("entry-mode");
    expect(Array.from(mode?.options ?? []).map((o) => o.value)).toEqual([
      "with_existing",
    ]);

    await click($("entry-create"));
    expect(text()).toContain("warehouse.validation.warehouse");
    expect(api.createEntry).not.toHaveBeenCalled();
  });

  it("the center opens a generate_new entry and goes to it", async () => {
    api.listWarehouses.mockResolvedValue(warehouses);
    api.createEntry.mockResolvedValue({ uuid: "e1" });
    await render(
      m,
      createElement(NewEntryForm, {
        slug: "acme",
        isCenter: true,
        onCancel: vi.fn(),
      }),
    );
    await fill($<HTMLSelectElement>("entry-warehouse"), "w1");
    expect($<HTMLSelectElement>("entry-mode")?.value).toBe("generate_new");
    await fill($("entry-note"), "x".repeat(501));
    await click($("entry-create"));
    expect(text()).toContain("warehouse.validation.too_long");
    expect(api.createEntry).not.toHaveBeenCalled();

    await fill($("entry-note"), "Container 12");
    await click($("entry-create"));
    expect(api.createEntry).toHaveBeenCalledWith({
      warehouse_uuid: "w1",
      mode: "generate_new",
      note: "Container 12",
    });
    expect(nav.push).toHaveBeenCalledWith("/t/acme/warehouse/entries/e1");
  });
});

describe("GenerateForm validation (TEC-231)", () => {
  const roll = {
    uuid: "p-roll",
    sku: "PPF-190",
    name: "Olex PPF 190",
    unit_type: "roll_meter",
  };
  const kit = { uuid: "p-kit", sku: "KIT", name: "Kit", unit_type: "piece" };

  it("needs a product and a quantity in range; a roll needs meters", async () => {
    catalog.listProducts.mockResolvedValue({ items: [roll, kit], total: 2 });
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    await render(
      m,
      createElement(GenerateForm, { submitLabel: "go", onSubmit }),
    );

    await click($("generate-submit"));
    expect(text()).toContain("warehouse.validation.product");
    expect(onSubmit).not.toHaveBeenCalled();

    await fill($<HTMLSelectElement>("generate-product"), "p-roll");
    await fill($("generate-quantity"), "1001");
    await click($("generate-submit"));
    expect(text()).toContain("warehouse.validation.quantity");
    expect(text()).toContain("warehouse.validation.meters");
    expect(onSubmit).not.toHaveBeenCalled();

    await fill($("generate-quantity"), "5");
    await fill($("generate-meters"), "15.00");
    await fill($("generate-prefix"), "olx");
    await click($("generate-submit"));
    expect(onSubmit).toHaveBeenCalledWith({
      product_uuid: "p-roll",
      quantity: 5,
      meters: "15.00",
      prefix: "OLX",
    });
  });

  it("a piece product sends no meters and rejects a bad prefix", async () => {
    catalog.listProducts.mockResolvedValue({ items: [roll, kit], total: 2 });
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    await render(
      m,
      createElement(GenerateForm, { submitLabel: "go", onSubmit }),
    );
    await fill($<HTMLSelectElement>("generate-product"), "p-kit");
    expect($("generate-meters")).toBeNull();
    await fill($("generate-prefix"), "O");
    await click($("generate-submit"));
    expect(text()).toContain("warehouse.validation.prefix");
    expect(onSubmit).not.toHaveBeenCalled();

    await fill($("generate-prefix"), "");
    await click($("generate-submit"));
    expect(onSubmit).toHaveBeenCalledWith({
      product_uuid: "p-kit",
      quantity: 1,
    });
  });
});
