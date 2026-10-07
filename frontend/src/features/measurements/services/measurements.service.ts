import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { parseContentDispositionFilename } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

export type MeasurementSummary = components["schemas"]["MeasurementSummary"];
export type MeasurementDetail = components["schemas"]["MeasurementDetail"];
export type MeasurementValue = components["schemas"]["MeasurementValue"];
export type MeasurementTire = components["schemas"]["MeasurementTire"];
export type MeasurementDevice = components["schemas"]["MeasurementDevice"];
export type MeasurementPartMap = components["schemas"]["MeasurementPartMap"];

/**
 * GET /v1/measurements params: `q` matches VIN, plate or device serial;
 * `status` and `device_uuid` are CSV; `measured_from` / `measured_to`
 * dates; `linked` true|false; `sort` one of measured_at, created_at, vin,
 * status, plate (`-` desc).
 */
export type MeasurementListQuery = {
  q?: string;
  status?: string;
  device_uuid?: string;
  measured_from?: string;
  measured_to?: string;
  linked?: string;
  sort?: string;
  limit: number;
  offset: number;
};

export type MeasurementListPage = {
  items: MeasurementSummary[];
  total: number;
  limit: number;
  offset: number;
};

export type MeasurementDeviceInput = {
  serial?: string;
  label?: string | null;
  model?: string | null;
  is_active?: boolean;
};

const enc = encodeURIComponent;
const sleep = (ms: number) =>
  new Promise<void>((resolve) => setTimeout(resolve, ms));

/** Polls of the 202 render answer before giving up. */
export const PDF_MAX_POLLS = 15;

export const measurementsService = {
  list(params: MeasurementListQuery) {
    return platformRequest<MeasurementListPage>("GET", "/v1/measurements", {
      query: params,
    });
  },
  get(uuid: string) {
    return platformRequest<MeasurementDetail>(
      "GET",
      `/v1/measurements/${enc(uuid)}`,
    );
  },
  /** PATCH /v1/measurements/{uuid}/vin: vin_pending → accepted. */
  completeVin(uuid: string, vin: string) {
    return platformRequest<MeasurementDetail>(
      "PATCH",
      `/v1/measurements/${enc(uuid)}/vin`,
      { body: { vin } },
    );
  },
  /** Raw part map SVGs of a body type (colored in the browser). */
  partMap(bodyType: string) {
    return platformRequest<MeasurementPartMap>(
      "GET",
      `/v1/measurement-part-maps/${enc(bodyType)}`,
    );
  },
  /**
   * GET /v1/measurements/{uuid}/pdf: the first request queues the render
   * and answers 202 (`Retry-After`); the same URL is polled until the PDF
   * streams. API errors (422 VIN pending / not normalized) are thrown.
   */
  async downloadPdf(
    uuid: string,
    wait: (ms: number) => Promise<void> = sleep,
  ): Promise<{ blob: Blob; filename: string }> {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const url = `${base}/v1/measurements/${enc(uuid)}/pdf`;
    for (let i = 0; i < PDF_MAX_POLLS; i++) {
      const response = await fetch(url, { credentials: "include" });
      if (response.status === 202) {
        const retry = Number(response.headers.get("Retry-After") ?? "2");
        await wait((Number.isFinite(retry) && retry > 0 ? retry : 2) * 1000);
        continue;
      }
      if (!response.ok) {
        const payload = await response.json().catch(() => undefined);
        await unwrap({ data: payload, response }, { silent: true });
      }
      const blob = await response.blob();
      const filename =
        parseContentDispositionFilename(
          response.headers.get("Content-Disposition"),
        ) ?? `measurement-${uuid}.pdf`;
      return { blob, filename };
    }
    throw new Error("PDF_TIMEOUT");
  },

  listDevices() {
    return platformRequest<MeasurementDevice[]>(
      "GET",
      "/v1/measurement-devices",
    );
  },
  createDevice(input: MeasurementDeviceInput) {
    return platformRequest<MeasurementDevice>(
      "POST",
      "/v1/measurement-devices",
      { body: input },
    );
  },
  /** PATCH label / model / is_active (serial is immutable). */
  updateDevice(uuid: string, input: Omit<MeasurementDeviceInput, "serial">) {
    return platformRequest<MeasurementDevice>(
      "PATCH",
      `/v1/measurement-devices/${enc(uuid)}`,
      { body: input },
    );
  },
};

export const measurementKeys = {
  all: ["measurements"] as const,
  lists: ["measurements", "list"] as const,
  list: (params: MeasurementListQuery) =>
    ["measurements", "list", params] as const,
  detail: (uuid: string) => ["measurements", "detail", uuid] as const,
  partMap: (bodyType: string) =>
    ["measurements", "part-map", bodyType] as const,
  devices: ["measurements", "devices"] as const,
};
