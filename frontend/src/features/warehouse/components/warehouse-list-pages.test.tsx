// @vitest-environment jsdom
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  listWarehouses: vi.fn(),
  listEntries: vi.fn(),
  cancel: vi.fn(),
  listCounts: vi.fn(),
  countAction: vi.fn(),
  listTransfers: vi.fn(),
  shipTransfer: vi.fn(),
  cancelTransfer: vi.fn(),
  listEodReports: vi.fn(),
  listBatches: vi.fn(),
}));
const catalog = vi.hoisted(() => ({ listProducts: vi.fn() }));
const nav = vi.hoisted(() => ({ push: vi.fn() }));
const dialogs = vi.hoisted(() => ({ confirm: vi.fn() }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
const files = vi.hoisted(() => ({
  platformDownloadFile: vi.fn(),
  triggerBrowserDownload: vi.fn(),
}));
const eodPdf = vi.hoisted(() => ({ downloadEodPdf: vi.fn() }));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "center",
}));
const captured = vi.hoisted(() => ({
  tables: [] as Record<string, unknown>[],
}));

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
    t: (key: string) => key,
    format: {
      timeZone: "Europe/Istanbul",
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
  useActiveOrganization: () => ({
    uuid: "org-1",
    slug: "acme",
    type: state.orgType,
  }),
}));
vi.mock("@/providers/dialog-provider", () => ({ useDialogs: () => dialogs }));
vi.mock("@/providers/toast-provider", () => ({ appToast: toast }));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/lib/api/platform-form-request", () => files);
vi.mock("@/features/warehouse/components/eod-report-detail-page", () => eodPdf);
vi.mock("@/features/warehouse/services/warehouse.service", async (orig) => ({
  ...(await orig<object>()),
  warehouseService: api,
}));
vi.mock("@/features/catalog/services/catalog.service", async (orig) => ({
  ...(await orig<object>()),
  catalogService: catalog,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (props: Record<string, unknown>) => {
    captured.tables.push(props);
    return createElement("div", null, props.toolbarExtra as ReactNode);
  },
}));

import { Permission } from "@/config/permissions";

import type {
  BarcodeBatch,
  EodReport,
  StockCount,
  StockEntry,
  WarehouseTransfer,
} from "../services/warehouse.service";
import { BarcodesPage } from "./barcodes-page";
import { CountsPage } from "./counts-page";
import { EodReportsPage } from "./eod-reports-page";
import { StockEntriesPage } from "./stock-entries-page";
import { flush, mount, render, unmount, type Mounted } from "./test-helpers";
import { TransfersPage } from "./transfers-page";

type TableProps<T> = {
  columns: ColumnDef<T, unknown>[];
  rowCount?: number;
  features?: { persistKey?: string };
  state?: {
    onColumnFiltersChange?: (v: { id: string; value: unknown }[]) => void;
    onSortingChange?: (v: { id: string; desc: boolean }[]) => void;
    onGlobalFilterChange?: (v: string) => void;
  };
};
type Action = { id: string; onSelect: () => void | Promise<void> };

const PAGE = (total: number) => ({ items: [], total, limit: 20, offset: 0 });

let m: Mounted;
beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  state.grants = new Set([
    Permission.WarehouseRead,
    Permission.WarehouseWrite,
    Permission.StockRead,
  ]);
  state.orgType = "center";
  dialogs.confirm.mockResolvedValue(true);
  api.listWarehouses.mockResolvedValue({
    items: [
      { uuid: "wh-1", code: "WH1", name: "Main", active: true },
      { uuid: "wh-2", code: "WH2", name: "Second", active: true },
    ],
  });
  for (const fn of [
    api.listEntries,
    api.listCounts,
    api.listTransfers,
    api.listEodReports,
    api.listBatches,
  ]) {
    fn.mockResolvedValue(PAGE(7));
  }
  catalog.listProducts.mockResolvedValue({
    items: [{ uuid: "p-1", sku: "PPF-190", name: "Olex PPF 190" }],
    total: 1,
  });
  m = mount();
});

afterEach(() => {
  unmount(m);
  vi.clearAllMocks();
});

function lastTable<T>() {
  return captured.tables[
    captured.tables.length - 1
  ] as unknown as TableProps<T>;
}

function column<T>(id: string) {
  return lastTable<T>().columns.find(
    (c) => c.id === id || ("accessorKey" in c && c.accessorKey === id),
  );
}

