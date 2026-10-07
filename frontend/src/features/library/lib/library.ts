import { normalizeLocale, SUPPORTED_LOCALES } from "@/config/i18n";
import type {
  LibraryAccessLevel,
  LibraryFolder,
  LibraryVersion,
} from "@/features/library/services/library.service";
import { isApiError } from "@/lib/api";

/** Backend limit (library usecase.MaxUploadBytes), enforced before upload. */
export const LIBRARY_MAX_UPLOAD_BYTES = 50 * 1024 * 1024;

/** Accepted by the backend content sniffer: PDF, images, zip, docx, xlsx. */
export const LIBRARY_UPLOAD_ACCEPT =
  ".pdf,.png,.jpg,.jpeg,.webp,.gif,.zip,.docx,.xlsx";

export const LIBRARY_ACCESS_LEVELS: readonly LibraryAccessLevel[] = [
  "all_network",
  "distributors",
  "dealers",
  "center_only",
];

export const LIBRARY_LOCALES = SUPPORTED_LOCALES;

/** `null` when the file may be uploaded, else the i18n key of the error. */
export function uploadFileError(file: File | null | undefined): string | null {
  if (!file) return "library.upload.file_required";
  if (file.size > LIBRARY_MAX_UPLOAD_BYTES) return "library.upload.too_large";
  return null;
}

/** UI locale → library locale (backend keys versions by `zh-CN`). */
export function libraryLocale(locale: string | null | undefined): string {
  return normalizeLocale(locale) ?? "tr";
}

/** Version history of one language, newest first. */
export function versionsForLocale(
  versions: LibraryVersion[],
  locale: string,
): LibraryVersion[] {
  return versions
    .filter((v) => v.locale === locale)
    .sort((a, b) => b.version_no - a.version_no);
}

/** Languages that have at least one version, in the supported order. */
export function availableLocales(versions: LibraryVersion[]): string[] {
  const present = new Set(versions.map((v) => v.locale));
  return LIBRARY_LOCALES.filter((l) => present.has(l));
}

/** "a, b ,a" → ["a", "b"] */
export function parseTags(raw: string): string[] {
  const out: string[] = [];
  for (const part of raw.split(",")) {
    const tag = part.trim();
    if (tag && !out.includes(tag)) out.push(tag);
  }
  return out;
}

export type FolderNode = LibraryFolder & {
  depth: number;
  children: FolderNode[];
};

/** Folder rows → tree (orphans become roots), sorted like the backend. */
export function buildFolderTree(folders: LibraryFolder[]): FolderNode[] {
  const byId = new Map<string, FolderNode>();
  for (const f of folders) byId.set(f.uuid, { ...f, depth: 0, children: [] });
  const roots: FolderNode[] = [];
  for (const node of byId.values()) {
    const parent = node.parent_uuid ? byId.get(node.parent_uuid) : undefined;
    if (parent) parent.children.push(node);
    else roots.push(node);
  }
  const order = (a: FolderNode, b: FolderNode) =>
    a.sort_order - b.sort_order || a.name.localeCompare(b.name);
  const walk = (nodes: FolderNode[], depth: number) => {
    nodes.sort(order);
    for (const n of nodes) {
      n.depth = depth;
      walk(n.children, depth + 1);
    }
  };
  walk(roots, 0);
  return roots;
}

/** Depth-first flat list (for the parent / folder pickers). */
export function flattenFolderTree(nodes: FolderNode[]): FolderNode[] {
  return nodes.flatMap((n) => [n, ...flattenFolderTree(n.children)]);
}

/** A folder and all of its descendants (a folder cannot move under them). */
export function folderSubtreeIds(
  nodes: FolderNode[],
  uuid: string,
): Set<string> {
  const out = new Set<string>();
  const visit = (list: FolderNode[], inside: boolean) => {
    for (const n of list) {
      const hit = inside || n.uuid === uuid;
      if (hit) out.add(n.uuid);
      visit(n.children, hit);
    }
  };
  visit(nodes, false);
  return out;
}

export function isFolderNotEmptyError(err: unknown): boolean {
  return (
    isApiError(err) &&
    (err.code === "LIBRARY_FOLDER_NOT_EMPTY" || err.status === 409)
  );
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB"];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[unit]}`;
}

export function localeShortLabel(locale: string): string {
  return locale === "zh-CN" ? "ZH" : locale.toUpperCase();
}
