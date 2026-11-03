// Paylaşılan alandaki görsel dosya adları ve tazelik kuralları. Saf dosya.
import { MARK_FILE, MARK_MONO_FILE } from './config.ts';

/** Aynı dosya bu süreden yeniyse yeniden indirilmez (`Cache` 304'e güvenilmez). */
export const FRESH_MS = 7 * 24 * 3_600_000;

const SAFE_PART = /^[A-Za-z0-9_-]{1,64}$/;

const PROTECTED = new Set([MARK_FILE, MARK_MONO_FILE]);

function named(prefix: string, part: string): string | null {
  return SAFE_PART.test(part) ? `${prefix}${part}.png` : null;
}

type HeroSource = { car_model?: { uuid?: string | null } | null; car_brand?: { uuid?: string | null } | null };

/** `heroUrl` ile aynı sıra: model, marka, varsayılan. Güvenli olmayan kimlik `null`. */
export function heroFileName(v: HeroSource): string | null {
  if (v.car_model?.uuid) return named('hero-m-', v.car_model.uuid);
  if (v.car_brand?.uuid) return named('hero-b-', v.car_brand.uuid);
  return 'hero-default.png';
}

export function logoFileName(brandUuid: string): string | null {
  return named('logo-', brandUuid);
}

export function qrFileName(code: string): string | null {
  return named('qr-', code);
}

/** `keep` içinde olmayan dosyalar; Olex işaretleri hiçbir zaman silinmez. */
export function staleFiles(existing: string[], keep: Set<string>): string[] {
  return existing.filter((name) => !PROTECTED.has(name) && !keep.has(name));
}

/** `modifiedMs` yedi günden yeniyse true; bilinmeyen veya gelecekteki zaman bayat sayılır. */
export function isFresh(modifiedMs: number | null, now: number): boolean {
  if (modifiedMs == null) return false;
  const age = now - modifiedMs;
  return age >= 0 && age < FRESH_MS;
}
