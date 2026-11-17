import { useQuery } from '@tanstack/react-query';
import { Directory, File, Paths } from 'expo-file-system';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Platform, Share } from 'react-native';

import { api, refreshTokens, unwrap } from '@/core/api/client';
import { ApiError, toApiError } from '@/core/api/errors';
import { getTokens } from '@/core/auth/tokens';
import type { components } from '@/core/api/schema';
import { APP_VERSION, MOBILE_API_VERSION } from '@/core/config';
import { i18n } from '@/core/i18n';
import { loadSharing } from '@/features/feedback/native';
import { PickerSheet } from '@/features/picker-sheet';

import { alertError, whError } from './format';
import { panelUrl, safeFilename } from './pdf-share';

type Template = components['schemas']['LabelTemplate'];
type Query = Record<string, string | string[] | undefined>;

/** What to print: a label endpoint, its query and the barcodes it covers. */
export type LabelTarget = {
  path: string;
  query: Query;
  name: string;
  barcodes: string[];
};

/** Whether this binary can hand a label file to the share sheet (print apps, Zebra apps, Files). */
export function canShareLabels() {
  return Platform.OS === 'ios' || !!loadSharing();
}

function labelDir() {
  const dir = new Directory(Paths.cache, 'warehouse-labels');
  if (dir.exists) dir.delete();
  dir.create({ intermediates: true });
  return dir;
}

/**
 * Downloads a label document (TEC-1230: A4 sheet, label-sized PDF or ZPL
 * by the template's output format) and opens the share sheet: AirPrint or
 * the Android print service for a PDF, the Zebra / printer app for a .zpl
 * file. No printer SDK on the device.
 */
export async function shareLabels(target: LabelTarget, template?: Template) {
  const t0 = getTokens();
  if (!t0 || t0.accessExpiresAt - Date.now() < 30_000) await refreshTokens();
  const token = getTokens()?.access;
  const zpl = template?.output_format === 'zpl';
  const ext = zpl ? 'zpl' : 'pdf';
  const query = template ? { ...target.query, template: template.uuid } : target.query;
  const file = new File(labelDir(), `${safeFilename(target.name)}.${ext}`);
  let out: File;
  try {
    out = await File.downloadFileAsync(panelUrl(target.path, query), file, {
      headers: {
        Accept: zpl ? 'application/vnd.zebra.zpl' : 'application/pdf',
        'X-Mobile-Api-Version': MOBILE_API_VERSION,
        'X-App-Version': APP_VERSION,
        'Accept-Language': i18n.language,
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      idempotent: true,
    });
  } catch {
    throw new ApiError(0, {
      code: 'PDF_DOWNLOAD_FAILED',
      message: 'Label download failed',
    });
  }
  const sharing = loadSharing();
  try {
    if (sharing) {
      await sharing.shareAsync(out.uri, {
        mimeType: zpl ? 'text/plain' : 'application/pdf',
        UTI: zpl ? 'public.plain-text' : 'com.adobe.pdf',
        dialogTitle: target.name,
      });
    } else {
      await Share.share({ url: out.uri, title: target.name });
    }
  } finally {
    if (Platform.OS === 'ios' && out.exists) out.delete();
  }
}

type LooseResult = { data?: unknown; error?: unknown; response: Response };
const loose = api as unknown as { POST: (path: string, init: { body: unknown }) => Promise<LooseResult> };

/**
 * Marks reserved units printed after the user confirmed the print
 * (POST /v1/stock/labels/units/mark-printed, TEC-1224). Untyped on purpose:
 * the route may be missing on an older server, which answers 404/405 and
 * leaves the units as they were (the old GET still marked them printed).
 */
export async function markUnitsPrinted(barcodes: string[]) {
  let r: LooseResult;
  try {
    r = await loose.POST('/v1/stock/labels/units/mark-printed', { body: { barcodes } });
  } catch (e) {
    throw new ApiError(0, { code: 'NETWORK_ERROR', message: e instanceof Error ? e.message : String(e) });
  }
  if (r.response.ok || r.response.status === 404 || r.response.status === 405) return;
  throw toApiError(r.response.status, r.error);
}

/** Unit label templates of the organization (GET /v1/stock/label-templates?kind=unit). */
function useUnitTemplates(enabled: boolean) {
  return useQuery({
    queryKey: ['warehouse', 'label-templates', 'unit'],
    enabled,
    staleTime: 60_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/stock/label-templates', {
          params: { query: { kind: 'unit' } },
        }),
      ),
    select: (d) => d.items.filter((x) => x.active),
  });
}

/**
 * "Print labels" with a template choice (TEC-1230). After the share sheet
 * closes the user confirms the print; only then are reserved units marked
 * printed (POST /v1/stock/labels/units/mark-printed, TEC-1224), so a
 * preview does not lock the entry.
 */
export function useLabelPrint(onPrinted?: () => void) {
  const { t } = useTranslation('warehouse');
  const [target, setTarget] = useState<LabelTarget | null>(null);
  const [busy, setBusy] = useState(false);
  const templates = useUnitTemplates(!!target);

  const print = async (tg: LabelTarget, tpl?: Template) => {
    setBusy(true);
    try {
      await shareLabels(tg, tpl);
    } catch (e) {
      alertError(e);
      return;
    } finally {
      setBusy(false);
    }
    if (tg.barcodes.length === 0) return;
    Alert.alert(t('labels.printed_question'), t('labels.printed_question_body'), [
      { text: t('labels.printed_no'), style: 'cancel' },
      {
        text: t('labels.printed_yes'),
        onPress: () =>
          void markUnitsPrinted(tg.barcodes)
            .then(() => onPrinted?.())
            .catch(alertError),
      },
    ]);
  };

  const list = templates.data ?? [];
  const sheet = (
    <PickerSheet
      visible={!!target}
      title={t('labels.pick_template')}
      options={[
        { key: '', title: t('labels.default_template') },
        ...list.map((x) => ({
          key: x.uuid,
          title: x.name,
          subtitle: `${t(`labels.output.${x.output_format ?? 'sheet'}`)} · ${x.width_mm}×${x.height_mm} mm`,
        })),
      ]}
      loading={templates.isLoading}
      error={templates.error}
      errorMessage={templates.error ? whError(templates.error) : undefined}
      onRetry={() => void templates.refetch()}
      onPick={(o) => {
        const tg = target;
        if (!tg) return;
        void print(
          tg,
          list.find((x) => x.uuid === o.key),
        );
      }}
      onClose={() => setTarget(null)}
    />
  );
  return { open: (tg: LabelTarget) => setTarget(tg), busy, sheet };
}
