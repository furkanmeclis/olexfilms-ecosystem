import { apiConfig } from "@/config/api";
import { SUPPORTED_LOCALES, normalizeLocale } from "@/config/i18n";
import type { components } from "@/generated/api";
import { ApiError } from "@/lib/api/errors";
import { platformRequest } from "@/lib/api/platform-request";

export type DocumentKind = components["schemas"]["DocumentKind"];
export type DocumentVariable = components["schemas"]["DocumentVariable"];
export type DocumentKindInfo = components["schemas"]["DocumentKindInfo"];
export type DocumentTemplate = components["schemas"]["DocumentTemplate"];
export type DocumentTemplateSaveRequest =
  components["schemas"]["DocumentTemplateSaveRequest"];
export type DocumentPreviewRequest =
  components["schemas"]["DocumentPreviewRequest"];

export type DocumentTemplateListResult = {
  items: DocumentTemplate[];
  total: number;
  limit: number;
  offset: number;
};

export type ListDocumentTemplatesParams = {
  /** One kind or a CSV of kinds */
  kind?: DocumentKind | string;
  /** One language or a CSV (zh-CN accepted) */
  language?: string;
  /** CSV of draft, active, superseded */
  status?: string;
  /** Brand slug */
  brand?: string;
  platform_default?: boolean;
  current?: boolean;
  limit?: number;
  offset?: number;
  sort?: string;
  q?: string;
};

/**
 * Template languages (K10: 12 languages + Arabic), the app's locale codes.
 * The API stores Chinese as "zh_CN"; it accepts "zh-CN" and rows are read
 * through documentLanguage().
 */
export const DOCUMENT_LANGUAGES = SUPPORTED_LOCALES;

/** Canonical code of a template language from the API ("zh_CN" -> zh-CN). */
export function documentLanguage(code: string): string {
  return normalizeLocale(code) ?? code;
}

export const DOCUMENT_KINDS: readonly DocumentKind[] = [
  "service",
  "measurement",
  "contract",
  "order_slip",
  "invoice_view",
  "warranty",
  "fleet_report",
  "price_list",
];

const base = "/v1/platform/document-templates";

export const documentTemplatesService = {
  list(params: ListDocumentTemplatesParams = {}) {
    return platformRequest<DocumentTemplateListResult>("GET", base, {
      query: {
        kind: params.kind,
        language: params.language,
        status: params.status,
        brand: params.brand,
        platform_default:
          params.platform_default === undefined
            ? undefined
            : String(params.platform_default),
        current: params.current === false ? "false" : undefined,
        limit: params.limit ?? 100,
        offset: params.offset,
        sort: params.sort,
        q: params.q,
      },
    });
  },
  kinds() {
    return platformRequest<DocumentKindInfo[]>("GET", `${base}/kinds`);
  },
  get(uuid: string) {
    return platformRequest<DocumentTemplate>("GET", `${base}/${uuid}`);
  },
  versions(uuid: string) {
    return platformRequest<DocumentTemplate[]>(
      "GET",
      `${base}/${uuid}/versions`,
    );
  },
  saveDraft(body: DocumentTemplateSaveRequest) {
    return platformRequest<DocumentTemplate>("POST", base, { body });
  },
  publish(uuid: string) {
    return platformRequest<DocumentTemplate>("POST", `${base}/${uuid}/publish`);
  },
  /** Renders a PDF with sample data; returns the PDF blob. */
  async preview(body: DocumentPreviewRequest): Promise<Blob> {
    const url = `${apiConfig.baseUrl.replace(/\/$/, "")}${base}/preview`;
    const response = await fetch(url, {
      method: "POST",
      credentials: "include",
      headers: {
        Accept: "application/pdf, application/json",
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      const payload = (await response.json().catch(() => undefined)) as
        { error?: { code?: string; message?: string } } | undefined;
      throw new ApiError({
        status: response.status,
        code: payload?.error?.code ?? "PREVIEW_FAILED",
        message:
          payload?.error?.message ?? `Preview failed (${response.status})`,
      });
    }
    return response.blob();
  },
};
