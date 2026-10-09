import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import {
  parseContentDispositionFilename,
  platformFormRequest,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Einvoice = Schemas["Einvoice"];
export type EinvoiceStatus = Einvoice["status"];
export type EinvoiceProfile = Einvoice["profile"];
export type EinvoiceValidationStatus = Einvoice["validation_status"];
export type EinvoiceBillable = Schemas["EinvoiceBillable"];
export type EinvoiceSourceType = EinvoiceBillable["source_type"];
export type EinvoiceSettings = Schemas["EinvoiceSettings"];
export type EinvoiceSettingsRequest = Schemas["EinvoiceSettingsRequest"];
export type EinvoiceCounter = Schemas["EinvoiceCounter"];
export type InvoiceProfile = Schemas["InvoiceProfile"];
export type InvoiceProfileRequest = Schemas["InvoiceProfileRequest"];

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** List query: paging, `sort`, `q` and the endpoint's filters. */
export type ListQuery = {
  limit?: number;
  offset?: number;
  sort?: string;
  q?: string;
} & Record<string, string | number | boolean | undefined>;

function toQuery(params: ListQuery) {
  const query: Record<string, string | number | undefined> = {};
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === "") continue;
    query[key] = typeof value === "boolean" ? String(value) : value;
  }
  return query;
}

const enc = encodeURIComponent;
const BASE = "/v1/einvoices";

export const einvoiceKeys = {
  all: ["einvoices"] as const,
  list: (params: ListQuery) => ["einvoices", "list", params] as const,
  billable: (params: ListQuery) => ["einvoices", "billable", params] as const,
  detail: (uuid: string) => ["einvoices", "detail", uuid] as const,
  html: (uuid: string, kind: string) =>
    ["einvoices", "html", uuid, kind] as const,
  settings: ["einvoices", "settings"] as const,
  samplePreview: (sha: string) => ["einvoices", "sample", sha] as const,
  profile: (orgUuid: string) => ["einvoices", "profile", orgUuid] as const,
};

/**
 * HTML / XML / PDF through the BFF. Unlike platformDownloadFile a failed
 * response throws the ApiError (EINVOICE_PDF_NOT_READY,
 * EINVOICE_SETTINGS_REQUIRED, ...) so the screen can name the reason.
 */
async function fetchFile(path: string, silent = false) {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  const response = await fetch(`${base}${path}`, { credentials: "include" });
  if (!response.ok) {
    const payload = await response.json().catch(() => undefined);
    await unwrap({ data: payload, response }, { silent });
    throw new Error(`Download failed (${response.status})`);
  }
  return {
    blob: await response.blob(),
    filename: parseContentDispositionFilename(
      response.headers.get("Content-Disposition"),
    ),
  };
}

/** Inline previews show their error in place (no global toast). */
async function html(path: string) {
  const { blob } = await fetchFile(path, true);
  return blob.text();
}

/**
 * E-invoices of the brand center (TEC-503 API, TEC-504 screens). Archive
 * and void need a recent step-up; callers ask for it first and
 * platformRequest retries through the step-up engine as well.
 */
export const einvoiceService = {
  list(params: ListQuery) {
    return platformRequest<Page<Einvoice>>("GET", BASE, {
      query: toQuery(params),
    });
  },
  billable(params: ListQuery) {
    return platformRequest<Page<EinvoiceBillable>>("GET", `${BASE}/billable`, {
      query: toQuery(params),
    });
  },
  get(uuid: string) {
    return platformRequest<Einvoice>("GET", `${BASE}/${enc(uuid)}`);
  },
  createDraft(sourceType: EinvoiceSourceType, sourceUuid: string) {
    return platformRequest<Einvoice>("POST", BASE, {
      body: { source_type: sourceType, source_uuid: sourceUuid },
    });
  },
  archive(uuid: string) {
    return platformRequest<Einvoice>("POST", `${BASE}/${enc(uuid)}/archive`);
  },
  void(uuid: string, reason: string) {
    return platformRequest<Einvoice>("POST", `${BASE}/${enc(uuid)}/void`, {
      body: { reason },
    });
  },
  retryPdf(uuid: string) {
    return platformRequest<Einvoice>("POST", `${BASE}/${enc(uuid)}/pdf/retry`);
  },
  /** Draft / failed: unnumbered preview; archived / voided: stored XML. */
  html(invoice: Pick<Einvoice, "uuid" | "status">) {
    const kind =
      invoice.status === "draft" || invoice.status === "failed"
        ? "preview"
        : "html";
    return html(`${BASE}/${enc(invoice.uuid)}/${kind}`);
  },
  async download(
    invoice: Pick<Einvoice, "uuid" | "number">,
    kind: "xml" | "pdf",
  ) {
    const { blob, filename } = await fetchFile(
      `${BASE}/${enc(invoice.uuid)}/${kind}`,
    );
    triggerBrowserDownload(
      blob,
      filename ?? `${invoice.number ?? invoice.uuid}.${kind}`,
    );
  },
  getSettings() {
    return platformRequest<EinvoiceSettings>("GET", `${BASE}/settings`);
  },
  putSettings(body: EinvoiceSettingsRequest) {
    return platformRequest<EinvoiceSettings>("PUT", `${BASE}/settings`, {
      body,
    });
  },
  uploadXslt(file: File) {
    const form = new FormData();
    form.append("file", file);
    return platformFormRequest<EinvoiceSettings>(
      "POST",
      `${BASE}/settings/xslt`,
      form,
    );
  },
  resetXslt() {
    return platformRequest<EinvoiceSettings>("DELETE", `${BASE}/settings/xslt`);
  },
  samplePreview() {
    return html(`${BASE}/settings/xslt/preview`);
  },
  getProfile(orgUuid: string) {
    return platformRequest<InvoiceProfile>(
      "GET",
      `/v1/platform/organizations/${enc(orgUuid)}/invoice-profile`,
    );
  },
  putProfile(orgUuid: string, body: InvoiceProfileRequest) {
    return platformRequest<InvoiceProfile>(
      "PUT",
      `/v1/platform/organizations/${enc(orgUuid)}/invoice-profile`,
      { body },
    );
  },
};
