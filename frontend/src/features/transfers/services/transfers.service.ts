import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type StockTransfer = Schemas["StockTransfer"];
export type StockTransferStatus = Schemas["StockTransferStatus"];
/** sibling (K13 transfer) or return to the direct parent (TEC-223). */
export type StockTransferKind = Schemas["StockTransferKind"];
export type StockTransferItem = Schemas["StockTransferItem"];
export type StockTransferCreateInput = Schemas["StockTransferCreateInput"];
export type TransferOrgRef = Schemas["OrderOrgRef"];

/** List direction: the active organization's side of the request. */
export type TransferDirection = "outgoing" | "incoming" | "approval";

/** GET /v1/stock-transfers filters (TEC-197). */
export type TransferListQuery = {
  kind?: StockTransferKind;
  direction?: TransferDirection;
  status?: StockTransferStatus;
  limit: number;
  offset: number;
};

export type TransferPage = {
  items: StockTransfer[];
  total: number;
  limit: number;
  offset: number;
};

const enc = encodeURIComponent;

/**
 * Stock transfer requests between siblings (TEC-197, K13) through the BFF
 * with the active organization. Writes go through platformRequest, so a
 * STEP_UP_REQUIRED answer opens the step-up dialog and retries.
 */
export const transfersService = {
  list(params: TransferListQuery) {
    return platformRequest<TransferPage>("GET", "/v1/stock-transfers", {
      query: params,
    });
  },
  /** Siblings, or the direct parent for kind=return (TEC-223). */
  targets(kind?: StockTransferKind) {
    return platformRequest<{ items: TransferOrgRef[] }>(
      "GET",
      "/v1/stock-transfers/targets",
      kind ? { query: { kind } } : undefined,
    );
  },
  get(uuid: string) {
    return platformRequest<StockTransfer>(
      "GET",
      `/v1/stock-transfers/${enc(uuid)}`,
    );
  },
  create(body: StockTransferCreateInput) {
    return platformRequest<StockTransfer>("POST", "/v1/stock-transfers", {
      body,
    });
  },
  transition(uuid: string, status: StockTransferStatus, reason?: string) {
    return platformRequest<StockTransfer>(
      "POST",
      `/v1/stock-transfers/${enc(uuid)}/transitions`,
      { body: reason ? { status, reason } : { status } },
    );
  },
};

export const transferKeys = {
  all: ["stock-transfers"] as const,
  list: (params: TransferListQuery) =>
    ["stock-transfers", "list", params] as const,
  detail: (uuid: string) => ["stock-transfers", "detail", uuid] as const,
  targets: ["stock-transfers", "targets"] as const,
  targetsOf: (kind: StockTransferKind) =>
    ["stock-transfers", "targets", kind] as const,
};
