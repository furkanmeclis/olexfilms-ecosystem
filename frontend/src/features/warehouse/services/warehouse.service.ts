import { apiConfig } from "@/config/api";
import type { CertificateClient } from "@/features/warranty/lib/certificate";
import type { components } from "@/generated/api";
import { platformDownloadFile } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Warehouse = Schemas["Warehouse"];
export type WarehouseInput = Schemas["WarehouseInput"];
export type WarehousePatch = Schemas["WarehousePatch"];
export type WarehouseRoom = Schemas["WarehouseRoom"];
export type WarehouseRoomInput = Schemas["WarehouseRoomInput"];
export type WarehouseRoomPatch = Schemas["WarehouseRoomPatch"];
export type WarehouseLocation = Schemas["WarehouseLocation"];
export type WarehouseLocationType = Schemas["WarehouseLocationType"];
export type WarehouseLocationInput = Schemas["WarehouseLocationInput"];
export type WarehouseLocationPatch = Schemas["WarehouseLocationPatch"];
export type WarehouseScanResult = Schemas["WarehouseScanResult"];
export type StockEntry = Schemas["StockEntry"];
export type StockEntryLine = Schemas["StockEntryLine"];
export type StockEntryInput = Schemas["StockEntryInput"];
export type StockEntryLinesInput = Schemas["StockEntryLinesInput"];
export type StockEntryPlaceInput = Schemas["StockEntryPlaceInput"];
export type StockEntryStatus = Schemas["StockEntryStatus"];
export type StockEntryMode = Schemas["StockEntryMode"];
export type BarcodeBatch = Schemas["BarcodeBatch"];
export type BarcodeBatchInput = Schemas["BarcodeBatchInput"];
// --- TEC-232 (slice 2) ---
export type WarehouseTransfer = Schemas["WarehouseTransfer"];
export type WarehouseTransferLine = Schemas["WarehouseTransferLine"];
export type WarehouseTransferStatus = Schemas["WarehouseTransferStatus"];
export type WarehouseTransferInput = Schemas["WarehouseTransferInput"];
export type WarehouseTransferCompleteInput =
  Schemas["WarehouseTransferCompleteInput"];
export type WarehouseMoveInput = Schemas["WarehouseMoveInput"];
export type WarehouseMoveResult = Schemas["WarehouseMoveResult"];
export type StockCount = Schemas["StockCount"];
export type StockCountStatus = Schemas["StockCountStatus"];
export type StockCountInput = Schemas["StockCountInput"];
export type StockCountMethod = StockCountInput["method"];
export type StockCountVisibility = StockCountInput["visibility"];
export type StockCountScopeType = StockCountInput["scope_type"];
export type StockCountScan = Schemas["StockCountScan"];
export type StockCountScanInput = Schemas["StockCountScanInput"];
export type StockCountScanResult = Schemas["StockCountScanResult"];
export type StockCountLine = Schemas["StockCountLine"];
export type StockCountReport = Schemas["StockCountReport"];
export type StockCountApproveInput = Schemas["StockCountApproveInput"];
export type StockCountResolution =
  StockCountLine["allowed_resolutions"][number];
export type EodReport = Schemas["EodReport"];
export type EodReportGenerateInput = Schemas["EodReportGenerateInput"];
export type ExportJob = Schemas["ExportJob"];

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export type StockEntryListQuery = {
  status?: StockEntryStatus;
  limit: number;
  offset: number;
};

export type TransferListQuery = {
  status?: WarehouseTransferStatus;
  limit: number;
  offset: number;
};

export type CountListQuery = {
  status?: StockCountStatus;
  limit: number;
  offset: number;
};

export type EodListQuery = {
  scope?: "system" | "warehouse";
  warehouse_uuid?: string;
  date_from?: string;
  date_to?: string;
  limit: number;
  offset: number;
};

const enc = encodeURIComponent;

/**
 * BFF URL of a label PDF. The backend answers paths like
 * `/v1/stock/barcodes/{uuid}/labels.pdf` (labels_url, label_url); the
 * browser reaches them through the same-origin BFF base.
 */
export function bffUrl(path: string): string {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  return `${base}${path.startsWith("/") ? path : `/${path}`}`;
}

