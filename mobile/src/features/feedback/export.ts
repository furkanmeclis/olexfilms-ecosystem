import { Directory, File, Paths } from 'expo-file-system';
import { strToU8, type Zippable, zipSync } from 'fflate';

import { deviceSummary } from './context';
import type { FeedbackMeta } from './meta';
import { loadSharing } from './native';
import { recordFile } from './store';
import { buildZipPlan, zipFileName } from './zip-plan';

/** Thrown when this binary has no share sheet module (built before the feature). */
export class SharingUnavailableError extends Error {}

/**
 * Packs every record into one ZIP in the cache folder and opens the system
 * share sheet. Records stay on the device afterwards; the previous ZIP is
 * replaced by the next one.
 */
export async function shareFeedbackZip(records: FeedbackMeta[], dialogTitle: string) {
  const sharing = loadSharing();
  if (!sharing) throw new SharingUnavailableError('expo-sharing missing');

  const now = new Date();
  const plan = buildZipPlan(records, await deviceSummary(), now);
  const files: Zippable = {};
  for (const e of plan) {
    if (e.kind === 'json') {
      files[e.path] = strToU8(JSON.stringify(e.data, null, 2));
      continue;
    }
    const src = recordFile(e.recordId, e.file);
    // A record whose image went missing still exports its metadata.
    if (!src.exists) continue;
    // PNG is already compressed: store it as is.
    files[e.path] = [await src.bytes(), { level: 0 }];
  }
  const zipped = zipSync(files, { level: 6 });

  const dir = new Directory(Paths.cache, 'feedback-export');
  if (dir.exists) dir.delete();
  dir.create({ intermediates: true });
  const out = new File(dir, zipFileName(now));
  await out.write(zipped);

  await sharing.shareAsync(out.uri, { mimeType: 'application/zip', UTI: 'public.zip-archive', dialogTitle });
}
