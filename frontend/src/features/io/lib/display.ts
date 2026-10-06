import type { IoResource } from "@/features/io/types";

const RESOURCE_LABEL_KEYS: Record<IoResource, string> = {
  "platform.users": "users.title",
  "platform.roles": "roles.title",
  "platform.notifications": "notifications.title",
  "platform.activity": "activity.title",
  "tenant.catalog.products": "catalog.products.title",
  "platform.organizations": "organizations.title",
};

/**
 * Export jobs of endpoints outside `IoResource` (their own export path,
 * e.g. POST /v1/leads/export, TEC-372); label at `exports.resources.*`.
 */
const EXPORT_ONLY_RESOURCES = new Set<string>(["leads.list"]);

export function resourceLabelKey(resource: string): string | null {
  const key =
    RESOURCE_LABEL_KEYS[resource as IoResource] ??
    (EXPORT_ONLY_RESOURCES.has(resource) ? resource : undefined);
  return key ? `resources.${resource}` : null;
}

/** Activity log resource slug → activity.resources.* i18n path. */
export function activityResourceLabelKey(resource: string): string {
  return `resources.${resource}`;
}

export function formatActivityResource(
  t: (key: string) => string,
  resource: string,
): string {
  const key = `activity.${activityResourceLabelKey(resource)}`;
  const label = t(key);
  return label === key ? resource : label;
}

export function formatImportStatus(
  t: (key: string) => string,
  status: string,
): string {
  const key = `imports.status.${status}`;
  const translated = t(key);
  return translated === key ? status : translated;
}

export function formatExportStatus(
  t: (key: string) => string,
  status: string,
): string {
  const key = `exports.status.${status}`;
  const translated = t(key);
  return translated === key ? status : translated;
}

export function formatImportFormat(
  t: (key: string) => string,
  format: string,
): string {
  const key = `imports.formats.${format.toLowerCase()}`;
  const translated = t(key);
  return translated === key ? format.toUpperCase() : translated;
}

export function formatExportFormat(
  t: (key: string) => string,
  format: string,
): string {
  const key = `exports.formats.${format.toLowerCase()}`;
  const translated = t(key);
  return translated === key ? format.toUpperCase() : translated;
}

export function rollbackWindowOpen(
  rollbackUntil: string | null | undefined,
  nowMs: number,
): boolean {
  return Boolean(rollbackUntil && Date.parse(rollbackUntil) > nowMs);
}

/** Client-side fallback when Content-Disposition is missing. */
export function exportDownloadFilename(
  resource: string,
  format: string,
  at?: Date,
): string {
  const slug = resource.includes(".") ? resource.split(".").pop()! : resource;
  const ext =
    format.toLowerCase() === "pdf"
      ? "pdf"
      : format.toLowerCase() === "xlsx"
        ? "xlsx"
        : format.toLowerCase() === "json"
          ? "json"
          : "csv";
  const date = (at ?? new Date()).toISOString().slice(0, 10).replace(/-/g, "");
  return `${slug}_${date}.${ext}`;
}
