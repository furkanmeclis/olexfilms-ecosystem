import type { components } from "@/generated/api";
import {
  platformDownloadFile,
  platformFormRequest,
} from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

/** TEC-1226: stock adjustment documents and the return inspection queue. */
export type StockAdjustment = Schemas["StockAdjustment"];
export type StockAdjustmentLine = Schemas["StockAdjustmentLine"];
export type StockAdjustmentInput = Schemas["StockAdjustmentInput"];
export type StockAdjustmentLineInput = Schemas["StockAdjustmentLineInput"];
export type StockAdjustmentSettings = Schemas["StockAdjustmentSettings"];
export type StockAdjustmentType = Schemas["StockAdjustmentType"];
export type StockAdjustmentStatus = Schemas["StockAdjustmentStatus"];
export type QuarantineItem = Schemas["QuarantineItem"];
export type QuarantineProduct = Schemas["QuarantineProduct"];
export type ReturnInspectionInput = Schemas["ReturnInspectionInput"];
export type ReturnInspectionResult = Schemas["ReturnInspectionResult"];
export type AdjustmentListQuery = Record<string, string | number | undefined>;

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

const enc = encodeURIComponent;
const base = "/v1/warehouse/adjustments";

export const adjustmentsService = {
  list(params: AdjustmentListQuery) {
    return platformRequest<Page<StockAdjustment>>("GET", base, {
      query: params,
    });
  },
  create(body: StockAdjustmentInput) {
    return platformRequest<StockAdjustment>("POST", base, { body });
  },
  get(uuid: string) {
    return platformRequest<StockAdjustment>("GET", `${base}/${enc(uuid)}`);
  },
  update(uuid: string, body: StockAdjustmentInput) {
    return platformRequest<StockAdjustment>("PATCH", `${base}/${enc(uuid)}`, {
      body,
    });
  },
  addLine(uuid: string, body: StockAdjustmentLineInput) {
    return platformRequest<StockAdjustment>(
      "POST",
      `${base}/${enc(uuid)}/lines`,
      { body },
    );
  },
  deleteLine(uuid: string, lineUuid: string) {
    return platformRequest<StockAdjustment>(
      "DELETE",
      `${base}/${enc(uuid)}/lines/${enc(lineUuid)}`,
    );
  },
  addPhoto(uuid: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return platformFormRequest<StockAdjustment>(
      "POST",
      `${base}/${enc(uuid)}/photos`,
      form,
    );
  },
  deletePhoto(uuid: string, photoUuid: string) {
    return platformRequest<StockAdjustment>(
      "DELETE",
      `${base}/${enc(uuid)}/photos/${enc(photoUuid)}`,
    );
  },
  async photoBlob(uuid: string, photoUuid: string) {
    const { blob } = await platformDownloadFile(
      `${base}/${enc(uuid)}/photos/${enc(photoUuid)}`,
    );
    return blob;
  },
  action(uuid: string, action: "submit" | "approve" | "cancel") {
    return platformRequest<StockAdjustment>(
      "POST",
      `${base}/${enc(uuid)}/${action}`,
    );
  },
  settings() {
    return platformRequest<StockAdjustmentSettings>(
      "GET",
      "/v1/warehouse/adjustment-settings",
    );
  },
  saveSettings(body: StockAdjustmentSettings) {
    return platformRequest<StockAdjustmentSettings>(
      "PUT",
      "/v1/warehouse/adjustment-settings",
      { body },
    );
  },
  quarantine(params: AdjustmentListQuery) {
    return platformRequest<Page<QuarantineItem>>(
      "GET",
      "/v1/warehouse/return-inspections",
      { query: params },
    );
  },
  quarantineSummary() {
    return platformRequest<{ items: QuarantineProduct[] }>(
      "GET",
      "/v1/warehouse/return-inspections/summary",
    );
  },
  inspect(body: ReturnInspectionInput) {
    return platformRequest<ReturnInspectionResult>(
      "POST",
      "/v1/warehouse/return-inspections",
      { body },
    );
  },
};

export const adjustmentKeys = {
  all: ["warehouse", "adjustments"] as const,
  list: (params: AdjustmentListQuery) =>
    ["warehouse", "adjustments", "list", params] as const,
  detail: (uuid: string) =>
    ["warehouse", "adjustments", "detail", uuid] as const,
  settings: ["warehouse", "adjustments", "settings"] as const,
  quarantine: (params: AdjustmentListQuery) =>
    ["warehouse", "quarantine", params] as const,
  quarantineSummary: ["warehouse", "quarantine", "summary"] as const,
};

/** Export job endpoint of the adjustment list (I/O engine, TEC-761). */
export const ADJUSTMENTS_EXPORT_PATH = "/v1/warehouse/adjustments/export";
