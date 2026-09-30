const IO_RESOURCES = [
  "platform.users",
  "platform.roles",
  "platform.notifications",
  "platform.activity",
] as const;

const EXPORT_FORMATS = ["pdf", "xlsx", "csv", "json"] as const;

type TranslateFn = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Replaces raw IO resource/format keys in stored notification text (legacy rows). */
export function formatNotificationText(text: string, t: TranslateFn): string {
  if (!text) return text;

  let result = text;
  for (const resource of IO_RESOURCES) {
    const key = `exports.resources.${resource}`;
    const label = t(key);
    if (label !== key) {
      result = result.replaceAll(resource, label);
    }
  }

  for (const format of EXPORT_FORMATS) {
    const key = `exports.formats.${format}`;
    const label = t(key);
    if (label === key) continue;
    result = result.replace(new RegExp(`\\(${format}\\)`, "gi"), `(${label})`);
    result = result.replace(
      new RegExp(`\\(${format.toUpperCase()}\\)`, "g"),
      `(${label})`,
    );
  }

  return result;
}
