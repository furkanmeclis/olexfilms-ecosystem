/**
 * PeriodBar state ↔ URL. Parameter names match the report API
 * (`period`, `range_from`, `range_to`, `granularity`, `compare`) so the
 * query can be passed straight to `GET /v1/reports/{report}`.
 */

export const PERIOD_PRESETS = ["today", "7d", "30d", "90d", "12m"] as const;
export type PeriodPreset = (typeof PERIOD_PRESETS)[number];
export const GRANULARITIES = ["day", "week", "month"] as const;
export type PeriodGranularity = (typeof GRANULARITIES)[number];
export const COMPARE_MODES = ["none", "previous", "year"] as const;
export type CompareMode = (typeof COMPARE_MODES)[number];

export type PeriodState = {
  preset: PeriodPreset | "custom";
  /** yyyy-MM-dd, only for custom. */
  from?: string;
  to?: string;
  /** undefined = report default. */
  granularity?: PeriodGranularity;
  compare: CompareMode;
};

export const DEFAULT_PERIOD: PeriodState = { preset: "30d", compare: "none" };

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

type ParamsLike = { get(name: string): string | null };

function oneOf<T extends string>(
  list: readonly T[],
  v: string | null,
): T | undefined {
  return v && (list as readonly string[]).includes(v) ? (v as T) : undefined;
}

export function parsePeriodParams(
  params: ParamsLike,
  defaults: PeriodState = DEFAULT_PERIOD,
): PeriodState {
  const from = params.get("range_from");
  const to = params.get("range_to");
  const granularity =
    oneOf(GRANULARITIES, params.get("granularity")) ?? defaults.granularity;
  const compare =
    oneOf(COMPARE_MODES, params.get("compare")) ?? defaults.compare;
  if (from && to && DATE_RE.test(from) && DATE_RE.test(to)) {
    const [a, b] = from <= to ? [from, to] : [to, from];
    return { preset: "custom", from: a, to: b, granularity, compare };
  }
  const preset = oneOf(PERIOD_PRESETS, params.get("period"));
  return {
    preset: preset ?? defaults.preset,
    from: preset ? undefined : defaults.from,
    to: preset ? undefined : defaults.to,
    granularity,
    compare,
  };
}

/** API query of a period (empty fields omitted). */
export function periodToQuery(state: PeriodState): Record<string, string> {
  const q: Record<string, string> = {};
  if (state.preset === "custom") {
    if (state.from && state.to) {
      q.range_from = state.from;
      q.range_to = state.to;
    }
  } else {
    q.period = state.preset;
  }
  if (state.granularity) q.granularity = state.granularity;
  if (state.compare !== "none") q.compare = state.compare;
  return q;
}

/**
 * Writes the period into a copy of `current` (other params are kept);
 * values equal to the defaults are removed to keep URLs short.
 */
export function writePeriodParams(
  current: URLSearchParams | string,
  state: PeriodState,
  defaults: PeriodState = DEFAULT_PERIOD,
): URLSearchParams {
  const next = new URLSearchParams(current.toString());
  for (const k of [
    "period",
    "range_from",
    "range_to",
    "granularity",
    "compare",
  ])
    next.delete(k);
  const q = periodToQuery(state);
  if (q.period && q.period === defaults.preset && defaults.preset !== "custom")
    delete q.period;
  if (q.granularity && q.granularity === defaults.granularity)
    delete q.granularity;
  // TEC-1212: with a compare default (report detail: previous) "none" must
  // stay in the URL, and the default itself is left out.
  if (state.compare === defaults.compare) delete q.compare;
  else if (state.compare === "none") q.compare = "none";
  for (const [k, v] of Object.entries(q)) next.set(k, v);
  return next;
}

/** Suggested granularity for a preset (day up to 90 days, else month). */
export function suggestedGranularity(state: PeriodState): PeriodGranularity {
  if (state.preset === "12m") return "month";
  if (state.preset === "custom" && state.from && state.to) {
    const days =
      (Date.parse(state.to) - Date.parse(state.from)) / 86_400_000 + 1;
    if (days > 366) return "month";
    if (days > 120) return "week";
  }
  return "day";
}