function rowActions<T>(row: T): Action[] {
  const cell = column<T>("actions")?.cell as (
    ctx: unknown,
  ) => ReactElement<{ actions: Action[] }>;
  return cell({ row: { original: row } }).props.actions;
}

async function apply<T>(
  filters: { id: string; value: unknown }[],
  sort?: { id: string; desc: boolean },
  q?: string,
) {
  await act(async () => {
    lastTable<T>().state?.onColumnFiltersChange?.(filters);
  });
  await act(async () => {
    if (sort) lastTable<T>().state?.onSortingChange?.([sort]);
    if (q !== undefined) lastTable<T>().state?.onGlobalFilterChange?.(q);
  });
  await flush();
}

const lastCall = (fn: ReturnType<typeof vi.fn>) => fn.mock.calls.at(-1)?.[0];

describe("StockEntriesPage (TEC-376 DataTable)", () => {
  const entry = (over: Partial<StockEntry> = {}) =>
    ({
      uuid: "e-1",
      mode: "with_existing",
      status: "draft",
      note: null,
      warehouse: { uuid: "wh-1", code: "WH1", name: "Main" },
      import_batch_uuid: null,
      line_count: 2,
      created_at: "2026-10-01T09:00:00Z",
      confirmed_at: null,
      cancelled_at: null,
      ...over,
    }) as StockEntry;

  it("maps filters, search and sort to the list params", async () => {
    await render(m, createElement(StockEntriesPage, { slug: "acme" }));
    expect(lastCall(api.listEntries)).toEqual({
      limit: 20,
      offset: 0,
      sort: "-created_at",
    });
    expect(lastTable().rowCount).toBe(7);
    expect(lastTable().features?.persistKey).toBe(
      "tenant-warehouse-entries-v1",
    );
    expect(column("warehouse")?.meta?.filterOptions).toEqual([
      { value: "wh-1", label: "WH1 · Main" },
      { value: "wh-2", label: "WH2 · Second" },
    ]);

    await apply(
      [
        { id: "status", value: ["draft", "confirmed"] },
        { id: "mode", value: ["import"] },
        { id: "warehouse", value: ["wh-1", "wh-2"] },
        { id: "created_at", value: ["2026-10-01", "2026-10-05"] },
      ],
      { id: "warehouse", desc: true },
      "Container",
    );
    expect(lastCall(api.listEntries)).toEqual({
      limit: 20,
      offset: 0,
      sort: "-warehouse",
      q: "Container",
      status: "draft,confirmed",
      mode: "import",
      warehouse_uuid: "wh-1,wh-2",
      created_from: "2026-10-01",
      created_to: "2026-10-05",
    });
    expect(column("line_count")?.enableSorting).toBe(false);
    expect(column("mode")?.enableSorting).toBe(false);
  });

  it("cancels a draft from the row after the confirmation", async () => {
    api.cancel.mockResolvedValue(entry({ status: "cancelled" }));
    await render(m, createElement(StockEntriesPage, { slug: "acme" }));
    const actions = rowActions(entry());
    expect(actions.map((a) => a.id)).toEqual(["view", "cancel"]);
    await act(async () => actions[1].onSelect());
    await flush();
    expect(dialogs.confirm).toHaveBeenCalled();
    expect(api.cancel).toHaveBeenCalledWith("e-1");
    expect(toast.success).toHaveBeenCalledWith("warehouse.entry.cancelled");

    expect(rowActions(entry({ status: "confirmed" })).map((a) => a.id)).toEqual(
      ["view"],
    );
    state.grants.delete(Permission.WarehouseWrite);
    await render(m, createElement(StockEntriesPage, { slug: "acme" }));
    expect(rowActions(entry()).map((a) => a.id)).toEqual(["view"]);
  });
});

