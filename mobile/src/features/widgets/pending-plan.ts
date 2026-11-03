// Bekleyenler satır kararı. Saf dosya: çekim sonuçlarının yalnız durumu gelir.
import type { PendingRowId } from './audience.ts';

export type RowStatus = 'ok' | 'blocked' | 'failed' | undefined;

export type PendingPlan =
  /** Gerçek hata: yazım atlanır, kayıt yazılmaz. */
  | { kind: 'failed' }
  /** Engel yüzünden satır kalmadı: `no_permission` kartı. */
  | { kind: 'blocked' }
  /** Satırlar yazılır; `dropped` engelli (403) satırlardır, listeden düşer. */
  | { kind: 'write'; dropped: PendingRowId[] };

/**
 * `status`: her satırın kaynağının sonucu (`ready`: hazır hizmetler, `claim`: açık talepler,
 * `expiring`: Son 30 gün özeti). Engelli (403) satır düşer, diğerleri kalır; sonucu olmayan
 * ya da hata veren istenmiş satır yazımı atlatır.
 */
export function planPending(rows: PendingRowId[], status: Partial<Record<PendingRowId, RowStatus>>): PendingPlan {
  if (rows.some((id) => status[id] !== 'ok' && status[id] !== 'blocked')) return { kind: 'failed' };
  const dropped = rows.filter((id) => status[id] === 'blocked');
  if (dropped.length === rows.length) return { kind: 'blocked' };
  return { kind: 'write', dropped };
}
