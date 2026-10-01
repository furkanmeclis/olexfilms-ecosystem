import { apiConfig } from "@/config/api";
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
  kind?: DocumentKind;
  language?: string;
  current?: boolean;
  limit?: number;
  offset?: number;
};

/** Template languages (K10: 12 languages + Arabic). */
export const DOCUMENT_LANGUAGES = [
  "tr",
  "en",
  "bg",
  "de",
  "el",
  "uk",
  "ru",
  "fr",
  "es",
  "it",
  "zh_CN",
  "az",
  "ar",
] as const;

export const DOCUMENT_KINDS: readonly DocumentKind[] = [
  "service",
  "measurement",
  "contract",
  "order_slip",
  "invoice_view",
  "warranty",
];

const base = "/v1/platform/document-templates";

export const documentTemplatesService = {
  list(params: ListDocumentTemplatesParams = {}) {
    return platformRequest<DocumentTemplateListResult>("GET", base, {
      query: {
        kind: params.kind,
        language: params.language,
        current: params.current === false ? "false" : undefined,
        limit: params.limit ?? 100,
        offset: params.offset,
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
