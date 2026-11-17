import { Directory, File, Paths } from 'expo-file-system';
import { Platform, Share } from 'react-native';

import { PANEL_MIRROR_PREFIX, refreshTokens } from '@/core/api/client';
import { ApiError } from '@/core/api/errors';
import { getTokens } from '@/core/auth/tokens';
import { API_URL, APP_VERSION, MOBILE_API_VERSION } from '@/core/config';
import { i18n } from '@/core/i18n';
import { loadSharing } from '@/features/feedback/native';

type Query = Record<string, string | string[] | undefined>;

function panelUrl(path: string, query: Query = {}) {
  const qs = Object.entries(query)
    .flatMap(([k, v]) => (v === undefined ? [] : (Array.isArray(v) ? v : [v]).map((x) => `${k}=${encodeURIComponent(x)}`)))
    .join('&');
  return `${API_URL}${PANEL_MIRROR_PREFIX}${path.replace(/^\/v1\//, '')}${qs ? `?${qs}` : ''}`;
}

/** Strips path separators, `..` and control characters from names built from codes or barcodes. */
export function safeFilename(name: string) {
  const cleaned = name.replace(/[\u0000-\u001f\u007f/\\]/g, '_').replace(/\.{2,}/g, '_').replace(/^\.+/, '').trim();
  return cleaned || 'document';
}

/** The app's own cache folder for downloaded PDFs; wiped before each new download. */
function pdfDir() {
  const dir = new Directory(Paths.cache, 'warehouse-pdf');
  if (dir.exists) dir.delete();
  dir.create({ intermediates: true });
  return dir;
}

/** Whether this device can hand a PDF to the system share sheet (print, Files, AirDrop). */
export const canSharePdf = Platform.OS === 'ios';

/**
 * Downloads a panel PDF (label sheet, end-of-day report) with the session
 * token and opens the system share sheet, where it can be printed, saved to
 * Files or sent. No extra native module: expo-file-system ships with expo,
 * React Native's Share takes a file URL on iOS.
 */
export async function sharePanelPdf(path: string, query: Query, filename: string, signal?: AbortSignal) {
  signal?.throwIfAborted();
  const t0 = getTokens();
  if (!t0 || t0.accessExpiresAt - Date.now() < 30_000) await refreshTokens();
  const token = getTokens()?.access;
  const safe = safeFilename(filename);
  const target = new File(pdfDir(), safe.endsWith('.pdf') ? safe : `${safe}.pdf`);
  let file: File;
  try {
    file = await File.downloadFileAsync(panelUrl(path, query), target, {
      headers: {
        Accept: 'application/pdf',
        'X-Mobile-Api-Version': MOBILE_API_VERSION,
        'X-App-Version': APP_VERSION,
        'Accept-Language': i18n.language,
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      idempotent: true,
    });
  } catch {
    // The download API reports HTTP failures without the error body.
    throw new ApiError(0, { code: 'PDF_DOWNLOAD_FAILED', message: 'PDF download failed' });
  }
  if (signal?.aborted) {
    file.delete();
    signal.throwIfAborted();
  }
  try {
    await Share.share({ url: file.uri, title: safe });
  } finally {
    // iOS hands the file over before resolving; Android keeps it until the next download clears the folder.
    if (Platform.OS === 'ios' && file.exists) file.delete();
  }
}

/** Whether a panel document (PDF or photo) can go to the share sheet: expo-sharing, or iOS Share with a file URL. */
export function canShareDocument() {
  return Platform.OS === 'ios' || !!loadSharing();
}

/**
 * TEC-1228: downloads an order document (shipping note, order slip, delivery
 * receipt photo) with the session token and opens the share sheet. Unlike
 * sharePanelPdf it also shares on Android through expo-sharing, and the
 * receipt may be an image: the extension comes from the response.
 */
export async function sharePanelDocument(path: string, title: string) {
  const t0 = getTokens();
  if (!t0 || t0.accessExpiresAt - Date.now() < 30_000) await refreshTokens();
  const token = getTokens()?.access;
  const dir = new Directory(Paths.cache, 'order-documents');
  if (dir.exists) dir.delete();
  dir.create({ intermediates: true });
  let file: File;
  try {
    // A folder destination keeps the server's file name and extension (receipt: pdf or image).
    file = await File.downloadFileAsync(panelUrl(path), dir, {
      headers: {
        'X-Mobile-Api-Version': MOBILE_API_VERSION,
        'X-App-Version': APP_VERSION,
        'Accept-Language': i18n.language,
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      idempotent: true,
    });
  } catch {
    throw new ApiError(0, { code: 'PDF_DOWNLOAD_FAILED', message: 'Document download failed' });
  }
  const type = file.type || 'application/pdf';
  const sharing = loadSharing();
  try {
    if (sharing) {
      await sharing.shareAsync(file.uri, { mimeType: type, dialogTitle: title });
    } else {
      await Share.share({ url: file.uri, title });
    }
  } finally {
    if (Platform.OS === 'ios' && file.exists) file.delete();
  }
}
