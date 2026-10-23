/**
 * SAPL interpretation of a paint thickness (pure, alias-free): the NexPTG
 * app's Interpretation.getInterpretation (REVERSE_ENGINEERING §7).
 *
 * Level 0 paint too thin, 1 original paint, 2 second coat, 3 thin filler,
 * 4 / 5 thick filler. General mode uses fixed limits; individual mode limits
 * around a reference value measured on the car (outside 105 µm, inside 60 µm
 * by default).
 */
export type SaplPlace = 'outside' | 'inside';
export type SaplLevel = 0 | 1 | 2 | 3 | 4 | 5;

/** color_interpretations of the NexPTG app, by level. */
export const SAPL_COLORS: Record<SaplLevel, string> = {
  0: '#E9DF28',
  1: '#55D37A',
  2: '#DEB50A',
  3: '#EC4A08',
  4: '#AF0025',
  5: '#AF0025',
};

export const SAPL_DEFAULT_REFERENCE: Record<SaplPlace, number> = { outside: 105, inside: 60 };

/** Upper limits (inclusive) of levels 0–4; above the last is level 5. */
const GENERAL: Record<SaplPlace, number[]> = {
  outside: [69, 165, 300, 500, 700],
  inside: [39, 90, 150, 350, 550],
};

/** Individual mode: limits relative to the reference. */
const INDIVIDUAL: Record<SaplPlace, number[]> = {
  outside: [-36, 35, 170, 370, 570],
  inside: [-31, 30, 90, 290, 490],
};

export function saplLevel(um: number, place: SaplPlace = 'outside', reference?: number | null): SaplLevel {
  const limits = reference != null ? INDIVIDUAL[place].map((d) => reference + d) : GENERAL[place];
  const i = limits.findIndex((max) => um <= max);
  return (i < 0 ? 5 : i) as SaplLevel;
}
