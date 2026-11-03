// Widget zaman çizelgesi: tazelik ve günlük kalan süre. Saf dosya: çeviri ve `build` parametre gelir.
import type { WidgetName } from './config.ts';
import { STALE_AFTER_MS } from './config.ts';
import type { TFn } from './format.ts';
import { staleAgo } from './format.ts';
import type { WidgetState } from './types.ts';

const HOUR_MS = 3_600_000;
const DAY_MS = 24 * HOUR_MS;

/** Tek `updateTimeline` çağrısındaki en fazla girdi. */
export const MAX_ENTRIES = 8;

/** Garanti widget'ları gün gün ilerler; diğerleri saatlik bayatlık eşiğiyle. */
const DAILY: readonly WidgetName[] = ['OlexKoruma', 'OlexGarantiKarti'];
const DAILY_DAYS = 7;

type Timed = { state: WidgetState; staleLabel: string | null };

export type TimelineEntry<P> = { date: Date; props: P };

/** `ok` olmayan durum tek `updateSnapshot`; `ok` ise `updateTimeline` girdileri. */
export type Timeline<P> = { mode: 'snapshot'; props: P } | { mode: 'timeline'; entries: TimelineEntry<P>[] };

function localDay(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

/** `now`'da `daysLeft` kalan garantinin `at` anındaki kalan günü (takvim günü farkı). */
export function daysLeftAt(daysLeft: number, now: Date, at: Date): number {
  return daysLeft - Math.round((localDay(at) - localDay(now)) / DAY_MS);
}

/** "Güncellenemedi. Son veri 2 saat önce." `t` widgets namespace'inin çevirisidir. */
export function staleLabelFor(t: TFn): (ageMs: number) => string {
  return (ageMs) => t('state.stale_body', { time: staleAgo(ageMs, t) });
}

type Slot = { date: Date; stale: boolean };

function slots(name: WidgetName, now: Date): Slot[] {
  const t0 = now.getTime();
  if (DAILY.includes(name)) {
    const out: Slot[] = [{ date: now, stale: false }];
    for (let k = 1; k <= DAILY_DAYS; k++) {
      out.push({ date: new Date(now.getFullYear(), now.getMonth(), now.getDate() + k), stale: k === DAILY_DAYS });
    }
    return out;
  }
  const staleAfter = STALE_AFTER_MS[name];
  const offsets = [...new Set([0, staleAfter, 6 * HOUR_MS, 24 * HOUR_MS])].sort((a, b) => a - b);
  return offsets.map((o) => ({ date: o === 0 ? now : new Date(t0 + o), stale: o >= staleAfter }));
}

/**
 * `build(at)` o an için `props` üretir (kalan gün ve düşen garanti `at`'a göre hesaplanır).
 * Bayat girdilerde `staleLabel` hazır metin olarak yazılır, widget saat hesaplamaz.
 * Politika `.atEnd`: son girdiden sonra sistem listeyi yeniden okur, son bayat hal kalır.
 */
export function buildEntries<P extends Timed>(
  name: WidgetName,
  now: Date,
  build: (at: Date) => P,
  staleText: (ageMs: number) => string,
): Timeline<P> {
  const first = build(now);
  if (first.state !== 'ok') return { mode: 'snapshot', props: { ...first, staleLabel: null } };

  const entries = slots(name, now)
    .slice(0, MAX_ENTRIES)
    .map(({ date, stale }, i) => {
      const props = i === 0 ? first : build(date);
      return { date, props: stale ? { ...props, staleLabel: staleText(date.getTime() - now.getTime()) } : props };
    });
  return { mode: 'timeline', entries };
}
