/**
 * What goes into a feedback ZIP, as a list of entries. Pure: the native side
 * reads the files and packs them (fflate); this only decides names and JSON.
 *
 * olexfilms-feedback-<date>.zip
 *   feedback.json                 every record + device summary
 *   001-<id>/original.png
 *   001-<id>/annotated.png        only when the record has a drawing
 *   001-<id>/meta.json
 */
import { type FeedbackMeta, sanitizeMeta } from './meta.ts';

export type ZipEntry =
  | { path: string; kind: 'json'; data: unknown }
  /** `file` is the name inside the record folder on the device (`original.png`). */
  | { path: string; kind: 'file'; recordId: string; file: string };

export type DeviceSummary = Record<string, unknown>;

/** `olexfilms-feedback-2026-11-01-1430.zip` in local time. */
export function zipFileName(d: Date) {
  const p = (n: number) => String(n).padStart(2, '0');
  return `olexfilms-feedback-${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}.zip`;
}

/** Folder of the n-th record (1-based) inside the ZIP. Ids are uuids, no path characters. */
export function recordFolder(index: number, id: string) {
  return `${String(index).padStart(3, '0')}-${id.replace(/[^\w-]/g, '_')}`;
}

/** Records oldest first, so folder numbers follow the order they were made. */
export function buildZipPlan(records: FeedbackMeta[], device: DeviceSummary, exportedAt: Date): ZipEntry[] {
  const sorted = [...records].sort((a, b) => a.created_at.localeCompare(b.created_at));
  const entries: ZipEntry[] = [];
  const index = sorted.map((r, i) => {
    const folder = recordFolder(i + 1, r.id);
    const meta = sanitizeMeta({
      ...r,
      files: {
        original: `${folder}/original.png`,
        annotated: r.files.annotated ? `${folder}/annotated.png` : null,
      },
    });
    entries.push({ path: `${folder}/original.png`, kind: 'file', recordId: r.id, file: 'original.png' });
    if (r.files.annotated) {
      entries.push({ path: `${folder}/annotated.png`, kind: 'file', recordId: r.id, file: 'annotated.png' });
    }
    entries.push({ path: `${folder}/meta.json`, kind: 'json', data: meta });
    return { folder, ...meta };
  });
  entries.unshift({
    path: 'feedback.json',
    kind: 'json',
    data: sanitizeMeta({ format: 1, exported_at: exportedAt.toISOString(), count: sorted.length, device, records: index }),
  });
  return entries;
}
