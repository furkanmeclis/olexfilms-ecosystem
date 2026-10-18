import { File, Paths } from 'expo-file-system';
import { Platform, Share } from 'react-native';

import { PANEL_MIRROR_PREFIX, refreshTokens } from '@/core/api/client';
import { ApiError } from '@/core/api/errors';
import { getTokens } from '@/core/auth/tokens';
import { API_URL, APP_VERSION, MOBILE_API_VERSION } from '@/core/config';
import { i18n } from '@/core/i18n';

type Query = Record<string, string | string[] | undefined>;

function panelUrl(path: string, query: Query = {}) {
  const qs = Object.entries(query)
    .flatMap(([k, v]) => (v === undefined ? [] : (Array.isArray(v) ? v : [v]).map((x) => `${k}=${encodeURIComponent(x)}`)))
    .join('&');
  return `${API_URL}${PANEL_MIRROR_PREFIX}${path.replace(/^\/v1\//, '')}${qs ? `?${qs}` : ''}`;
}

/** Whether this device can hand a PDF to the system share sheet (print, Files, AirDrop). */
export const canSharePdf = Platform.OS === 'ios';

/**
 * Downloads a panel PDF (label sheet, end-of-day report) with the session
 * token and opens the system share sheet, where it can be printed, saved to
 * Files or sent. No extra native module: expo-file-system ships with expo,
 * React Native's Share takes a file URL on iOS.
 */
export async function sharePanelPdf(path: string, query: Query, filename: string) {
  const t0 = getTokens();
  if (!t0 || t0.accessExpiresAt - Date.now() < 30_000) await refreshTokens();
  const token = getTokens()?.access;
  const target = new File(Paths.cache, filename.endsWith('.pdf') ? filename : `${filename}.pdf`);
  if (target.exists) target.delete();
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
  await Share.share({ url: file.uri, title: filename });
}
