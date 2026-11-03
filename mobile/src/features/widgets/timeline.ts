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

type Timed = {
  state: WidgetState;
  staleLabel: string | null;
  /** Ok olmayan durumlarda tek görünen gövde; boş durumun bayat girdisi gövdeyi bayat metinle değiştirir. */
  card?: { body?: string };
};

/** Bayat metin: `openEnded` son girdidir, sonrasında sistem yeniden okumaz ve metin "…den eski" diye okunur. */
export type StaleText = (ageMs: number, openEnded: boolean) => string;

export type TimelineEntry<P> = { date: Date; props: P };

/** Saatlik widget'ta `ok` ve `empty` `updateTimeline` girdileri; diğer durumlar tek `updateSnapshot`. */
export type Timeline<P> = { mode: 'snapshot'; props: P } | { mode: 'timeline'; entries: TimelineEntry<P>[] };

function localDay(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

/** `now`'da `daysLeft` kalan garantinin `at` anındaki kalan günü (takvim günü farkı). */
export function daysLeftAt(daysLeft: number, now: Date, at: Date): number {
  return daysLeft - Math.round((localDay(at) - localDay(now)) / DAY_MS);
}

/**
 * Araçların aktif garantilerinin kalan gününü `at` anına kaydırır; kaynağa dokunmaz.
 * Süresi dolan garantiyi özet düşürür (`days_left <= 0` aktif sayılmaz).
 */
export function shiftWarranties<V extends { active_warranties: { days_left: number }[] }>(vehicles: V[], now: Date, at: Date): V[] {
  return vehicles.map((v) => ({
    ...v,
    active_warranties: v.active_warranties.map((w) => ({ ...w, days_left: daysLeftAt(w.days_left, now, at) })),
  }));
}

/**
 * "Güncellenemedi. Son veri 2 saat önce." `t` widgets namespace'inin çevirisidir.
 * Açık uçlu (son girdi): "Güncellenemedi. Son veri 1 günden eski."
 */
export function staleLabelFor(t: TFn): StaleText {
  return (ageMs, openEnded = false) =>
    openEnded
      ? t('state.stale_older', { count: Math.max(1, Math.floor(ageMs / DAY_MS)) })
      : t('state.stale_body', { time: staleAgo(ageMs, t) });
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
 * Saatlik widget'ta `empty` de zaman çizelgesidir: bayat girdi `empty` kalır, kart gövdesi bayat metin olur
 * (yanlış "bekleyen yok" okunmasın). Günlük widget'lar (Koruma, Kart) ve diğer durumlar tek anlık görüntüdür.
 * Politika `.atEnd`: son girdiden sonra sistem listeyi yeniden okur, son bayat hal kalır;
 * bu yüzden son girdinin metni açık uçludur ve değerler son girdide donar.
 */
export function buildEntries<P extends Timed>(
  name: WidgetName,
  now: Date,
  build: (at: Date) => P,
  staleText: StaleText,
): Timeline<P> {
  const first = build(now);
  const timed = first.state === 'ok' || (first.state === 'empty' && !DAILY.includes(name));
  if (!timed) return { mode: 'snapshot', props: { ...first, staleLabel: null } };

  const all = slots(name, now).slice(0, MAX_ENTRIES);
  const entries = all.map(({ date, stale }, i) => {
    const props = i === 0 ? first : build(date);
    if (!stale) return { date, props };
    const text = staleText(date.getTime() - now.getTime(), i === all.length - 1);
    const card = props.state === 'empty' && props.card ? { card: { ...props.card, body: text } } : {};
    return { date, props: { ...props, ...card, staleLabel: text } };
  });
  return { mode: 'timeline', entries };
}
