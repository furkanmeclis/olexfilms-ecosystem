// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const toastWarning = vi.fn();

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: {
    warning: (m: string) => toastWarning(m),
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
  },
}));

import { VehicleBrandLogo } from "./vehicle-brand-logo";
import { VehicleImageField } from "./vehicle-image-field";

const UUID = "0b5c4a39-6a43-4d47-9a3f-1f3a2b4c5d6e";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  toastWarning.mockReset();
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

function pick(file: File) {
  const input = container.querySelector<HTMLInputElement>('input[type="file"]');
  expect(input).not.toBeNull();
  Object.defineProperty(input!, "files", {
    value: [file],
    configurable: true,
  });
  act(() => {
    input!.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

describe("VehicleImageField", () => {
  it("warns about SVG and never uploads it", () => {
    const onUpload = vi.fn();
    act(() =>
      root.render(
        createElement(VehicleImageField, {
          kind: "logo",
          label: "Logo",
          hasImage: false,
          onUpload,
        }),
      ),
    );

    pick(new File(["<svg/>"], "bmw.svg", { type: "image/svg+xml" }));

    expect(onUpload).not.toHaveBeenCalled();
    expect(toastWarning).toHaveBeenCalledWith("vehicles.image.svg_refused");
    const warning = container.querySelector(
      '[data-slot="vehicle-image-warning"]',
    );
    expect(warning?.textContent).toContain("vehicles.image.svg_refused");
  });

  it("uploads a PNG and clears the warning", () => {
    const onUpload = vi.fn();
    act(() =>
      root.render(
        createElement(VehicleImageField, {
          kind: "logo",
          label: "Logo",
          hasImage: false,
          onUpload,
        }),
      ),
    );
    pick(new File(["<svg/>"], "bmw.svg", { type: "image/svg+xml" }));
    const png = new File(["png"], "bmw.png", { type: "image/png" });
    pick(png);

    expect(onUpload).toHaveBeenCalledWith(png);
    expect(
      container.querySelector('[data-slot="vehicle-image-warning"]'),
    ).toBeNull();
  });

  it("accepts only raster types in the picker", () => {
    act(() =>
      root.render(
        createElement(VehicleImageField, {
          kind: "hero",
          label: "Hero",
          hasImage: false,
          onUpload: vi.fn(),
        }),
      ),
    );
    const input =
      container.querySelector<HTMLInputElement>('input[type="file"]');
    expect(input?.accept).toBe("image/jpeg,image/png,image/webp");
  });
});

describe("VehicleBrandLogo", () => {
  it("renders a lazy img on the fixed /brand-logos URL", () => {
    act(() =>
      root.render(
        createElement(VehicleBrandLogo, {
          uuid: UUID,
          name: "BMW",
          version: "v1",
        }),
      ),
    );
    const img = container.querySelector("img");
    expect(img?.getAttribute("src")).toBe(`/brand-logos/${UUID}?v=v1`);
    expect(img?.getAttribute("loading")).toBe("lazy");
    expect(img?.getAttribute("alt")).toBe("BMW");
  });

  it("shows the placeholder without a uuid or after a load error", () => {
    act(() => root.render(createElement(VehicleBrandLogo, { name: "BMW" })));
    expect(container.querySelector("img")).toBeNull();
    expect(
      container.querySelector('[data-slot="vehicle-brand-logo-placeholder"]'),
    ).not.toBeNull();

    act(() => root.render(createElement(VehicleBrandLogo, { uuid: UUID })));
    const img = container.querySelector("img");
    act(() => {
      img!.dispatchEvent(new Event("error"));
    });
    expect(container.querySelector("img")).toBeNull();
    expect(
      container.querySelector('[data-slot="vehicle-brand-logo-placeholder"]'),
    ).not.toBeNull();
  });
});
