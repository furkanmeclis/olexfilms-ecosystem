import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type StockBulkJob = Schemas["StockBulkJob"];
export type StockBulkItem = Schemas["StockBulkItem"];
export type StockBulkAction = Schemas["StockBulkAction"];
export type StockBulkParams = Schemas["StockBulkParams"];
export type StockBulkSelection = Schemas["StockBulkSelection"];

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export const STOCK_BULK_ACTIONS: StockBulkAction[] = [
  "convert_to_roll",
  "change_product",
  "move_location",
  "void",
  "set_lot",
  "set_note",
];

/** Statuses of a job that is still working. */
export const ACTIVE_JOB_STATUSES = ["queued", "running", "undoing"];

const base = "/v1/platform/stock-bulk";
const enc = encodeURIComponent;

/**
 * TEC-1008 bulk barcode operations (super_admin): preview, run and undo
 * (the run and undo ask for a step-up through platformRequest).
 */
export const stockBulkService = {
  list(params: Record<string, string | number | undefined>) {
    return platformRequest<Page<StockBulkJob>>("GET", base, { query: params });
  },
  get(uuid: string) {
    return platformRequest<StockBulkJob>("GET", `${base}/${enc(uuid)}`);
  },
  items(uuid: string, params: Record<string, string | number | undefined>) {
    return platformRequest<Page<StockBulkItem>>(
      "GET",
      `${base}/${enc(uuid)}/items`,
      { query: params },
    );
  },
  preview(body: {
    action: StockBulkAction;
    params: StockBulkParams;
    selection: StockBulkSelection;
  }) {
    return platformRequest<StockBulkJob>("POST", base, { body });
  },
  run(uuid: string) {
    return platformRequest<StockBulkJob>("POST", `${base}/${enc(uuid)}/run`);
  },
  undo(uuid: string) {
    return platformRequest<StockBulkJob>("POST", `${base}/${enc(uuid)}/undo`);
  },
  discard(uuid: string) {
    return platformRequest<void>("DELETE", `${base}/${enc(uuid)}`);
  },
};

export const stockBulkKeys = {
  all: ["stock-bulk"] as const,
  list: (params: Record<string, unknown>) =>
    ["stock-bulk", "list", params] as const,
  job: (uuid: string) => ["stock-bulk", "job", uuid] as const,
  items: (uuid: string, params: Record<string, unknown>) =>
    ["stock-bulk", "items", uuid, params] as const,
};

/** Reads pasted / uploaded barcode lines (one per line, Excel columns kept). */
export function parseBarcodeLines(text: string): string[] {
  return text
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter((l) => l !== "" && !/^barkod|^barcode/i.test(l));
}
