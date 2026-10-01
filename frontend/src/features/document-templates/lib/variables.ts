import type { DocumentVariable } from "@/features/document-templates/services/document-templates.service";

const PLACEHOLDER_RE = /\{\{\s*\.?([a-zA-Z0-9_]+)\s*\}\}/g;

/** `{{key}}` placeholder text inserted by the variable menu. */
export function placeholder(key: string) {
  return `{{${key}}}`;
}

/** Distinct placeholder keys used in a template (lower-case, sorted). */
export function placeholdersIn(html: string): string[] {
  const keys = new Set<string>();
  for (const match of html.matchAll(PLACEHOLDER_RE)) {
    keys.add(match[1].toLowerCase());
  }
  return [...keys].sort();
}

/** Placeholders the kind does not define (the server rejects them). */
export function unknownPlaceholders(
  html: string,
  variables: readonly DocumentVariable[],
): string[] {
  const allowed = new Set(variables.map((v) => v.key));
  return placeholdersIn(html).filter((key) => !allowed.has(key));
}

/** Variables grouped for the menu, in schema order. */
export function groupVariables(
  variables: readonly DocumentVariable[],
): { group: string; items: DocumentVariable[] }[] {
  const groups = new Map<string, DocumentVariable[]>();
  for (const v of variables) {
    const list = groups.get(v.group) ?? [];
    list.push(v);
    groups.set(v.group, list);
  }
  return [...groups.entries()].map(([group, items]) => ({ group, items }));
}

/** Inserts text into a textarea value at the selection; returns new value and caret. */
export function insertAt(
  value: string,
  start: number,
  end: number,
  text: string,
): { value: string; caret: number } {
  const s = Math.max(0, Math.min(start, value.length));
  const e = Math.max(s, Math.min(end, value.length));
  return {
    value: value.slice(0, s) + text + value.slice(e),
    caret: s + text.length,
  };
}
