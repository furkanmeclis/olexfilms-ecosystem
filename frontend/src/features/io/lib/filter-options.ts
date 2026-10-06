import activityCatalog from "@/locales/en/activity.json";
import exportsCatalog from "@/locales/en/exports.json";
import importsCatalog from "@/locales/en/imports.json";

/**
 * Filter option values for the activity / export / import lists. The
 * backends have no option endpoint; the values that have a label in the
 * (reference, English) catalog are the known slugs, labels come from `t()`.
 */
function catalogValues(catalog: Record<string, string>, prefix: string) {
  return Object.keys(catalog)
    .filter((key) => key.startsWith(prefix))
    .map((key) => key.slice(prefix.length))
    .sort();
}

export type FilterOption = { value: string; label: string; labelKey: string };

function options(
  catalog: Record<string, string>,
  prefix: string,
  namespace: string,
  keep?: (value: string) => boolean,
): FilterOption[] {
  return catalogValues(catalog, prefix)
    .filter((value) => (keep ? keep(value) : true))
    .map((value) => ({
      value,
      label: value,
      labelKey: `${namespace}.${prefix}${value}`,
    }));
}

export const ACTIVITY_RESOURCE_OPTIONS = options(
  activityCatalog,
  "resources.",
  "activity",
);
export const ACTIVITY_ACTION_OPTIONS = options(
  activityCatalog,
  "actions.",
  "activity",
);

/** Backend statuses (TEC-365); `pending` is a legacy label only. */
export const EXPORT_STATUS_VALUES = [
  "queued",
  "processing",
  "completed",
  "failed",
  "expired",
] as const;
export const EXPORT_FORMAT_VALUES = ["pdf", "xlsx", "csv", "json"] as const;
export const IMPORT_STATUS_VALUES = [
  "uploaded",
  "mapped",
  "previewed",
  "queued",
  "applying",
  "applied",
  "failed",
  "rolled_back",
] as const;
export const IMPORT_FORMAT_VALUES = ["json", "xlsx", "csv", "tsv"] as const;

/** Export/import job resources of one scope (`platform.*` / `tenant.*`). */
export function ioResourceOptions(
  kind: "exports" | "imports",
  scope: "platform" | "tenant",
): FilterOption[] {
  const catalog = kind === "exports" ? exportsCatalog : importsCatalog;
  return options(catalog, "resources.", kind, (value) =>
    value.startsWith(`${scope}.`),
  );
}
