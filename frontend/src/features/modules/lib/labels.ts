type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/**
 * Display name of a module key. Keys come from the backend catalog; a key
 * without a translation yet falls back to the key itself.
 */
export function moduleName(t: Translate, key: string): string {
  const i18nKey = `modules.names.${key}`;
  const text = t(i18nKey);
  return text === i18nKey ? key : text;
}

export function moduleSourceLabel(t: Translate, source: string): string {
  const i18nKey = `modules.source.${source}`;
  const text = t(i18nKey);
  return text === i18nKey ? source : text;
}

export function moduleLevelLabel(t: Translate, level: string): string {
  const i18nKey = `modules.level.${level}`;
  const text = t(i18nKey);
  return text === i18nKey ? level : text;
}
