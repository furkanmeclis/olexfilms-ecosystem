import { SUPPORTED_LOCALES, normalizeLocale } from "@/config/i18n";
import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type ContractTemplateKind = Schemas["ContractTemplateKind"];
export type ContractTemplate = Schemas["ContractTemplate"];
export type ContractTemplateLocale = Schemas["ContractTemplateLocale"];
export type ContractTemplateVariable = Schemas["ContractTemplateVariable"];
export type ContractTemplateRequest = Schemas["ContractTemplateRequest"];
export type ContractTemplateLocaleRequest =
  Schemas["ContractTemplateLocaleRequest"];

export const CONTRACT_TEMPLATE_KINDS: readonly ContractTemplateKind[] = [
  "vehicle_intake",
  "service_sale",
];

/** Template languages: the 13 app locales (the API stores zh-CN). */
export const CONTRACT_TEMPLATE_LOCALES = SUPPORTED_LOCALES;

/** Canonical app locale of an API locale code ("zh_CN" -> "zh-CN"). */
export function templateLocale(code: string): string {
  return normalizeLocale(code) ?? code;
}

/** Locale version of a template for one app locale, if written. */
export function findLocale(
  template: Pick<ContractTemplate, "locales"> | undefined,
  locale: string,
): ContractTemplateLocale | undefined {
  return template?.locales?.find((l) => templateLocale(l.locale) === locale);
}

const base = "/v1/platform/contract-templates";

export const contractTemplatesService = {
  list(params: { kind?: ContractTemplateKind } = {}) {
    return platformRequest<{ items: ContractTemplate[] }>("GET", base, {
      query: { kind: params.kind },
    });
  },
  get(uuid: string) {
    return platformRequest<ContractTemplate>("GET", `${base}/${uuid}`);
  },
  create(body: ContractTemplateRequest) {
    return platformRequest<ContractTemplate>("POST", base, { body });
  },
  update(uuid: string, body: ContractTemplateRequest) {
    return platformRequest<ContractTemplate>("PATCH", `${base}/${uuid}`, {
      body,
    });
  },
  putLocale(uuid: string, locale: string, body: ContractTemplateLocaleRequest) {
    return platformRequest<ContractTemplateLocale>(
      "PUT",
      `${base}/${uuid}/locales/${encodeURIComponent(locale)}`,
      { body },
    );
  },
  setDefault(uuid: string) {
    return platformRequest<ContractTemplate>("POST", `${base}/${uuid}/default`);
  },
  variables() {
    return platformRequest<{ items: ContractTemplateVariable[] }>(
      "GET",
      "/v1/contract-templates/variables",
    );
  },
};
