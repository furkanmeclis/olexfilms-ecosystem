// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import en from "@/locales/en/imports.json";
import type { ImportJob, ImportPreviewSummary } from "@/features/io/types";

const catalog = en as Record<string, string>;
const fill = (text: string, params?: Record<string, string | number>) =>
  text.replace(/{{\s*(\w+)\s*}}/g, (_, k: string) => String(params?.[k] ?? ""));
const t = (key: string, params?: Record<string, string | number>) => {
  const text = key.startsWith("imports.")
    ? catalog[key.slice("imports.".length)]
    : undefined;
  return text === undefined ? key : fill(text, params);
};
const tp = (
  key: string,
  count: number,
  params?: Record<string, string | number>,
) => t(`${key}_${count === 1 ? "one" : "other"}`, { count, ...params });

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t,
    tp,
    locale: "en",
    format: { dateTime: (v: string) => v, date: (v: string) => v },
  }),
}));
vi.mock("@/hooks/use-mobile", () => ({
  useIsMobile: () => false,
  useIsXl: () => true,
}));
vi.mock("next/navigation", () => ({
  useParams: () => ({ slug: "olex" }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

const JOB = "0b9c4c1e-0000-4000-8000-000000000759";

/** Stock import preview as the staged importer returns it (TEC-158). */
const preview: ImportPreviewSummary = {
  total: 3,
  valid: 2,
  invalid: 1,
  batch_uuid: "0b9c4c1e-0000-4000-8000-00000000b759",
  counts: {
    new: 2,
    duplicate: 0,
    invalid: 1,
    conflict: 0,
    applied: 0,
    undone: 0,
    undo_rejected: 0,
  },
  rows: [1, 2].map((index) => ({
    index,
    data: { barcode: `OLX-${index}`, product_sku: "PPF-GLOSS" },
    status: "new",
    status_label: "New",
    target: {
      organization_name: "Merkez Depo",
      owner_type: "warehouse_location",
      location_code: "A-01",
      product_sku: "PPF-GLOSS",
      product_name: "Olex PPF Gloss",
    },
  })),
  errors: [
    {
      index: 3,
      field: "product_sku",
      code: "STOCK_IMPORT_PRODUCT_NOT_FOUND",
      error: "Product not found",
    },
  ],
};

const job = (status: string, summary?: ImportPreviewSummary): ImportJob => ({
  uuid: JOB,
  resource: "tenant.stock.units",
  format: "csv",
  status,
  mapping: { barcode: "barcode" },
  preview_summary: summary,
  created_at: "2026-10-16T09:00:00Z",
});

const service = vi.hoisted(() => ({
  upload: vi.fn(),
  updateMapping: vi.fn(),
  preview: vi.fn(),
  confirm: vi.fn(),
  get: vi.fn(),
  rollback: vi.fn(),
  downloadSample: vi.fn(),
  errorReport: vi.fn(),
}));
vi.mock("@/features/io/services/imports.service", () => ({
  importsService: service,
}));

import { ImportWizard } from "@/features/io/components/import-wizard";
import { STOCK_IMPORT_PATHS } from "@/features/io/types";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

// jsdom's Blob has no text(); browsers read the file head with it.
if (!Blob.prototype.text) {
  Blob.prototype.text = function text(this: Blob) {
    return new Promise<string>((resolve) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result));
      reader.readAsText(this);
    });
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  service.upload.mockResolvedValue(job("uploaded"));
  service.updateMapping.mockResolvedValue(job("mapped"));
  service.preview.mockResolvedValue(job("previewed", preview));
  service.confirm.mockResolvedValue(job("applied", preview));
  service.get.mockResolvedValue(
    job("applied", { ...preview, counts: { applied: 2, invalid: 1 } }),
  );
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.clearAllMocks();
});

const byTestId = (id: string) =>
  document.querySelector<HTMLElement>(`[data-testid="${id}"]`);

async function flush() {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

async function click(el: HTMLElement | null) {
  expect(el).not.toBeNull();
  await act(async () => {
    el!.click();
  });
  await flush();
}

describe("ImportWizard with the stock import preview", () => {
  it("walks file → columns → preview → confirm → result", async () => {
    const onComplete = vi.fn();
    act(() =>
      root.render(
        createElement(
          QueryClientProvider,
          { client: new QueryClient() },
          createElement(ImportWizard, {
            resource: "tenant.stock.units",
            paths: STOCK_IMPORT_PATHS,
            scope: "tenant",
            open: true,
            onOpenChange: () => undefined,
            onComplete,
            initialOptions: { barcode_policy: "skip" },
            extraOptions: ({ values }) =>
              createElement(
                "p",
                { "data-testid": "extra" },
                `policy:${values.barcode_policy}`,
              ),
          }),
        ),
      ),
    );
    expect(byTestId("extra")?.textContent).toBe("policy:skip");

    const input = document.querySelector<HTMLInputElement>("#import-file")!;
    const file = new File(
      ["barcode,SKU\nOLX-1,PPF-GLOSS\nOLX-2,PPF-GLOSS\nOLX-3,NOPE\n"],
      "stock.csv",
      { type: "text/csv" },
    );
    Object.defineProperty(input, "files", { value: [file] });
    await act(async () => {
      input.dispatchEvent(new Event("change", { bubbles: true }));
    });
    expect(byTestId("import-detected-format")?.textContent).toBe("CSV");

    await click(byTestId("import-next"));
    expect(service.upload).toHaveBeenCalledWith(
      "/v1/stock/import",
      file,
      "csv",
      "en",
      { barcode_policy: "skip" },
    );
    const sku = document.querySelector('tr[data-field="product_sku"]');
    expect(sku?.textContent).toContain("PPF-GLOSS · PPF-GLOSS · NOPE");

    await click(byTestId("import-next"));
    expect(service.updateMapping).toHaveBeenCalledWith(
      JOB,
      {
        mapping: { barcode: "barcode", SKU: "product_sku" },
        defaults: { barcode_policy: "skip" },
      },
      "tenant",
    );
    expect(byTestId("import-status-card-new")?.textContent).toContain("2");
    expect(document.body.textContent).toContain("Product not found");
    expect(document.body.textContent).toContain("Location: A-01");

    await click(byTestId("import-next"));
    expect(byTestId("import-confirm-sentence")?.textContent).toBe(
      "2 units will be added to the Merkez Depo stock and a single stock entry document will be created. 1 row will be skipped.",
    );
    const confirm = byTestId("import-confirm") as HTMLButtonElement;
    expect(confirm.textContent).toBe("Import 2 units");
    expect(confirm.disabled).toBe(true);
    await click(byTestId("import-ack-skipped"));
    expect(confirm.disabled).toBe(false);

    await click(confirm);
    expect(service.confirm).toHaveBeenCalledWith(JOB, "tenant");
    expect(byTestId("import-result")?.dataset.status).toBe("applied");
    expect(byTestId("import-result-detail")?.getAttribute("href")).toBe(
      `/t/olex/imports/${JOB}`,
    );
    expect(onComplete).toHaveBeenCalled();
  });
});
