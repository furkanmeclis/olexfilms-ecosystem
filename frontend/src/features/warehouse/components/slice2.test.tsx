// @vitest-environment jsdom
import { act, createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  listWarehouses: vi.fn(),
  listRooms: vi.fn(),
  listLocations: vi.fn(),
  createTransfer: vi.fn(),
  getTransfer: vi.fn(),
  addTransferLines: vi.fn(),
  deleteTransferLine: vi.fn(),
  placeTransfer: vi.fn(),
  shipTransfer: vi.fn(),
  completeTransfer: vi.fn(),
  cancelTransfer: vi.fn(),
  move: vi.fn(),
  createCount: vi.fn(),
  scanCount: vi.fn(),
  getCountReport: vi.fn(),
  approveCount: vi.fn(),
}));
const catalog = vi.hoisted(() => ({ listProducts: vi.fn() }));
const nav = vi.hoisted(() => ({ push: vi.fn() }));
const dialogs = vi.hoisted(() => ({ confirm: vi.fn() }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
const files = vi.hoisted(() => ({
  platformDownloadFile: vi.fn(),
  triggerBrowserDownload: vi.fn(),
}));
const req = vi.hoisted(() => ({ platformRequest: vi.fn() }));

vi.mock("next/navigation", () => ({ useRouter: () => nav }));
vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: { href: string; children: unknown } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    t: (key: string, params?: Record<string, string | number>) =>
      key.startsWith("warehouse.errors.")
        ? `ERR ${key.slice("warehouse.errors.".length)}`
        : params
          ? `${key} ${JSON.stringify(params)}`
          : key,
    format: {
      timeZone: "Europe/Istanbul",
      number: (v: number) => String(v),
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({
    uuid: "org-1",
    slug: "acme",
    type: "center",
  }),
}));
vi.mock("@/providers/dialog-provider", () => ({ useDialogs: () => dialogs }));
vi.mock("@/providers/toast-provider", () => ({ appToast: toast }));
vi.mock("@/hooks/use-mobile", () => ({
  useIsMobile: () => false,
  useIsXl: () => true,
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/lib/api/platform-form-request", () => files);
vi.mock("@/lib/api/platform-request", () => req);
vi.mock("@/features/warehouse/services/warehouse.service", async (orig) => ({
  ...(await orig<object>()),
  warehouseService: api,
}));
vi.mock("@/features/catalog/services/catalog.service", async (orig) => ({
  ...(await orig<object>()),
  catalogService: catalog,
}));

import { ApiError } from "@/lib/api";

import type {
  StockCount,
  StockCountLine,
  WarehouseTransfer,
} from "../services/warehouse.service";
import { CountReportCard } from "./count-detail-page";
import { NewCountForm } from "./counts-page";
import { EodPdfButton } from "./eod-report-detail-page";
import { TransferDetailPage } from "./transfer-detail-page";
import { BinMoveCard, NewTransferForm } from "./transfers-page";
import {
  click,
  fill,
  flush,
  mount,
  render,
  submit,
  unmount,
  type Mounted,
} from "./test-helpers";

let m: Mounted;
beforeEach(() => {
  m = mount();
  api.listWarehouses.mockResolvedValue({
    items: [
      { uuid: "wh-1", code: "WH1", name: "Main", active: true },
      { uuid: "wh-2", code: "WH2", name: "Second", active: true },
    ],
  });
  api.listRooms.mockResolvedValue({
    items: [{ uuid: "room-1", code: "R1", name: "R1" }],
  });
  api.listLocations.mockResolvedValue({ items: [] });
  catalog.listProducts.mockResolvedValue({
    items: [{ uuid: "p-1", sku: "PPF-190", name: "Olex PPF 190" }],
  });
  dialogs.confirm.mockResolvedValue(true);
});
afterEach(() => {
  unmount(m);
  vi.clearAllMocks();
});

const $ = <T extends Element = HTMLInputElement>(testId: string) =>
  m.container.querySelector<T>(`[data-testid="${testId}"]`);
const text = () => m.container.textContent ?? "";

async function scan(id: string, code: string) {
  await fill($(`${id}-input`), code);
  await submit($(`${id}-input`)?.closest("form") ?? null);
}

const ROUTE = {
  from_warehouse: { uuid: "wh-1", code: "WH1", name: "Main" },
  to_warehouse: { uuid: "wh-2", code: "WH2", name: "Second" },
};

function transfer(over: Partial<WarehouseTransfer> = {}): WarehouseTransfer {
  return {
    uuid: "tr-1",
    transfer_no: "WT-0001",
    status: "draft",
    note: null,
    ...ROUTE,
    to_location: null,
    line_count: 0,
    lines: [],
    created_at: "2026-10-01T09:00:00Z",
    shipped_at: null,
    completed_at: null,
    cancelled_at: null,
    ...over,
  };
}

const LINE = {
  uuid: "line-1",
  unit_uuid: "u-1",
  barcode: "OLEX-00000001",
  unit_status: "placed",
  product: { uuid: "p-1", sku: "PPF-190", name: "Olex PPF 190" },
  source_location: { uuid: "loc-1", code: "01", full_code: "WH1-R1-A-S1-01" },
  target_location: null,
  out_movement_uuid: null,
  in_movement_uuid: null,
  placement_movement_uuid: null,
  restore_movement_uuid: null,
};

describe("NewTransferForm (TEC-232)", () => {
  it("refuses the same warehouse on both sides without calling the API", async () => {
    await render(
      m,
      createElement(NewTransferForm, { slug: "acme", onCancel: vi.fn() }),
    );
    await fill($<HTMLSelectElement>("transfer-from"), "wh-1");
    await fill($<HTMLSelectElement>("transfer-to"), "wh-1");
    await submit($("transfer-form")?.querySelector("form") ?? null);
    expect(text()).toContain("warehouse.validation.same_warehouse");
    expect(api.createTransfer).not.toHaveBeenCalled();
  });

  it("requires both warehouses", async () => {
    await render(
      m,
      createElement(NewTransferForm, { slug: "acme", onCancel: vi.fn() }),
    );
    await submit($("transfer-form")?.querySelector("form") ?? null);
    expect(text()).toContain("warehouse.validation.warehouse");
    expect(api.createTransfer).not.toHaveBeenCalled();
  });

  it("opens the draft and goes to its page", async () => {
    api.createTransfer.mockResolvedValue(transfer());
    await render(
      m,
      createElement(NewTransferForm, { slug: "acme", onCancel: vi.fn() }),
    );
    await fill($<HTMLSelectElement>("transfer-from"), "wh-1");
    await fill($<HTMLSelectElement>("transfer-to"), "wh-2");
    await fill($("transfer-note"), " Van 3 ");
    await submit($("transfer-form")?.querySelector("form") ?? null);
    expect(api.createTransfer).toHaveBeenCalledWith({
      from_warehouse_uuid: "wh-1",
      to_warehouse_uuid: "wh-2",
      note: "Van 3",
    });
    expect(nav.push).toHaveBeenCalledWith("/t/acme/warehouse/transfers/tr-1");
  });
});

describe("TransferDetailPage (TEC-232)", () => {
  it("draft: ship is blocked without lines; a unit QR adds its barcode", async () => {
    api.getTransfer.mockResolvedValue(transfer());
    api.addTransferLines.mockResolvedValue(
      transfer({ lines: [LINE], line_count: 1 }),
    );
    await render(
      m,
      createElement(TransferDetailPage, { slug: "acme", uuid: "tr-1" }),
    );
    expect($("transfer-status")?.getAttribute("data-status")).toBe("draft");
    expect($<HTMLButtonElement>("transfer-ship")?.disabled).toBe(true);
    expect($("transfer-complete")).toBeNull();

    // A location QR in the unit field is refused locally.
    await scan("transfer-barcode", "OFW:LOC:WH1-R1-A-S1-01");
    expect($("transfer-line-error")?.textContent).toBe(
      "warehouse.entry.scan_location_here",
    );
    expect(api.addTransferLines).not.toHaveBeenCalled();

    await scan("transfer-barcode", "ofw:unit:OLEX-00000001");
    expect(api.addTransferLines).toHaveBeenCalledWith("tr-1", [
      "OLEX-00000001",
    ]);
    await flush();
    expect(
      m.container.querySelectorAll('[data-testid="transfer-line"]'),
    ).toHaveLength(1);
    expect($<HTMLButtonElement>("transfer-ship")?.disabled).toBe(false);
  });

  it("shows the server's error code for a busy unit", async () => {
    api.getTransfer.mockResolvedValue(transfer());
    api.addTransferLines.mockRejectedValue(
      new ApiError({
        status: 409,
        code: "WAREHOUSE_UNIT_BUSY",
        message: "busy",
      }),
    );
    await render(
      m,
      createElement(TransferDetailPage, { slug: "acme", uuid: "tr-1" }),
    );
    await scan("transfer-barcode", "OLEX-00000009");
    expect($("transfer-line-error")?.textContent).toBe(
      "ERR WAREHOUSE_UNIT_BUSY",
    );
  });

  it("in transit: a location QR targets the open lines, then receive", async () => {
    const inTransit = transfer({
      status: "in_transit",
      lines: [LINE],
      line_count: 1,
    });
    const placed = transfer({
      status: "in_transit",
      line_count: 1,
      lines: [
        {
          ...LINE,
          target_location: {
            uuid: "loc-9",
            code: "02",
            full_code: "WH2-R1-A-S1-02",
          },
        },
      ],
    });
    api.getTransfer.mockResolvedValue(inTransit);
    api.placeTransfer.mockResolvedValue(placed);
    api.completeTransfer.mockResolvedValue({
      ...placed,
      status: "completed",
    });
    await render(
      m,
      createElement(TransferDetailPage, { slug: "acme", uuid: "tr-1" }),
    );
    expect($("transfer-barcode-input")).toBeNull();
    expect($<HTMLButtonElement>("transfer-complete")?.disabled).toBe(true);

    await scan("transfer-location", "OFW:LOC:WH2-R1-A-S1-02");
    expect(api.placeTransfer).toHaveBeenCalledWith("tr-1", {
      location_code: "OFW:LOC:WH2-R1-A-S1-02",
      line_uuids: ["line-1"],
    });
    await flush();
    expect($("transfer-line-target")?.textContent).toBe("WH2-R1-A-S1-02");

    await click($("transfer-complete"));
    expect(dialogs.confirm).toHaveBeenCalled();
    expect(api.completeTransfer).toHaveBeenCalledWith("tr-1");
    await flush();
    expect($("transfer-status")?.getAttribute("data-status")).toBe("completed");
    expect($("transfer-cancel")).toBeNull();
  });
});

describe("TransferDetailPage lines table (TEC-376)", () => {
  it("in transit: the selected lines are placed instead of every open line", async () => {
    const second = { ...LINE, uuid: "line-2", barcode: "OLEX-00000002" };
    const inTransit = transfer({
      status: "in_transit",
      lines: [LINE, second],
      line_count: 2,
    });
    api.getTransfer.mockResolvedValue(inTransit);
    api.placeTransfer.mockResolvedValue(inTransit);
    await render(
      m,
      createElement(TransferDetailPage, { slug: "acme", uuid: "tr-1" }),
    );
    expect(
      m.container.querySelectorAll('[data-testid="transfer-line"]'),
    ).toHaveLength(2);
    const boxes = m.container.querySelectorAll<HTMLButtonElement>(
      'button[aria-label="table.select_row"]',
    );
    expect(boxes).toHaveLength(2);
    await click(boxes[1]);
    expect(m.container.textContent).toContain(
      'warehouse.entry.place_selected {"count":1}',
    );
    await scan("transfer-location", "OFW:LOC:WH2-R1-B");
    expect(api.placeTransfer).toHaveBeenCalledWith("tr-1", {
      location_code: "OFW:LOC:WH2-R1-B",
      line_uuids: ["line-2"],
    });
  });
});

describe("BinMoveCard (TEC-232)", () => {
  it("needs units first, then moves every listed unit to the scanned bin", async () => {
    api.move.mockResolvedValue({
      location: { uuid: "loc-2", code: "02", full_code: "WH1-R1-A-S1-02" },
      units: [
        {
          unit_uuid: "u-1",
          barcode: "A1",
          from_location: null,
          movement_uuid: "m1",
        },
        {
          unit_uuid: "u-2",
          barcode: "A2",
          from_location: null,
          movement_uuid: "m2",
        },
      ],
    });
    await render(m, createElement(BinMoveCard));
    await scan("move-location", "OFW:LOC:WH1-R1-A-S1-02");
    expect($("move-error")?.textContent).toBe("warehouse.move.no_units");
    expect(api.move).not.toHaveBeenCalled();

    const area = $<HTMLTextAreaElement>("move-units");
    await act(async () => {
      Object.getOwnPropertyDescriptor(
        HTMLTextAreaElement.prototype,
        "value",
      )?.set?.call(area, "A1\nOFW:UNIT:A2, A1");
      area?.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await scan("move-location", "OFW:LOC:WH1-R1-A-S1-02");
    expect(api.move).toHaveBeenCalledWith({
      barcodes: ["A1", "A2"],
      location_code: "OFW:LOC:WH1-R1-A-S1-02",
    });
    expect($("move-result")?.textContent).toContain('"count":2');
  });
});

describe("NewCountForm (TEC-232)", () => {
  const form = () => $("count-form")?.querySelector("form") ?? null;

  it("asks for the scope target before calling the API", async () => {
    await render(
      m,
      createElement(NewCountForm, { slug: "acme", onCancel: vi.fn() }),
    );
    await submit(form());
    expect(text()).toContain("warehouse.validation.warehouse");

    await fill($<HTMLSelectElement>("count-warehouse"), "wh-1");
    await fill($<HTMLSelectElement>("count-scope"), "room");
    await submit(form());
    expect(text()).toContain("warehouse.validation.room");
    expect(api.createCount).not.toHaveBeenCalled();
  });

  it("initial placement offers no product scope", async () => {
    await render(
      m,
      createElement(NewCountForm, { slug: "acme", onCancel: vi.fn() }),
    );
    const options = () =>
      [...($<HTMLSelectElement>("count-scope")?.options ?? [])].map(
        (o) => o.value,
      );
    expect(options()).toContain("product");
    await fill($<HTMLSelectElement>("count-method"), "initial_placement");
    expect(options()).toEqual(["warehouse", "room", "location"]);
  });

  it("sends only the chosen scope's target", async () => {
    api.createCount.mockResolvedValue({ uuid: "c-1" });
    await render(
      m,
      createElement(NewCountForm, { slug: "acme", onCancel: vi.fn() }),
    );
    await fill($<HTMLSelectElement>("count-warehouse"), "wh-1");
    await fill($<HTMLSelectElement>("count-method"), "product_qty");
    await fill($<HTMLSelectElement>("count-visibility"), "guided");
    await fill($<HTMLSelectElement>("count-scope"), "product");
    await flush();
    await fill($<HTMLSelectElement>("count-product"), "p-1");
    await submit(form());
    expect(api.createCount).toHaveBeenCalledWith({
      warehouse_uuid: "wh-1",
      method: "product_qty",
      visibility: "guided",
      scope_type: "product",
      product_uuid: "p-1",
      note: null,
    });
    expect(nav.push).toHaveBeenCalledWith("/t/acme/warehouse/counts/c-1");
  });
});

function countLine(over: Partial<StockCountLine>): StockCountLine {
  return {
    uuid: "l-1",
    line_kind: "serial",
    unit: { uuid: "u-1", barcode: "OLEX-1", unit_kind: "serial" },
    product: { uuid: "p-1", sku: "PPF-190", name: "Olex PPF 190" },
    expected_location: null,
    counted_location: null,
    expected_quantity: 1,
    counted_quantity: 0,
    expected_meters: null,
    counted_meters: null,
    result: "missing",
    resolution: null,
    allowed_resolutions: ["ignore", "void_missing"],
    note: null,
    resolved_at: null,
    ...over,
  };
}

const COUNT = {
  uuid: "c-1",
  status: "pending_review",
  summary: { lines: 3, by_result: {}, unresolved: 2 },
} as unknown as StockCount;

describe("CountReportCard (TEC-232)", () => {
  it("resolves every difference: ignore by default, the chosen one otherwise", async () => {
    const lines = [
      countLine({ uuid: "l-1" }),
      countLine({
        uuid: "l-2",
        result: "wrong_location",
        allowed_resolutions: ["ignore", "relocate"],
      }),
      countLine({ uuid: "l-3", result: "matched", allowed_resolutions: [] }),
    ];
    api.getCountReport.mockResolvedValue({ count: COUNT, lines });
    api.approveCount.mockResolvedValue({
      count: { ...COUNT, status: "approved" },
      lines,
    });
    await render(
      m,
      createElement(CountReportCard, { count: COUNT, canApprove: true }),
    );
    // The matched line needs no resolution.
    const rows = m.container.querySelectorAll('[data-testid="count-line"]');
    expect(rows).toHaveLength(2);
    const selects = m.container.querySelectorAll<HTMLSelectElement>(
      '[data-testid="count-resolution"]',
    );
    expect(selects[0].value).toBe("ignore");
    await fill(selects[1], "relocate");
    await click($("count-approve"));
    expect(api.approveCount).toHaveBeenCalledWith("c-1", {
      resolutions: [
        { line_uuid: "l-1", resolution: "ignore" },
        { line_uuid: "l-2", resolution: "relocate" },
      ],
    });
    expect(toast.success).toHaveBeenCalledWith("warehouse.count.approved");
  });

  it("shows COUNT_STALE from the server", async () => {
    api.getCountReport.mockResolvedValue({
      count: COUNT,
      lines: [countLine({})],
    });
    api.approveCount.mockRejectedValue(
      new ApiError({ status: 409, code: "COUNT_STALE", message: "stale" }),
    );
    await render(
      m,
      createElement(CountReportCard, { count: COUNT, canApprove: true }),
    );
    await click($("count-approve"));
    await flush();
    expect($("count-approve-error")?.textContent).toBe("ERR COUNT_STALE");
  });

  it("read-only without the approval right", async () => {
    api.getCountReport.mockResolvedValue({
      count: COUNT,
      lines: [countLine({})],
    });
    await render(
      m,
      createElement(CountReportCard, { count: COUNT, canApprove: false }),
    );
    expect($("count-approve")).toBeNull();
    expect($("count-resolution")).toBeNull();
    expect($("count-export")).not.toBeNull();
  });
});

describe("EodPdfButton (TEC-232)", () => {
  it("queues the PDF, polls the job and downloads it", async () => {
    const job = (status: string) => ({
      uuid: "job-1",
      resource: "eod_report",
      format: "pdf",
      status,
      row_count: 0,
      created_at: "2026-10-01T09:00:00Z",
    });
    req.platformRequest
      .mockResolvedValueOnce(job("queued"))
      .mockResolvedValueOnce(job("completed"));
    const blob = new Blob(["%PDF"]);
    files.platformDownloadFile.mockResolvedValue({ blob, filename: null });
    await render(
      m,
      createElement(EodPdfButton, {
        reportUuid: "r-1",
        filename: "eod-2026-10-01.pdf",
        waitOptions: { sleep: () => Promise.resolve() },
      }),
    );
    await click($("eod-pdf"));
    await flush();
    expect(req.platformRequest).toHaveBeenNthCalledWith(
      1,
      "POST",
      "/v1/warehouse/eod-reports/r-1/pdf",
      { body: { locale: "en" } },
    );
    expect(req.platformRequest).toHaveBeenNthCalledWith(
      2,
      "GET",
      "/v1/warehouse/eod-report-pdfs/job-1",
    );
    expect(files.platformDownloadFile).toHaveBeenCalledWith(
      "/v1/warehouse/eod-report-pdfs/job-1/download",
    );
    expect(files.triggerBrowserDownload).toHaveBeenCalledWith(
      blob,
      "eod-2026-10-01.pdf",
    );
  });

  it("reports a failed job", async () => {
    req.platformRequest.mockResolvedValueOnce({
      uuid: "job-2",
      status: "failed",
      error: "boom",
    });
    await render(
      m,
      createElement(EodPdfButton, { reportUuid: "r-1", filename: "x.pdf" }),
    );
    await click($("eod-pdf"));
    await flush();
    expect(toast.error).toHaveBeenCalledWith("warehouse.eod.pdf_failed");
    expect(files.platformDownloadFile).not.toHaveBeenCalled();
  });
});
