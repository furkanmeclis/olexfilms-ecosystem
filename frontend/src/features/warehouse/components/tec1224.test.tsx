// @vitest-environment jsdom
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const req = vi.hoisted(() => ({ platformRequest: vi.fn() }));
const files = vi.hoisted(() => ({
  platformDownloadFile: vi.fn(),
  triggerBrowserDownload: vi.fn(),
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock("@/lib/api/platform-request", () => req);
vi.mock("@/lib/api/platform-form-request", () => files);
vi.mock("@/providers/toast-provider", () => ({ appToast: toast }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ locale: "en", t: (key: string) => key }),
}));

import {
  MAX_UNIT_LABELS,
  stockService,
} from "@/features/stock/services/stock.service";
import { LabelButton } from "./label-button";
import { click, mount, render, unmount, type Mounted } from "./test-helpers";

let m: Mounted;
beforeEach(() => {
  m = mount();
});
afterEach(() => {
  unmount(m);
  vi.clearAllMocks();
});

describe("TEC-1224 label print confirmation", () => {
  it("marks printed with POST in chunks of the sheet cap", async () => {
    req.platformRequest.mockResolvedValue({ marked: 2 });
    const codes = Array.from(
      { length: MAX_UNIT_LABELS + 1 },
      (_, i) => `B${i}`,
    );
    const marked = await stockService.markUnitsPrinted(codes);
    expect(marked).toBe(4);
    expect(req.platformRequest).toHaveBeenNthCalledWith(
      1,
      "POST",
      "/v1/stock/labels/units/mark-printed",
      { body: { barcodes: codes.slice(0, MAX_UNIT_LABELS) } },
    );
    expect(req.platformRequest).toHaveBeenNthCalledWith(
      2,
      "POST",
      "/v1/stock/labels/units/mark-printed",
      { body: { barcodes: [`B${MAX_UNIT_LABELS}`] } },
    );
  });

  it("calls onPrinted only after the sheet was downloaded", async () => {
    const onPrinted = vi.fn();
    files.platformDownloadFile.mockResolvedValueOnce({
      blob: new Blob(["%PDF"]),
      filename: null,
    });
    await render(
      m,
      createElement(LabelButton, {
        path: "/v1/stock/labels/units.pdf?barcode=A",
        filename: "a.pdf",
        testId: "lbl",
        onPrinted,
      }),
    );
    await click(m.container.querySelector('[data-testid="lbl"]'));
    expect(files.triggerBrowserDownload).toHaveBeenCalled();
    expect(onPrinted).toHaveBeenCalledTimes(1);

    files.platformDownloadFile.mockRejectedValueOnce(new Error("boom"));
    await click(m.container.querySelector('[data-testid="lbl"]'));
    expect(onPrinted).toHaveBeenCalledTimes(1);
    expect(toast.error).toHaveBeenCalledWith("warehouse.labels.failed");
  });
});
