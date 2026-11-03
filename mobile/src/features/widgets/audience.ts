// Hedef kitle ve widget durumu. Saf dosya: rol, izin ve modül girdisi hazır gelir
// (`core/spaces.ts` expo-router import ettiği için burada okunamaz).
import type { SpaceKind } from '@/core/spaces';

import type { WidgetName } from './config.ts';
import { CUSTOMER_WIDGETS, DEALER_WIDGETS } from './config.ts';
import type { WidgetState } from './types.ts';

export type AudienceInput = {
  kinds: SpaceKind[];
  permissions: Set<string>;
  superAdmin: boolean;
  /** `null`: modül listesi henüz bilinmiyor, hepsi açık sayılır. */
  enabledModules: Set<string> | null;
};

export type Audience = 'customer' | 'dealer' | 'none';

export type QuickActionId = 'scan' | 'new_service' | 'measure' | 'customers' | 'services';
export type PendingRowId = 'ready' | 'claim' | 'expiring';

const MAX_QUICK_ACTIONS = 4;

export function audienceOf(input: AudienceInput): Audience {
  if (input.kinds.includes('musteri')) return 'customer';
  if (input.kinds.includes('bayi')) return 'dealer';
  return 'none';
}

export function can(input: AudienceInput, permission: string): boolean {
  return input.superAdmin || input.permissions.has(permission);
}

/** `core/modules.ts` ile aynı: bilinmiyorsa açık. */
export function moduleOn(input: AudienceInput, key: string): boolean {
  return input.enabledModules === null || input.enabledModules.has(key);
}

/**
 * Hızlı işlem düğmeleri. Tara her zaman ilk ve ön koşulsuzdur, küçük boy yalnız
 * onu çizer. Müşteri kaydı yoksa yerine hizmetler gelir; müşteri sayısı yalnız
 * `customers` varsa gösterilir.
 */
export function quickActions(input: AudienceInput): { id: QuickActionId }[] {
  const ids: QuickActionId[] = ['scan'];
  if (can(input, 'services.write')) ids.push('new_service');
  // Ölçüm sekmesi `services.write` ister (`bayi/_layout.tsx`); gizli sekmeye gidilemez, o yüzden ikisi de gerekir.
  if (can(input, 'services.write') && can(input, 'measurements.read') && moduleOn(input, 'measurements')) ids.push('measure');
  if (can(input, 'customers.read')) ids.push('customers');
  else if (can(input, 'services.read')) ids.push('services');
  return ids.slice(0, MAX_QUICK_ACTIONS).map((id) => ({ id }));
}

/**
 * Bekleyenler satırları. Tasarım modül kapalıyken tüm widget'ı kapatıyor; burada
 * erişilebilir satırlar gizlenmesin diye satır bazlı seçilir.
 */
export function pendingRows(input: AudienceInput): PendingRowId[] {
  const rows: PendingRowId[] = [];
  if (can(input, 'services.read')) rows.push('ready');
  if (can(input, 'warranty_claims.read') && moduleOn(input, 'warranty_claims')) rows.push('claim');
  if (can(input, 'warranties.read')) rows.push('expiring');
  return rows;
}

/** Bekleyenler'de satır kalmadıysa neden: talep modülü kapalıysa `module_off`, yoksa `no_permission`. */
export function pendingBlockedState(input: AudienceInput): WidgetState {
  if (pendingRows(input).length > 0) return 'ok';
  return moduleOn(input, 'warranty_claims') ? 'no_permission' : 'module_off';
}

/**
 * Widget'ın veri çekmeden önceki durumu. `forbidden`: sunucu 403 döndü
 * (Son 30 gün için tek yetki kaynağı sonuçtur). Modül kapalılığı izin eksikliğinden
 * önce gelir.
 */
export function resolveState(
  name: WidgetName,
  input: AudienceInput,
  result: { forbidden?: boolean } = {},
): WidgetState {
  const audience = audienceOf(input);
  if (CUSTOMER_WIDGETS.includes(name)) return audience === 'customer' ? 'ok' : 'wrong_role';
  if (!DEALER_WIDGETS.includes(name) || audience !== 'dealer') return 'wrong_role';

  switch (name) {
    case 'OlexPanom':
      return can(input, 'services.read') ? 'ok' : 'no_permission';
    case 'OlexSon30Gun':
      return result.forbidden ? 'no_permission' : 'ok';
    case 'OlexCiro':
      if (!moduleOn(input, 'accounting')) return 'module_off';
      return can(input, 'accounting.read') ? 'ok' : 'no_permission';
    case 'OlexBekleyenler':
      return pendingBlockedState(input);
    default:
      return 'ok';
  }
}