describe("CountsPage (TEC-376 DataTable)", () => {
  const count = (over: Partial<StockCount> = {}) =>
    ({
      uuid: "c-1",
      status: "draft",
      method: "location_first",
      visibility: "blind",
      scope_type: "warehouse",
      warehouse: { uuid: "wh-1", code: "WH1", name: "Main" },
      scope_room: null,
      scope_location: null,
      scope_product: null,
      note: null,
      start_approval_required: false,
      start_approved_at: null,
      created_at: "2026-10-01T09:00:00Z",
      ...over,
    }) as StockCount;

  it("maps every facet, the date range and the sort", async () => {
    await render(m, createElement(CountsPage, { slug: "acme" }));
    expect(lastCall(api.listCounts)).toEqual({
      limit: 20,
      offset: 0,
      sort: "-created_at",
    });
    await apply(
      [
        { id: "status", value: ["in_progress", "pending_review"] },
        { id: "method", value: ["unit_first"] },
        { id: "visibility", value: ["guided"] },
        { id: "scope_type", value: ["room", "location"] },
        { id: "warehouse", value: ["wh-2"] },
        { id: "created_at", value: ["2026-09-01", undefined] },
      ],
      { id: "status", desc: false },
      "yearly",
    );
    expect(lastCall(api.listCounts)).toEqual({
      limit: 20,
      offset: 0,
      sort: "status",
      q: "yearly",
      status: "in_progress,pending_review",
      method: "unit_first",
      visibility: "guided",
      scope_type: "room,location",
      warehouse_uuid: "wh-2",
      created_from: "2026-09-01",
    });
  });

  it("row actions: approve the start, start, cancel and the CSV", async () => {
    api.countAction.mockResolvedValue(count({ status: "in_progress" }));
    files.platformDownloadFile.mockResolvedValue({
      blob: new Blob(["x"]),
      filename: "count.csv",
    });
    await render(m, createElement(CountsPage, { slug: "acme" }));

    const approval = count({ start_approval_required: true });
    expect(rowActions(approval).map((a) => a.id)).toEqual([
      "view",
      "approve-start",
      "cancel",
    ]);
    const draft = rowActions(count());
    expect(draft.map((a) => a.id)).toEqual(["view", "start", "cancel"]);
    await act(async () => draft[1].onSelect());
    await flush();
    expect(api.countAction).toHaveBeenLastCalledWith("c-1", "start");

    await act(async () => rowActions(count())[2].onSelect());
    await flush();
    expect(dialogs.confirm).toHaveBeenCalled();
    expect(api.countAction).toHaveBeenLastCalledWith("c-1", "cancel");

    const review = rowActions(count({ status: "pending_review" }));
    expect(review.map((a) => a.id)).toEqual(["view", "export", "cancel"]);
    await act(async () => review[1].onSelect());
    await flush();
    expect(files.platformDownloadFile).toHaveBeenCalledWith(
      "/v1/warehouse/stock-counts/c-1/export",
    );
    expect(files.triggerBrowserDownload).toHaveBeenCalledWith(
      expect.any(Blob),
      "count.csv",
    );
  });
});

describe("TransfersPage (TEC-376 DataTable)", () => {
  const transfer = (over: Partial<WarehouseTransfer> = {}) =>
    ({
      uuid: "t-1",
      transfer_no: "WT-0001",
      status: "draft",
      note: null,
      from_warehouse: { uuid: "wh-1", code: "WH1", name: "Main" },
      to_warehouse: { uuid: "wh-2", code: "WH2", name: "Second" },
      to_location: null,
      line_count: 1,
      created_at: "2026-10-01T09:00:00Z",
      shipped_at: null,
      completed_at: null,
      cancelled_at: null,
      ...over,
    }) as WarehouseTransfer;

  it("maps source / target warehouse, status, created and the sort", async () => {
    await render(m, createElement(TransfersPage, { slug: "acme" }));
    expect(lastCall(api.listTransfers)).toEqual({
      limit: 20,
      offset: 0,
      sort: "-created_at",
    });
    await apply(
      [
        { id: "from_warehouse", value: ["wh-1"] },
        { id: "to_warehouse", value: ["wh-2"] },
        { id: "status", value: ["in_transit"] },
        { id: "created_at", value: [undefined, "2026-10-05"] },
      ],
      { id: "transfer_no", desc: true },
      "WT-00",
    );
    expect(lastCall(api.listTransfers)).toEqual({
      limit: 20,
      offset: 0,
      sort: "-transfer_no",
      q: "WT-00",
      from_warehouse_uuid: "wh-1",
      to_warehouse_uuid: "wh-2",
      status: "in_transit",
      created_to: "2026-10-05",
    });
  });

  it("ships a draft with lines and cancels from the row", async () => {
    api.shipTransfer.mockResolvedValue(transfer({ status: "in_transit" }));
    api.cancelTransfer.mockResolvedValue(transfer({ status: "cancelled" }));
    await render(m, createElement(TransfersPage, { slug: "acme" }));
    expect(rowActions(transfer({ line_count: 0 })).map((a) => a.id)).toEqual([
      "view",
      "cancel",
    ]);
    const actions = rowActions(transfer());
    expect(actions.map((a) => a.id)).toEqual(["view", "ship", "cancel"]);
    await act(async () => actions[1].onSelect());
    await flush();
    expect(api.shipTransfer).toHaveBeenCalledWith("t-1");
    await act(async () => rowActions(transfer())[2].onSelect());
    await flush();
    expect(api.cancelTransfer).toHaveBeenCalledWith("t-1");
    expect(
      rowActions(transfer({ status: "completed" })).map((a) => a.id),
    ).toEqual(["view"]);
  });
});

