import { Directory, File, Paths } from 'expo-file-system';
import { create } from 'zustand';

import { storage } from '@/core/storage';

import type { FeedbackMeta } from './meta';
import { sanitizeMeta } from './meta';

/**
 * Shake-to-report records live on the device only: one folder per record
 * under `<documents>/feedback/<id>/` with `original.png`, `annotated.png`
 * (when drawn on) and `meta.json`. Nothing is sent to the server; the user
 * shares a ZIP from Settings. The on/off choice sits in the key-value store.
 */
const PREF_KEY = 'pref.feedback.shake';

function rootDir() {
  return new Directory(Paths.document, 'feedback');
}

function recordDir(id: string) {
  return new Directory(rootDir(), id);
}

/** File of a record by name (`original.png`, `annotated.png`, `meta.json`). */
export function recordFile(id: string, name: string) {
  return new File(recordDir(id), name);
}

function readRecords(): FeedbackMeta[] {
  const root = rootDir();
  if (!root.exists) return [];
  const out: FeedbackMeta[] = [];
  for (const entry of root.list()) {
    if (!(entry instanceof Directory)) continue;
    try {
      const meta = new File(entry, 'meta.json');
      if (!meta.exists) continue;
      out.push(JSON.parse(meta.textSync()) as FeedbackMeta);
    } catch {
      // A half-written or damaged record is skipped; deleting all removes it.
    }
  }
  return out.sort((a, b) => b.created_at.localeCompare(a.created_at));
}

type FeedbackStore = {
  enabled: boolean;
  /** Newest first; null until first read from disk. */
  records: FeedbackMeta[] | null;
  setEnabled: (on: boolean) => void;
  refresh: () => void;
  /** Moves the captured images into the record folder and writes `meta.json`. */
  save: (meta: FeedbackMeta, originalUri: string, annotatedUri: string | null) => void;
  remove: (id: string) => void;
  removeAll: () => void;
};

export const useFeedback = create<FeedbackStore>((set, get) => ({
  enabled: storage.getString(PREF_KEY) === '1',
  records: null,
  setEnabled: (on) => {
    storage.set(PREF_KEY, on ? '1' : '0');
    set({ enabled: on });
  },
  refresh: () => set({ records: readRecords() }),
  save: (meta, originalUri, annotatedUri) => {
    const dir = recordDir(meta.id);
    dir.create({ intermediates: true, idempotent: true });
    new File(originalUri).moveSync(new File(dir, 'original.png'));
    if (annotatedUri) new File(annotatedUri).moveSync(new File(dir, 'annotated.png'));
    const clean = sanitizeMeta({
      ...meta,
      files: { original: 'original.png', annotated: annotatedUri ? 'annotated.png' : null },
    });
    new File(dir, 'meta.json').writeSync(JSON.stringify(clean, null, 2));
    get().refresh();
  },
  remove: (id) => {
    const dir = recordDir(id);
    if (dir.exists) dir.delete();
    get().refresh();
  },
  removeAll: () => {
    const root = rootDir();
    if (root.exists) root.delete();
    set({ records: [] });
  },
}));
