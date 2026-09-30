import type { PaletteItem } from "@/features/search-engine/types";

const STORAGE_KEY = "app.search.recent.v1";
const MAX_RECENT = 8;

type StoredRecentItem = Pick<
  PaletteItem,
  "id" | "spec" | "label" | "description" | "href" | "iconKey" | "group"
>;

function toStored(item: PaletteItem): StoredRecentItem {
  const { id, spec, label, description, href, iconKey, group } = item;
  return { id, spec, label, description, href, iconKey, group };
}

export function readRecentItems(): PaletteItem[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as StoredRecentItem[];
    return Array.isArray(parsed) ? parsed.slice(0, MAX_RECENT) : [];
  } catch {
    return [];
  }
}

export function pushRecentItem(item: PaletteItem) {
  if (typeof window === "undefined") return;
  const stored = toStored(item);
  const next = [
    stored,
    ...readRecentItems()
      .map(toStored)
      .filter((entry) => entry.id !== stored.id),
  ].slice(0, MAX_RECENT);
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  } catch {
    // Ignore storage quota/security errors
  }
}