/** QR label sheet of a room's locations and/or the given locations. */
export function locationLabelsPath(opts: {
  room?: string;
  locations?: string[];
}): string {
  const params = new URLSearchParams();
  if (opts.room) params.set("room", opts.room);
  for (const uuid of opts.locations ?? []) params.append("location", uuid);
  return `/v1/warehouse/labels/locations.pdf?${params.toString()}`;
}

/**
 * Tenant warehouse (TEC-201..204, slice 1 of the frontend TEC-231): the
 * location tree, the universal scan, stock entries and the center's
 * barcode batches. Center and distributor only (K12); barcodes center only
 * (K14). Writes go through platformRequest (step-up retry).
 */
export const warehouseService = {
  // --- Tree (TEC-201) -----------------------------------------------------
  listWarehouses() {
    return platformRequest<{ items: Warehouse[] }>(
      "GET",
      "/v1/warehouse/warehouses",
    );
  },
  createWarehouse(body: WarehouseInput) {
    return platformRequest<Warehouse>("POST", "/v1/warehouse/warehouses", {
      body,
    });
  },
  updateWarehouse(uuid: string, body: WarehousePatch) {
    return platformRequest<Warehouse>(
      "PATCH",
      `/v1/warehouse/warehouses/${enc(uuid)}`,
      { body },
    );
  },
  deleteWarehouse(uuid: string) {
    return platformRequest<void>(
      "DELETE",
      `/v1/warehouse/warehouses/${enc(uuid)}`,
    );
  },
  listRooms(warehouseUuid: string) {
    return platformRequest<{ items: WarehouseRoom[] }>(
      "GET",
      `/v1/warehouse/warehouses/${enc(warehouseUuid)}/rooms`,
    );
  },
  createRoom(warehouseUuid: string, body: WarehouseRoomInput) {
    return platformRequest<WarehouseRoom>(
      "POST",
      `/v1/warehouse/warehouses/${enc(warehouseUuid)}/rooms`,
      { body },
    );
  },
  updateRoom(uuid: string, body: WarehouseRoomPatch) {
    return platformRequest<WarehouseRoom>(
      "PATCH",
      `/v1/warehouse/rooms/${enc(uuid)}`,
      { body },
    );
  },
  deleteRoom(uuid: string) {
    return platformRequest<void>("DELETE", `/v1/warehouse/rooms/${enc(uuid)}`);
  },
  listLocations(roomUuid: string) {
    return platformRequest<{ items: WarehouseLocation[] }>(
      "GET",
      `/v1/warehouse/rooms/${enc(roomUuid)}/locations`,
    );
  },
  createLocation(body: WarehouseLocationInput) {
    return platformRequest<WarehouseLocation>(
      "POST",
      "/v1/warehouse/locations",
      { body },
    );
  },
  updateLocation(uuid: string, body: WarehouseLocationPatch) {
    return platformRequest<WarehouseLocation>(
      "PATCH",
      `/v1/warehouse/locations/${enc(uuid)}`,
      { body },
    );
  },
  deleteLocation(uuid: string) {
    return platformRequest<void>(
      "DELETE",
      `/v1/warehouse/locations/${enc(uuid)}`,
    );
  },

  // --- Universal scan (TEC-203) ---------------------------------------------
  scan(code: string) {
    return platformRequest<WarehouseScanResult>("POST", "/v1/warehouse/scan", {
      body: { code },
    });
  },

  // --- Stock entries (TEC-204) ----------------------------------------------
  listEntries(params: StockEntryListQuery) {
    return platformRequest<Page<StockEntry>>(
      "GET",
      "/v1/warehouse/stock-entries",
      { query: params },
    );
  },
  createEntry(body: StockEntryInput) {
    return platformRequest<StockEntry>("POST", "/v1/warehouse/stock-entries", {
      body,
    });
  },
  getEntry(uuid: string) {
    return platformRequest<StockEntry>(
      "GET",
      `/v1/warehouse/stock-entries/${enc(uuid)}`,
    );
  },
  addLines(uuid: string, body: StockEntryLinesInput) {
    return platformRequest<StockEntry>(
      "POST",
      `/v1/warehouse/stock-entries/${enc(uuid)}/lines`,
      { body },
    );
  },
  deleteLine(uuid: string, lineUuid: string) {
    return platformRequest<StockEntry>(
      "DELETE",
      `/v1/warehouse/stock-entries/${enc(uuid)}/lines/${enc(lineUuid)}`,
    );
  },
  place(uuid: string, body: StockEntryPlaceInput) {
    return platformRequest<StockEntry>(
      "POST",
      `/v1/warehouse/stock-entries/${enc(uuid)}/place`,
      { body },
    );
  },
  confirm(uuid: string) {
    return platformRequest<StockEntry>(
      "POST",
      `/v1/warehouse/stock-entries/${enc(uuid)}/confirm`,
    );
  },
  cancel(uuid: string) {
    return platformRequest<StockEntry>(
      "POST",
      `/v1/warehouse/stock-entries/${enc(uuid)}/cancel`,
    );
  },

  // --- Barcode batches, center only (TEC-202, K14) ---------------------------
  listBatches(params: { limit: number; offset: number }) {
    return platformRequest<Page<BarcodeBatch>>("GET", "/v1/stock/barcodes", {
      query: params,
    });
  },
  createBatch(body: BarcodeBatchInput) {
    return platformRequest<BarcodeBatch>("POST", "/v1/stock/barcodes", {
      body,
    });
  },

  // --- Bin moves and warehouse transfers (TEC-205) ---------------------------
  move(body: WarehouseMoveInput) {
    return platformRequest<WarehouseMoveResult>("POST", "/v1/warehouse/moves", {
      body,
    });
  },
  listTransfers(params: TransferListQuery) {
    return platformRequest<Page<WarehouseTransfer>>(
      "GET",
      "/v1/warehouse/transfers",
      { query: params },
    );
  },
  createTransfer(body: WarehouseTransferInput) {
    return platformRequest<WarehouseTransfer>(
      "POST",
      "/v1/warehouse/transfers",
      { body },
    );
  },
  getTransfer(uuid: string) {
    return platformRequest<WarehouseTransfer>(
      "GET",
      `/v1/warehouse/transfers/${enc(uuid)}`,
    );
  },
  addTransferLines(uuid: string, barcodes: string[]) {
    return platformRequest<WarehouseTransfer>(
      "POST",
      `/v1/warehouse/transfers/${enc(uuid)}/lines`,
      { body: { barcodes } },
    );
  },
  deleteTransferLine(uuid: string, lineUuid: string) {
    return platformRequest<WarehouseTransfer>(
      "DELETE",
      `/v1/warehouse/transfers/${enc(uuid)}/lines/${enc(lineUuid)}`,
    );
  },
  placeTransfer(uuid: string, body: StockEntryPlaceInput) {
    return platformRequest<WarehouseTransfer>(
      "POST",
      `/v1/warehouse/transfers/${enc(uuid)}/place`,
      { body },
    );
  },
  shipTransfer(uuid: string) {
    return platformRequest<WarehouseTransfer>(
      "POST",
      `/v1/warehouse/transfers/${enc(uuid)}/ship`,
    );
  },
  completeTransfer(uuid: string, body: WarehouseTransferCompleteInput = {}) {
    return platformRequest<WarehouseTransfer>(
      "POST",
      `/v1/warehouse/transfers/${enc(uuid)}/complete`,
      { body },
    );
  },
  cancelTransfer(uuid: string) {
    return platformRequest<WarehouseTransfer>(
      "POST",
      `/v1/warehouse/transfers/${enc(uuid)}/cancel`,
    );
  },

  // --- Stock counts (TEC-206) ------------------------------------------------
  listCounts(params: CountListQuery) {
    return platformRequest<Page<StockCount>>(
      "GET",
      "/v1/warehouse/stock-counts",
      { query: params },
    );
  },
  createCount(body: StockCountInput) {
    return platformRequest<StockCount>("POST", "/v1/warehouse/stock-counts", {
      body,
    });
  },
  getCount(uuid: string) {
    return platformRequest<StockCount>(
      "GET",
      `/v1/warehouse/stock-counts/${enc(uuid)}`,
    );
  },
  countAction(
    uuid: string,
    action: "approve-start" | "start" | "complete" | "cancel",
  ) {
    return platformRequest<StockCount>(
      "POST",
      `/v1/warehouse/stock-counts/${enc(uuid)}/${action}`,
    );
  },
  listCountScans(uuid: string) {
    return platformRequest<{ items: StockCountScan[] }>(
      "GET",
      `/v1/warehouse/stock-counts/${enc(uuid)}/scans`,
    );
  },
  scanCount(uuid: string, body: StockCountScanInput) {
    return platformRequest<StockCountScanResult>(
      "POST",
      `/v1/warehouse/stock-counts/${enc(uuid)}/scans`,
      { body },
    );
  },
  deleteCountScan(uuid: string, scanUuid: string) {
    return platformRequest<void>(
      "DELETE",
      `/v1/warehouse/stock-counts/${enc(uuid)}/scans/${enc(scanUuid)}`,
    );
  },
  getCountReport(uuid: string) {
    return platformRequest<StockCountReport>(
      "GET",
      `/v1/warehouse/stock-counts/${enc(uuid)}/report`,
    );
  },
  approveCount(uuid: string, body: StockCountApproveInput) {
    return platformRequest<StockCountReport>(
      "POST",
      `/v1/warehouse/stock-counts/${enc(uuid)}/approve`,
      { body },
    );
  },

  // --- End-of-day reports (TEC-207) ------------------------------------------
  listEodReports(params: EodListQuery) {
    return platformRequest<Page<EodReport>>(
      "GET",
      "/v1/warehouse/eod-reports",
      { query: params },
    );
  },
  generateEodReport(body: EodReportGenerateInput) {
    return platformRequest<EodReport>("POST", "/v1/warehouse/eod-reports", {
      body,
    });
  },
  getEodReport(uuid: string) {
    return platformRequest<EodReport>(
      "GET",
      `/v1/warehouse/eod-reports/${enc(uuid)}`,
    );
  },
};

