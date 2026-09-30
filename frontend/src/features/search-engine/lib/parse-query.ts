import { normalizePrefixToken } from "@/features/search-engine/lib/spec-prefixes";

export type ParsedPaletteQuery = {
  spec?: string;
  text: string;
};

export function parsePaletteQuery(
  raw: string,
  prefixMap: Map<string, string>,
): ParsedPaletteQuery {
  const trimmed = raw.trim();
  if (!trimmed) return { text: "" };

  const colonIdx = trimmed.indexOf(":");
  if (colonIdx > 0) {
    const prefix = trimmed.slice(0, colonIdx);
    const spec = prefixMap.get(normalizePrefixToken(prefix));
    if (spec) {
      return {
        spec,
        text: trimmed.slice(colonIdx + 1).trim(),
      };
    }
  }

  const spaceIdx = trimmed.indexOf(" ");
  if (spaceIdx > 0) {
    const prefix = trimmed.slice(0, spaceIdx);
    const spec = prefixMap.get(normalizePrefixToken(prefix));
    if (spec) {
      return {
        spec,
        text: trimmed.slice(spaceIdx + 1).trim(),
      };
    }
  }

  return { text: trimmed };
}
