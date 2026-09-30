/** API mapping: source column header → target field key. */
export type ApiColumnMapping = Record<string, string>;

/** UI mapping: target field key → source column header. */
export type UiColumnMapping = Record<string, string>;

export const MAPPING_SKIP = "__skip__";

export function normalizeHeader(value: string): string {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9\u00C0-\u024F]+/gi, "");
}

export function apiMappingToUi(
  api: ApiColumnMapping | undefined,
): UiColumnMapping {
  const ui: UiColumnMapping = {};
  if (!api) return ui;
  for (const [header, fieldKey] of Object.entries(api)) {
    if (header && fieldKey) ui[fieldKey] = header;
  }
  return ui;
}

export function uiMappingToApi(ui: UiColumnMapping): ApiColumnMapping {
  const api: ApiColumnMapping = {};
  for (const [fieldKey, header] of Object.entries(ui)) {
    if (!fieldKey || !header || header === MAPPING_SKIP) continue;
    api[header] = fieldKey;
  }
  return api;
}

type ImportFieldLike = { key: string; labelKey: string };

const ALIASES: Record<string, string[]> = {
  email: ["eposta", "e-posta", "mail", "e-mail", "emailaddress"],
  name: ["ad", "firstname", "first", "firstname", "givenname"],
  surname: ["soyad", "lastname", "last", "lastname", "familyname"],
  status: ["durum", "state"],
  locale: ["dil", "language", "lang"],
  slug: ["kod", "code", "identifier"],
  description: ["aciklama", "açıklama", "desc"],
  role_slugs: ["roles", "roller", "role", "rol"],
  permission_slugs: ["permissions", "izinler", "permission", "izin"],
};

function scoreMatch(
  headerNorm: string,
  headerRaw: string,
  fieldKey: string,
  label: string,
): number {
  const keyNorm = normalizeHeader(fieldKey);
  if (headerNorm === keyNorm) return 100;
  if (headerNorm.includes(keyNorm) || keyNorm.includes(headerNorm)) return 80;

  const labelNorm = normalizeHeader(label);
  if (labelNorm) {
    if (headerNorm === labelNorm) return 95;
    if (headerNorm.includes(labelNorm) || labelNorm.includes(headerNorm))
      return 85;
  }
  if (headerRaw.trim().toLowerCase() === label.trim().toLowerCase()) return 90;

  for (const alias of ALIASES[fieldKey] ?? []) {
    if (headerNorm === normalizeHeader(alias)) return 75;
  }
  return 0;
}

/** Fuzzy-match file headers to import fields (UI shape: field → header). */
export function suggestColumnMapping(
  headers: string[],
  fields: ImportFieldLike[],
  labelFor: (labelKey: string) => string,
): UiColumnMapping {
  const ui: UiColumnMapping = {};
  const usedFields = new Set<string>();
  const usedHeaders = new Set<string>();

  const ranked: { header: string; fieldKey: string; score: number }[] = [];
  for (const header of headers) {
    const headerNorm = normalizeHeader(header);
    if (!headerNorm) continue;
    for (const field of fields) {
      ranked.push({
        header,
        fieldKey: field.key,
        score: scoreMatch(
          headerNorm,
          header,
          field.key,
          labelFor(field.labelKey),
        ),
      });
    }
  }
  ranked.sort((a, b) => b.score - a.score);

  for (const match of ranked) {
    if (match.score < 60) continue;
    if (usedFields.has(match.fieldKey) || usedHeaders.has(match.header))
      continue;
    ui[match.fieldKey] = match.header;
    usedFields.add(match.fieldKey);
    usedHeaders.add(match.header);
  }

  return ui;
}

export function countMappedFields(mapping: UiColumnMapping): number {
  return Object.values(mapping).filter((v) => v && v !== MAPPING_SKIP).length;
}

export function applyAutoDefaults(
  resource: string,
  mapping: UiColumnMapping,
  defaults: UiColumnMapping,
): UiColumnMapping {
  const next = { ...defaults };
  if (resource === "platform.users") {
    if (!mapping.status && !next.status) next.status = "pending";
    if (!mapping.locale && !next.locale) next.locale = "tr";
  }
  if (resource === "platform.roles") {
    if (!mapping.permission_slugs && !next.permission_slugs)
      next.permission_slugs = "";
  }
  return next;
}
