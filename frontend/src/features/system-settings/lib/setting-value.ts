import type {
  SystemSetting,
  SystemSettingGroup,
  SystemSettingValue,
} from "@/features/system-settings/services/system-settings.service";

/** Server-side mask for secret values; writing it back keeps the stored one. */
export const SECRET_MASK = "********";

/** Page order of the catalog groups; unknown future groups go last. */
export const GROUP_ORDER: SystemSettingGroup[] = [
  "general",
  "contracts",
  "forecast",
  "services",
  "smtp",
  "warehouse",
  "scanning",
  "mobile",
];

export function groupSettings(items: SystemSetting[]) {
  const rank = (g: string) => {
    const i = GROUP_ORDER.indexOf(g as SystemSettingGroup);
    return i < 0 ? GROUP_ORDER.length : i;
  };
  const map = new Map<string, SystemSetting[]>();
  for (const item of items) {
    const list = map.get(item.group) ?? [];
    list.push(item);
    map.set(item.group, list);
  }
  return [...map.entries()]
    .sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b))
    .map(([group, settings]) => ({
      group,
      settings: [...settings].sort((a, b) => a.key.localeCompare(b.key)),
    }));
}

/** Form draft: ints and strings are edited as text, bools as booleans. */
export type Draft = string | boolean;

export function toDraft(setting: SystemSetting): Draft {
  if (setting.kind === "bool") return setting.value === true;
  // A secret starts empty: the mask is shown as a placeholder only.
  if (setting.secret) return "";
  return setting.value === undefined || setting.value === null
    ? ""
    : String(setting.value);
}

export type DraftError =
  | { code: "integer" }
  | { code: "min"; min: number }
  | { code: "max"; max: number }
  | { code: "max_len"; max: number };

/**
 * Mirrors the catalog checks of the backend (sysconfig.Definition.Validate)
 * so a bad value is flagged before the PUT; the server stays authoritative.
 */
export function parseDraft(
  setting: SystemSetting,
  draft: Draft,
): { value: SystemSettingValue } | { error: DraftError } {
  if (setting.kind === "bool") return { value: draft === true };
  const text = typeof draft === "string" ? draft : String(draft);
  if (setting.kind === "int") {
    const trimmed = text.trim();
    if (!/^-?\d+$/.test(trimmed)) return { error: { code: "integer" } };
    const n = Number(trimmed);
    if (!Number.isSafeInteger(n)) return { error: { code: "integer" } };
    if (setting.min !== undefined && n < setting.min) {
      return { error: { code: "min", min: setting.min } };
    }
    if (setting.max !== undefined && n > setting.max) {
      return { error: { code: "max", max: setting.max } };
    }
    return { value: n };
  }
  if (setting.secret && text === "") return { value: SECRET_MASK };
  if (setting.max_len && [...text].length > setting.max_len) {
    return { error: { code: "max_len", max: setting.max_len } };
  }
  return { value: text };
}
