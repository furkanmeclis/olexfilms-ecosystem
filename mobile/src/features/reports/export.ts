import { Directory, File, Paths } from 'expo-file-system';
import { Platform, Share } from 'react-native';

import { api } from '@/core/api/client';
import { ApiError, toApiError } from '@/core/api/errors';
import type { PeriodQuery } from '@/features/analytics-kit/period';
import { loadSharing } from '@/features/feedback/native';
import { safeFilename } from '@/features/warehouse/pdf-share';

export type ExportFormat = 'xlsx' | 'csv';

const MIME: Record<ExportFormat, { mime: string; uti: string }> = {
  xlsx: { mime: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', uti: 'org.openxmlformats.spreadsheetml.sheet' },
  csv: { mime: 'text/csv', uti: 'public.comma-separated-values-text' },
};

/** Own cache folder for report exports; wiped before every download (they can hold customer data). */
function exportDir() {
  const dir = new Directory(Paths.cache, 'report-exports');
  if (dir.exists) dir.delete();
  dir.create({ intermediates: true });
  return dir;
}

/** Whether this binary can hand a file to the share sheet (expo-sharing, or iOS Share with a file URL). */
export function canShareExport() {
  return Platform.OS === 'ios' || !!loadSharing();
}

/**
 * Downloads a report as XLSX or CSV from the backend export endpoint
 * (GET /v1/reports/{report}/export, same query as the report, TEC-1210) and
 * opens the system share sheet (Files, Mail, WhatsApp, AirDrop). No new
 * native module: expo-file-system and expo-sharing ship with the app.
 */
export async function shareReportExport(key: string, query: PeriodQuery & { limit?: number; all_time?: boolean }, format: ExportFormat, title: string) {
  // The export path joins the typed client after TEC-1210; until then it is untyped.
  const get = api.GET as unknown as (
    path: string,
    init: { params: { path: { report: string }; query: Record<string, unknown> }; parseAs: 'arrayBuffer' },
  ) => Promise<{ data?: ArrayBuffer; error?: unknown; response: Response }>;
  let r: Awaited<ReturnType<typeof get>>;
  try {
    r = await get('/v1/reports/{report}/export', { params: { path: { report: key }, query: { ...query, format } }, parseAs: 'arrayBuffer' });
  } catch (e) {
    throw new ApiError(0, { code: 'NETWORK_ERROR', message: e instanceof Error ? e.message : String(e) });
  }
  if (!r.response.ok || !r.data) throw toApiError(r.response.status, r.error);

  const stamp = new Date().toISOString().slice(0, 10);
  const file = new File(exportDir(), `${safeFilename(`${title} ${stamp}`)}.${format}`);
  await file.write(new Uint8Array(r.data));

  const sharing = loadSharing();
  try {
    if (sharing) {
      await sharing.shareAsync(file.uri, { mimeType: MIME[format].mime, UTI: MIME[format].uti, dialogTitle: title });
    } else {
      await Share.share({ url: file.uri, title });
    }
  } finally {
    // iOS hands the file over before resolving; Android keeps it until the next export clears the folder.
    if (Platform.OS === 'ios' && file.exists) file.delete();
  }
}
