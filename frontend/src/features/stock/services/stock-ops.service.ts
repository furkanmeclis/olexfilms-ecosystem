import type { ServerListQuery } from "@/components/entity";
import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

import type { Page } from "./stock.service";

type Schemas = components["schemas"];

export type StockUnitHistory = Schemas["StockUnitHistory"];
export type StockMovement = Schemas["StockMovement"];
export type StockMovementDocument = Schemas["StockMovementDocument"];
export type StockOwner = Schemas["StockOwner"];
export type StockReclassification = Schemas["StockReclassification"];
export type StockReclassificationStatus =
  Schemas["StockReclassificationStatus"];
export type StockSplit = Schemas["StockSplit"];

/**
 * GET /v1/stock/reclassifications params (TEC-763 list contract): `sort`
 * created_at / decided_at / status, `q`, CSV `status` / `requested_by`,
 * `from_product_uuid`, `to_product_uuid`, `created_from` / `created_to`.
 */
export type ReclassificationListQuery = ServerListQuery;

export type ReclassificationInput = {
  barcode: string;
  to_product_uuid: string;
  reason: string;
};

export type SplitInput = {
  barcode: string;
  meters: string;
  idempotency_key: string;
  reason?: string;
};

/** Movements per history page (the backend caps limit at 100). */
export const HISTORY_PAGE_SIZE = 100;

/** Label sheet of one unit (TEC-202). */
export function unitLabelPath(barcode: string): string {
  return `/v1/stock/labels/units.pdf?barcode=${encodeURIComponent(barcode)}`;
}

const enc = encodeURIComponent;

/**
 * Stock operations (TEC-763): the barcode history, reclassification
 * requests (TEC-157) and roll splits (TEC-184).
 */
export const stockOpsService = {
  unitHistory(barcode: string, offset = 0) {
    return platformRequest<StockUnitHistory>(
      "GET",
      `/v1/stock/units/by-barcode/${enc(barcode)}`,
      { query: { limit: HISTORY_PAGE_SIZE, offset } },
    );
  },
  listReclassifications(params: ReclassificationListQuery) {
    return platformRequest<Page<StockReclassification>>(
      "GET",
      "/v1/stock/reclassifications",
      { query: params },
    );
  },
  getReclassification(uuid: string) {
    return platformRequest<StockReclassification>(
      "GET",
      `/v1/stock/reclassifications/${enc(uuid)}`,
    );
  },
  requestReclassification(body: ReclassificationInput) {
    return platformRequest<StockReclassification>(
      "POST",
      "/v1/stock/reclassifications",
      { body },
    );
  },
  /** Approve (stock.reclassify + step-up), reject or cancel a request. */
  decideReclassification(
    uuid: string,
    action: "approve" | "reject" | "cancel",
    note?: string,
  ) {
    return platformRequest<StockReclassification>(
      "POST",
      `/v1/stock/reclassifications/${enc(uuid)}/${action}`,
      { body: note ? { note } : {} },
    );
  },
  split(body: SplitInput) {
    return platformRequest<StockSplit>("POST", "/v1/stock/splits", { body });
  },
};

export const stockOpsKeys = {
  unit: (barcode: string) => ["stock", "unit", barcode] as const,
  reclassifications: (params: ReclassificationListQuery) =>
    ["stock", "reclassifications", params] as const,
  reclassification: (uuid: string) =>
    ["stock", "reclassification", uuid] as const,
};