/** CSV export of a completed count (GET, through the BFF). */
export function countExportPath(uuid: string): string {
  return `/v1/warehouse/stock-counts/${enc(uuid)}/export`;
}

/**
 * End-of-day PDF (TEC-207): the same export-job flow as the service PDF,
 * so it plugs into `fetchCertificate` (request, poll, download).
 */
export function eodPdfClient(reportUuid: string): CertificateClient {
  return {
    request: (locale) =>
      platformRequest<ExportJob>(
        "POST",
        `/v1/warehouse/eod-reports/${enc(reportUuid)}/pdf`,
        { body: locale ? { locale } : {} },
      ),
    get: (jobUuid) =>
      platformRequest<ExportJob>(
        "GET",
        `/v1/warehouse/eod-report-pdfs/${enc(jobUuid)}`,
      ),
    download: (job) =>
      platformDownloadFile(
        `/v1/warehouse/eod-report-pdfs/${enc(job.uuid)}/download`,
      ),
  };
}

export const warehouseKeys = {
  all: ["warehouse"] as const,
  warehouses: ["warehouse", "warehouses"] as const,
  rooms: (warehouseUuid: string) =>
    ["warehouse", "rooms", warehouseUuid] as const,
  locations: (roomUuid: string) =>
    ["warehouse", "locations", roomUuid] as const,
  entries: (params: StockEntryListQuery) =>
    ["warehouse", "entries", params] as const,
  entry: (uuid: string) => ["warehouse", "entry", uuid] as const,
  batches: (params: { limit: number; offset: number }) =>
    ["warehouse", "batches", params] as const,
  transfers: (params: TransferListQuery) =>
    ["warehouse", "transfers", params] as const,
  transfer: (uuid: string) => ["warehouse", "transfer", uuid] as const,
  counts: (params: CountListQuery) => ["warehouse", "counts", params] as const,
  count: (uuid: string) => ["warehouse", "count", uuid] as const,
  countScans: (uuid: string) => ["warehouse", "count-scans", uuid] as const,
  countReport: (uuid: string) => ["warehouse", "count-report", uuid] as const,
  eodReports: (params: EodListQuery) =>
    ["warehouse", "eod-reports", params] as const,
  eodReport: (uuid: string) => ["warehouse", "eod-report", uuid] as const,
};
