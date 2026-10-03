import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
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
};

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
};
