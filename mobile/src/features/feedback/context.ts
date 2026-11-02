import * as Application from 'expo-application';
import * as Device from 'expo-device';
import { getCalendars } from 'expo-localization';
import { Appearance, Dimensions, I18nManager, PixelRatio, Platform } from 'react-native';

import { recentApiErrors } from '@/core/api/error-log';
import type { MobileMe } from '@/core/auth/api';
import { API_URL, APP_VERSION } from '@/core/config';
import { i18n } from '@/core/i18n';
import { usePreferences } from '@/core/preferences';
import { activeMembership } from '@/core/spaces';
import { getUpdateInfo } from '@/core/updates';

import type { FeedbackMeta } from './meta';
import { loadNetwork } from './native';

/** Where the user was when they shook the phone (read from router hooks at that moment). */
export type LocationSnapshot = FeedbackMeta['location'];

function app(): FeedbackMeta['app'] {
  const ota = getUpdateInfo();
  return {
    version: APP_VERSION,
    build: Application.nativeBuildVersion,
    bundle_id: Application.applicationId,
    ota_enabled: ota.enabled,
    ota_channel: ota.channel ?? null,
    ota_runtime_version: ota.runtimeVersion ?? null,
    ota_update_id: ota.updateId ?? null,
    ota_embedded: ota.enabled ? ota.embedded : null,
    environment: __DEV__ ? 'development' : 'production',
    api_url: API_URL,
  };
}

async function network(): Promise<FeedbackMeta['device']['network']> {
  const mod = loadNetwork();
  if (!mod) return null;
  try {
    const s = await mod.getNetworkStateAsync();
    return { type: s.type ?? null, connected: s.isConnected ?? null, internet: s.isInternetReachable ?? null };
  } catch {
    return null;
  }
}

async function device(): Promise<FeedbackMeta['device']> {
  const screen = Dimensions.get('screen');
  return {
    brand: Device.brand ?? Device.manufacturer,
    model: Device.modelName ?? Device.modelId,
    os: Device.osName ?? Platform.OS,
    os_version: Device.osVersion ?? String(Platform.Version),
    screen: {
      width: Math.round(screen.width),
      height: Math.round(screen.height),
      scale: PixelRatio.get(),
      font_scale: PixelRatio.getFontScale(),
    },
    language: i18n.language,
    rtl: I18nManager.isRTL,
    theme_preference: usePreferences.getState().theme,
    color_scheme: Appearance.getColorScheme() ?? null,
    network: await network(),
    battery: null,
  };
}

function session(me: MobileMe | null): FeedbackMeta['session'] {
  const org = activeMembership(me);
  return {
    session_kind: me?.session_kind ?? null,
    user_uuid: me?.user.uuid ?? null,
    organization_uuid: org?.uuid ?? null,
    organization_type: org?.type ?? null,
    membership_role: org?.role ?? null,
    roles: me?.organization_roles ?? me?.roles ?? [],
    permission_count: me?.permissions.length ?? 0,
  };
}

/** Everything but the note and files, taken when the phone is shaken. */
export async function collectMeta(id: string, at: Date, location: LocationSnapshot, me: MobileMe | null): Promise<FeedbackMeta> {
  return {
    id,
    created_at: at.toISOString(),
    timezone: getCalendars()[0]?.timeZone ?? null,
    utc_offset_minutes: -at.getTimezoneOffset(),
    note: '',
    files: { original: 'original.png', annotated: null },
    location,
    session: session(me),
    app: app(),
    device: await device(),
    api_errors: recentApiErrors(),
  };
}

/** Summary at the top of `feedback.json` (the exporting device, now). */
export async function deviceSummary(): Promise<Record<string, unknown>> {
  return { app: app(), device: await device() };
}
