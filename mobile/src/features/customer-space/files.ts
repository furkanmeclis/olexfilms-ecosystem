import { useMutation } from '@tanstack/react-query';
import { Directory, File, Paths } from 'expo-file-system';
import { useEffect, useRef } from 'react';
import { Alert, Platform, Share } from 'react-native';

import { api, refreshTokens, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import type { components } from '@/core/api/schema';
import { getTokens } from '@/core/auth/tokens';
import { API_URL, APP_VERSION, MOBILE_API_VERSION } from '@/core/config';
import { errorMessage, i18n } from '@/core/i18n';

export type ExportJob = components['schemas']['ExportJob'];

const POLL_MS = 1500;
const POLL_MAX = 40;
const PORTAL_BASE = `${API_URL}/v1/mobile/portal`;

class Cancelled extends Error {}

/** Plain file name: no path separators or dot-only parts, never empty. */
export function safeName(name: string | undefined, fallback: string) {
  const clean = (name ?? '')
    .replace(/[\\/:*?"<>|\u0000-\u001f]+/g, '_')
    .replace(/\.{2,}/g, '_')
    .replace(/^[.\s]+|[.\s]+$/g, '')
    .trim();
  return clean || fallback;
}

const sleep = (ms: number, signal: AbortSignal) =>
  new Promise<void>((resolve, reject) => {
    if (signal.aborted) return reject(new Cancelled());
    const id = setTimeout(resolve, ms);
    signal.addEventListener('abort', () => {
      clearTimeout(id);
      reject(new Cancelled());
    });
  });

/** Waits for an export job (queued / processing) to complete; throws on failure, timeout or cancel. */
async function waitForJob(job: ExportJob, signal: AbortSignal): Promise<ExportJob> {
  let current = job;
  for (let i = 0; i < POLL_MAX; i++) {
    if (current.status === 'completed') return current;
    if (current.status === 'failed' || current.status === 'expired') throw new Error('EXPORT_FAILED');
    await sleep(POLL_MS, signal);
    current = await unwrap(api.GET('/v1/portal/exports/{uuid}', { params: { path: { uuid: job.uuid } } }));
  }
  throw new Error('EXPORT_TIMEOUT');
}

/** iOS: share sheet. Android: system viewer through the file preview (React Native's Share ignores files there). */
async function openFile(file: File) {
  if (Platform.OS === 'ios') {
    await Share.share({ url: file.uri });
    return;
  }
  if (await File.canPreview(file)) await File.preview(file);
  else throw new Error('NO_PREVIEW');
}

/** Own download folder in the cache; wiped before every download (exports can hold personal data). */
function freshDownloadDir() {
  const dir = new Directory(Paths.cache, 'exports');
  try {
    if (dir.exists) dir.delete();
  } catch {
    // Best effort: a file still held by a viewer is removed on a later run.
  }
  dir.create({ intermediates: true, idempotent: true });
  return dir;
}

/**
 * Downloads a portal file with the bearer token into the app's cache folder
 * and opens it. iOS deletes it once the share sheet closes; on Android the
 * viewer may read it after `File.preview` returns, so old files are cleared
 * before the next download instead. `path` is relative to `/v1/mobile/portal`.
 */
async function downloadAndOpen(path: string, filename: string, signal?: AbortSignal) {
  const t0 = getTokens();
  if (t0 && t0.accessExpiresAt - Date.now() < 30_000) await refreshTokens();
  const t = getTokens();
  const target = new File(freshDownloadDir(), filename);
  let downloaded: File | undefined;
  try {
    try {
      downloaded = await File.downloadFileAsync(`${PORTAL_BASE}${path}`, target, {
        idempotent: true,
        headers: {
          ...(t ? { Authorization: `Bearer ${t.access}` } : {}),
          'X-Mobile-Api-Version': MOBILE_API_VERSION,
          'X-App-Version': APP_VERSION,
        },
      });
    } catch {
      throw new Error('DOWNLOAD_FAILED');
    }
    if (signal?.aborted) throw new Cancelled();
    await openFile(downloaded);
  } finally {
    if (Platform.OS === 'ios') {
      try {
        (downloaded ?? target).delete();
      } catch {
        // Nothing to clean up (never created) or already gone.
      }
    }
  }
}

/** Runs an export request (job), waits for it and opens the file. */
async function exportAndOpen(start: () => Promise<ExportJob>, fallbackName: string, signal: AbortSignal) {
  const job = await waitForJob(await start(), signal);
  if (signal.aborted) throw new Cancelled();
  await downloadAndOpen(`/exports/${encodeURIComponent(job.uuid)}/download`, safeName(job.filename, fallbackName), signal);
}

export function fileErrorMessage(e: unknown) {
  if (e instanceof Error) {
    if (e.message === 'EXPORT_FAILED') return i18n.t('customer:export_failed');
    if (e.message === 'EXPORT_TIMEOUT') return i18n.t('customer:export_timeout');
    if (e.message === 'DOWNLOAD_FAILED') return i18n.t('customer:download_failed');
    if (e.message === 'NO_PREVIEW') return i18n.t('customer:no_viewer');
  }
  return errorMessage(isApiError(e) ? e.code : undefined);
}

/** Aborts running polls when the screen goes away: no share sheet after leaving. */
function useAbortOnUnmount() {
  const ref = useRef<AbortController>(new AbortController());
  useEffect(() => {
    const c = new AbortController();
    ref.current = c;
    return () => c.abort();
  }, []);
  return ref;
}

const onFileError = (e: unknown) => {
  if (e instanceof Cancelled) return;
  Alert.alert(fileErrorMessage(e));
};

/** Mutation hook: runs `start` (queues a job), downloads the result and opens it; alerts on failure. */
export function useExportDownload() {
  const abort = useAbortOnUnmount();
  return useMutation({
    mutationFn: (v: { start: () => Promise<ExportJob>; fallbackName: string; tag?: string }) =>
      exportAndOpen(v.start, v.fallbackName, abort.current.signal),
    onError: onFileError,
  });
}

/** Mutation hook: downloads a direct PDF (portal contract) and opens it. */
export function useDirectDownload() {
  const abort = useAbortOnUnmount();
  return useMutation({
    mutationFn: (v: { path: string; filename: string }) =>
      downloadAndOpen(v.path, safeName(v.filename, 'document.pdf'), abort.current.signal),
    onError: onFileError,
  });
}

export const portalFiles = {
  servicePdf: (uuid: string) => () => unwrap(api.GET('/v1/portal/services/{uuid}/pdf', { params: { path: { uuid } } })),
  warrantyCertificate: (uuid: string) => () =>
    unwrap(api.POST('/v1/portal/services/{uuid}/warranty-certificate', { params: { path: { uuid } }, body: {} })),
  dataExport: (format: 'json' | 'pdf') => () => unwrap(api.POST('/v1/portal/me/data-export', { body: { format } })),
  contractPath: (uuid: string) => `/contracts/${encodeURIComponent(uuid)}/pdf`,
};
