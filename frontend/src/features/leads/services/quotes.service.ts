import type { components } from "@/generated/api";
import { platformDownloadFile } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Quote = Schemas["Quote"];
export type QuoteLine = Schemas["QuoteLine"];
export type QuoteLineInput = Schemas["QuoteLineInput"];
export type QuoteInput = Schemas["QuoteInput"];
export type QuoteStatus = Schemas["QuoteStatus"];
export type QuoteLineType = Schemas["QuoteLineType"];
export type QuoteSendResult = Schemas["QuoteSendResult"];
export type DocumentRender = Schemas["DocumentRender"];
export type LeadConvertInput = Schemas["LeadConvertInput"];
export type LeadConvertResult = Schemas["LeadConvertResult"];
export type LeadExistingUser = Schemas["LeadExistingUser"];

const enc = encodeURIComponent;

/**
 * Quote client (TEC-314/315/319): a lead's quotes, the draft editor
 * (lines are replaced as a whole, totals always come from the backend),
 * WhatsApp send / reminder, the accept / reject decision and the PDF
 * render (documents module, polled until ready).
 */
export const quotesService = {
  listByLead(leadUuid: string) {
    return platformRequest<{ items: Quote[]; total: number }>(
      "GET",
      `/v1/leads/${enc(leadUuid)}/quotes`,
    );
  },
  create(leadUuid: string, body: QuoteInput) {
    return platformRequest<Quote>("POST", `/v1/leads/${enc(leadUuid)}/quotes`, {
      body,
    });
  },
  get(uuid: string) {
    return platformRequest<Quote>("GET", `/v1/quotes/${enc(uuid)}`);
  },
  patch(uuid: string, body: { valid_until: string | null }) {
    return platformRequest<Quote>("PATCH", `/v1/quotes/${enc(uuid)}`, {
      body,
    });
  },
  replaceLines(uuid: string, lines: QuoteLineInput[]) {
    return platformRequest<Quote>("PUT", `/v1/quotes/${enc(uuid)}/lines`, {
      body: { lines },
    });
  },
  send(uuid: string) {
    return platformRequest<QuoteSendResult>(
      "POST",
      `/v1/quotes/${enc(uuid)}/send`,
    );
  },
  remind(uuid: string) {
    return platformRequest<Quote>("POST", `/v1/quotes/${enc(uuid)}/remind`);
  },
  accept(uuid: string, reason?: string) {
    return platformRequest<Quote>("POST", `/v1/quotes/${enc(uuid)}/accept`, {
      body: reason ? { reason } : {},
    });
  },
  reject(uuid: string, reason?: string) {
    return platformRequest<Quote>("POST", `/v1/quotes/${enc(uuid)}/reject`, {
      body: reason ? { reason } : {},
    });
  },
  requestPdf(uuid: string, locale?: string) {
    return platformRequest<DocumentRender>(
      "GET",
      `/v1/quotes/${enc(uuid)}/pdf`,
      { query: { locale } },
    );
  },
  getRender(renderUuid: string) {
    return platformRequest<DocumentRender>(
      "GET",
      `/v1/tenant/documents/${enc(renderUuid)}`,
    );
  },
  downloadRender(render: DocumentRender) {
    return platformDownloadFile(
      render.download_url ??
        `/v1/tenant/documents/${enc(render.uuid)}/download`,
    );
  },
  convert(leadUuid: string, body: LeadConvertInput) {
    return platformRequest<LeadConvertResult>(
      "POST",
      `/v1/leads/${enc(leadUuid)}/convert`,
      { body },
    );
  },
};

export const quoteKeys = {
  all: ["quotes"] as const,
  byLead: (leadUuid: string) => ["quotes", "lead", leadUuid] as const,
  detail: (uuid: string) => ["quotes", "detail", uuid] as const,
};
