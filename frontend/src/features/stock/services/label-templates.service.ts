import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type LabelTemplate = Schemas["LabelTemplate"];
export type LabelTemplateInput = Schemas["LabelTemplateInput"];
export type LabelTemplateKind = Schemas["LabelTemplateKind"];
export type LabelSymbology = Schemas["LabelSymbology"];
export type LabelLogoMode = Schemas["LabelLogoMode"];

const enc = encodeURIComponent;

/**
 * Label templates of the active organization (TEC-202 backend, TEC-764
 * screen): how unit and location labels print. Reads need stock.read,
 * writes stock.write; center and distributor only (K12). The list is the
 * full array (no paging).
 */
export const labelTemplatesService = {
  list() {
    return platformRequest<{ items: LabelTemplate[] }>(
      "GET",
      "/v1/stock/label-templates",
    );
  },
  create(body: LabelTemplateInput) {
    return platformRequest<LabelTemplate>("POST", "/v1/stock/label-templates", {
      body,
    });
  },
  update(uuid: string, body: LabelTemplateInput) {
    return platformRequest<LabelTemplate>(
      "PUT",
      `/v1/stock/label-templates/${enc(uuid)}`,
      { body },
    );
  },
  remove(uuid: string) {
    return platformRequest<void>(
      "DELETE",
      `/v1/stock/label-templates/${enc(uuid)}`,
    );
  },
};

export const labelTemplateKeys = {
  all: ["stock", "label-templates"] as const,
};
