// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  upload: vi.fn(),
  remove: vi.fn(),
  reorder: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: mocks.toastError },
}));
vi.mock("@/features/catalog/services/catalog.service", async (orig) => ({
  ...(await orig<object>()),
  catalogService: {
    uploadProductImage: mocks.upload,
    deleteProductImage: mocks.remove,
    reorderProductImages: mocks.reorder,
  },
}));

import type { CatalogProduct } from "@/features/catalog/services/catalog.service";

import { moveKey, ProductImagesField } from "./product-images-field";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const A = `${"a".repeat(32)}.png`;
const B = `${"b".repeat(32)}.webp`;

function product(images: CatalogProduct["images"]): CatalogProduct {
  return {
    uuid: "p1",
    category: { uuid: "c1", name: "PPF" },
    sku: "PPF-190",
    name: "Olex PPF 190",
    description_md: "",
    warranty_duration_months: null,
    micron_thickness: null,
    images,
    unit_type: "piece",
    uses_fixed_barcode: false,
    active: true,
    external_id: null,
    locked_fields: [],
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-01T00:00:00Z",
  };
}

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

async function render(p?: CatalogProduct) {
  const client = new QueryClient();
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(ProductImagesField, { product: p }),
      ),
    );
  });
}

async function pick(file: File) {
  const input = container.querySelector<HTMLInputElement>(
    "[data-testid=product-image-input]",
  );
  if (!input) throw new Error("no file input");
  Object.defineProperty(input, "files", { value: [file], configurable: true });
  await act(async () => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

describe("ProductImagesField", () => {
  it("asks to save a new product before uploads", async () => {
    await render(undefined);
    expect(container.textContent).toContain("catalog.images.save_first");
  });

  it("previews uploaded images through the public URL in sort order", async () => {
    await render(
      product([
        { key: B, sort: 1 },
        { key: A, sort: 0 },
        { key: "legacy/key.jpg", sort: 2 },
      ]),
    );
    const srcs = Array.from(container.querySelectorAll("img")).map((img) =>
      img.getAttribute("src"),
    );
    expect(srcs).toEqual([`/product-images/${A}`, `/product-images/${B}`]);
    expect(container.textContent).toContain("catalog.images.cover");
  });

  it("refuses SVG before upload and uploads a PNG", async () => {
    mocks.upload.mockResolvedValue(product([{ key: A, sort: 0 }]));
    await render(product([]));
    await pick(new File(["<svg/>"], "x.svg", { type: "image/svg+xml" }));
    expect(mocks.upload).not.toHaveBeenCalled();
    expect(mocks.toastError).toHaveBeenCalledWith(
      "catalog.images.invalid_type",
    );

    const png = new File([new Uint8Array([137, 80, 78, 71])], "x.png", {
      type: "image/png",
    });
    await pick(png);
    expect(mocks.upload).toHaveBeenCalledWith("p1", png);
  });

  it("reorders with the move buttons", async () => {
    mocks.reorder.mockResolvedValue(
      product([
        { key: B, sort: 0 },
        { key: A, sort: 1 },
      ]),
    );
    await render(
      product([
        { key: A, sort: 0 },
        { key: B, sort: 1 },
      ]),
    );
    const later = container.querySelector<HTMLButtonElement>(
      "button[aria-label='catalog.images.move_later']",
    );
    await act(async () => later?.click());
    expect(mocks.reorder).toHaveBeenCalledWith("p1", [B, A]);
  });
});

describe("moveKey", () => {
  it("moves one key and clamps the target", () => {
    expect(moveKey(["a", "b", "c"], 0, 2)).toEqual(["b", "c", "a"]);
    expect(moveKey(["a", "b", "c"], 2, 0)).toEqual(["c", "a", "b"]);
    expect(moveKey(["a", "b"], 0, 9)).toEqual(["b", "a"]);
  });
});
