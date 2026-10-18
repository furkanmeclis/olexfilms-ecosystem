import { useMutation } from '@tanstack/react-query';
import { File, Paths } from 'expo-file-system';
import { Alert, Platform, Share } from 'react-native';

import { api, refreshTokens, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import type { components } from '@/core/api/schema';
import { getTokens } from '@/core/auth/tokens';
import { API_URL, APP_VERSION, MOBILE_API_VERSION } from '@/core/config';
import { errorMessage } from '@/core/i18n';

export type ExportJob = components['schemas']['ExportJob'];

const POLL_MS = 1500;
const POLL_MAX = 40;
const PORTAL_BASE = `${API_URL}/v1/mobile/portal`;

/** Plain file name: no path separators, never empty. */
function safeName(name: string | undefined, fallback: string) {
  const clean = (name ?? '').replace(/[\\/:*?"<>|]+/g, '_').trim();
  return clean || fallback;
}

/** Waits for an export job (queued / processing) to complete; throws on failure or timeout. */
async function waitForJob(job: ExportJob): Promise<ExportJob> {
  let current = job;
  for (let i = 0; i < POLL_MAX; i++) {
    if (current.status === 'completed') return current;
    if (current.status === 'failed' || current.status === 'expired') throw new Error('EXPORT_FAILED');
    await new Promise((r) => setTimeout(r, POLL_MS));
    current = await unwrap(api.GET('/v1/portal/exports/{uuid}', { params: { path: { uuid: job.uuid } } }));
  }
  throw new Error('EXPORT_TIMEOUT');
}

/**
 * Downloads a portal file with the bearer token into the cache and opens the
 * system share sheet (iOS: Save to Files, AirDrop, print). `path` is relative
 * to `/v1/mobile/portal`.
 */
async function downloadAndShare(path: string, filename: string) {
  const t0 = getTokens();
  if (t0 && t0.accessExpiresAt - Date.now() < 30_000) await refreshTokens();
  const t = getTokens();
  const file = new File(Paths.cache, filename);
  const downloaded = await File.downloadFileAsync(`${PORTAL_BASE}${path}`, file, {
    idempotent: true,
    headers: {
      ...(t ? { Authorization: `Bearer ${t.access}` } : {}),
      'X-Mobile-Api-Version': MOBILE_API_VERSION,
      'X-App-Version': APP_VERSION,
    },
  });
  // React Native's share sheet takes a file URL on iOS; Android has no file
  // sharing without a native module (see the PR notes).
  if (Platform.OS === 'ios') await Share.share({ url: downloaded.uri });
  else await Share.share({ message: filename, url: downloaded.uri });
}

/** Runs an export request (job), waits for it and shares the file. */
async function exportAndShare(start: () => Promise<ExportJob>, fallbackName: string) {
  const job = await waitForJob(await start());
  await downloadAndShare(`/exports/${encodeURIComponent(job.uuid)}/download`, safeName(job.filename, fallbackName));
}

export function fileErrorMessage(e: unknown) {
  return errorMessage(isApiError(e) ? e.code : undefined);
}

/** Mutation hook: runs `start` (queues a job), downloads the result and shares it; alerts on failure. */
export function useExportDownload() {
  return useMutation({
    mutationFn: (v: { start: () => Promise<ExportJob>; fallbackName: string; tag?: string }) => exportAndShare(v.start, v.fallbackName),
    onError: (e) => Alert.alert(fileErrorMessage(e)),
  });
}

/** Mutation hook: downloads a direct PDF (portal contract) and shares it. */
export function useDirectDownload() {
  return useMutation({
    mutationFn: (v: { path: string; filename: string }) => downloadAndShare(v.path, v.filename),
    onError: (e) => Alert.alert(fileErrorMessage(e)),
  });
}

export const portalFiles = {
  servicePdf: (uuid: string) => () => unwrap(api.GET('/v1/portal/services/{uuid}/pdf', { params: { path: { uuid } } })),
  warrantyCertificate: (uuid: string) => () =>
    unwrap(api.POST('/v1/portal/services/{uuid}/warranty-certificate', { params: { path: { uuid } }, body: {} })),
  dataExport: (format: 'json' | 'pdf') => () => unwrap(api.POST('/v1/portal/me/data-export', { body: { format } })),
  contractPath: (uuid: string) => `/contracts/${encodeURIComponent(uuid)}/pdf`,
};