describe("EodReportsPage (TEC-376 DataTable)", () => {
  const report = {
    uuid: "r-1",
    report_date: "2026-10-01",
    kind: "auto",
    warehouse: null,
    summary: { totals: { movement_count: 3 } },
    generated_at: "2026-10-02T00:10:00Z",
  } as unknown as EodReport;

  it("maps day range, scope, warehouse, kind and the sort", async () => {
    state.grants.delete(Permission.WarehouseWrite);
    await render(m, createElement(EodReportsPage, { slug: "acme" }));
    expect(lastCall(api.listEodReports)).toEqual({
      limit: 20,
      offset: 0,
      sort: "-report_date",
    });
    expect(lastTable().features?.persistKey).toBe("tenant-warehouse-eod-v1");
    await apply(
      [
        { id: "report_date", value: ["2026-09-01", "2026-09-30"] },
        { id: "scope", value: "warehouse" },
        { id: "warehouse", value: "wh-1" },
        { id: "kind", value: ["manual"] },
      ],
      { id: "generated_at", desc: true },
    );
    expect(lastCall(api.listEodReports)).toEqual({
      limit: 20,
      offset: 0,
      sort: "-generated_at",
      date_from: "2026-09-01",
      date_to: "2026-09-30",
      scope: "warehouse",
      warehouse_uuid: "wh-1",
      kind: "manual",
    });
  });

  it("downloads the PDF from the row", async () => {
    state.grants.delete(Permission.WarehouseWrite);
    eodPdf.downloadEodPdf.mockResolvedValue(undefined);
    await render(m, createElement(EodReportsPage, { slug: "acme" }));
    const actions = rowActions(report);
    expect(actions.map((a) => a.id)).toEqual(["view", "pdf"]);
    await act(async () => actions[1].onSelect());
    await flush();
    expect(eodPdf.downloadEodPdf).toHaveBeenCalledWith(
      "r-1",
      "en",
      "eod-2026-10-01.pdf",
    );
    expect(toast.success).toHaveBeenCalledWith("warehouse.eod.pdf_ready");
  });
});

describe("BarcodesPage (TEC-376 DataTable)", () => {
  it("maps product, printed, created and the sort; prints from the row", async () => {
    files.platformDownloadFile.mockResolvedValue({
      blob: new Blob(["%PDF"]),
      filename: null,
    });
    await render(m, createElement(BarcodesPage, { slug: "acme" }));
    expect(lastCall(api.listBatches)).toEqual({
      limit: 20,
      offset: 0,
      sort: "-created_at",
    });
    expect(column("product")?.meta?.filterOptions).toEqual([
      { value: "p-1", label: "PPF-190 · Olex PPF 190" },
    ]);
    await apply(
      [
        { id: "product", value: ["p-1"] },
        { id: "print_count", value: false },
        { id: "created_at", value: ["2026-10-01", "2026-10-02"] },
      ],
      { id: "print_count", desc: true },
      "OLEX-0001",
    );
    expect(lastCall(api.listBatches)).toEqual({
      limit: 20,
      offset: 0,
      sort: "-print_count",
      q: "OLEX-0001",
      product_uuid: "p-1",
      printed: "false",
      created_from: "2026-10-01",
      created_to: "2026-10-02",
    });

    const batch = {
      uuid: "b-1",
      first_barcode: "OLEX-00000001",
      labels_url: "/v1/stock/barcodes/b-1/labels.pdf",
    } as BarcodeBatch;
    const actions = rowActions(batch);
    expect(actions.map((a) => a.id)).toEqual(["print"]);
    await act(async () => actions[0].onSelect());
    await flush();
    expect(files.platformDownloadFile).toHaveBeenCalledWith(
      "/v1/stock/barcodes/b-1/labels.pdf",
    );
    expect(files.triggerBrowserDownload).toHaveBeenCalledWith(
      expect.any(Blob),
      "OLEX-00000001.pdf",
    );
  });

  it("stays closed to a distributor", async () => {
    state.orgType = "distributor";
    await render(m, createElement(BarcodesPage, { slug: "acme" }));
    expect(api.listBatches).not.toHaveBeenCalled();
    expect(m.container.textContent).toContain("warehouse.barcodes.forbidden");
  });
});
