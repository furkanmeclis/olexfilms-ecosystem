// Eşitleme kapısının tek örneği ve diskteki kaydı. Müşteri verisi burada tutulmaz.
import { readJSON, storage, writeJSON } from '@/core/storage';

import type { SyncRecord } from './sync-gate';
import { createSyncGate } from './sync-gate';

const RECORD_KEY = 'widgets.sync.v1';

function readRecord(): SyncRecord | null {
  const saved = readJSON<Partial<SyncRecord>>(RECORD_KEY);
  return saved && typeof saved.generatedAt === 'number' && typeof saved.owner === 'string'
    ? { generatedAt: saved.generatedAt, owner: saved.owner }
    : null;
}

export const syncGate = createSyncGate();
syncGate.restore(readRecord());

export function saveSyncRecord() {
  const { generatedAt, owner } = syncGate.snapshot();
  if (generatedAt !== null && owner !== null) writeJSON(RECORD_KEY, { generatedAt, owner } satisfies SyncRecord);
}

export function removeSyncRecord() {
  storage.remove(RECORD_KEY);
}
