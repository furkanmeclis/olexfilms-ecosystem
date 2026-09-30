export type SpecPrefixEntry = {
  id: string;
  aliases: string[];
};

/** Normalizes prefix tokens for locale-aware matching (TR + EN). */
export function normalizePrefixToken(token: string): string {
  return token.trim().toLocaleLowerCase("tr");
}

export function buildSpecPrefixMap(
  entries: SpecPrefixEntry[],
): Map<string, string> {
  const map = new Map<string, string>();
  for (const { id, aliases } of entries) {
    for (const alias of aliases) {
      const key = normalizePrefixToken(alias);
      if (!key) continue;
      map.set(key, id);
    }
  }
  return map;
}

export function resolvePrefixToken(
  token: string,
  prefixMap: Map<string, string>,
): string | undefined {
  return prefixMap.get(normalizePrefixToken(token));
}
