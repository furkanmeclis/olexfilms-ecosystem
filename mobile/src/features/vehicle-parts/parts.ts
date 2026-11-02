/**
 * Vehicle body diagram: selectable parts, quick-select presets and key helpers.
 *
 * Part keys match the backend (`applied_parts`, `part_key`, product category
 * `available_parts`; label `dealer:part.<key>`). The shape paths come from the legacy mobile
 * app's `vehicle-scheme` file; its English ids (such as `body_hood`) are accepted as aliases.
 * Plain data + functions: no React and no `@/` imports (runs directly under node --test).
 */

export const VEHICLE_VIEWBOX = { width: 1216, height: 927 } as const;

export type VehiclePartCategory = 'body' | 'bumpers' | 'fenders' | 'doors' | 'windows';

export type VehiclePart = {
  /** Backend part key. */
  key: string;
  /** Id in the legacy mobile app (alias). */
  legacyId: string;
  category: VehiclePartCategory;
  d: string;
  evenOdd?: boolean;
};

export const VEHICLE_CATEGORY_ORDER: readonly VehiclePartCategory[] = ['body', 'bumpers', 'fenders', 'doors', 'windows'];

export const VEHICLE_PARTS: readonly VehiclePart[] = [
  {
    key: 'body_tavan',
    legacyId: 'body_roof',
    category: 'body',
    evenOdd: true,
    d: 'm619 463c0 44.6-11 90.5-11 90.5h-185c0 0-10-50.7-10-90.5 0-39.8 10-92 10-92h185c0 0 11 47.4 11 92z',
  },
  {
    key: 'body_kaput',
    legacyId: 'body_hood',
    category: 'body',
    evenOdd: true,
    d: 'm734 324.2c0 0 119.5 0 143 0 23.5 0 32.6 0.3 49 24.8 16.4 24.5 29 72.9 29 114 0 41.1-12.6 88-29 112.4-16.4 24.4-25.5 24.6-49 24.6-23.5 0-143 0-143 0 25.2-43.9 26-137 26-137 0 0-0.8-94.6-26-138.8z',
  },
  {
    key: 'body_bagaj',
    legacyId: 'body_trunk',
    category: 'body',
    evenOdd: true,
    d: 'm242 567.4c10 20.7 30.2 32.6 55 32.6 24.8 0 40 0 40 0 0 0-13-81.3-13-137 0-55.7 13-138.8 13-138.8 0 0-15.2 0-40 0-24.8 0-45 12-55 32.8-10 20.8-15 45-15 105 0 60 5 84.7 15 105.4z',
  },
  {
    key: 'body_on_tampon',
    legacyId: 'bumper_front',
    category: 'bumpers',
    evenOdd: true,
    d: 'm1056 306.6c13.9-8.6 72-8.4 72-8.4 13.3 0 24 11.6 24 25.8v277.1c0 14.3-10.7 25.9-24 25.9 0 0-58.1 0.2-72-8.4zm40.5 298.6c16.2-6.6 22.1-31.6 22.1-50 0-18.3 0.6-29.7-21.1-24-21.7 5.8-24.2 61-24.2 61-2.1 25.8 6.9 19.7 23.2 13zm-22.9-272.8c0 0 2.4 53.7 24 59.2 21.6 5.6 21-5.4 21-23.3 0-17.8-5.9-42.1-22-48.6-16.1-6.4-25.1-12.4-23 12.7z',
  },
  {
    key: 'body_arka_tampon',
    legacyId: 'bumper_rear',
    category: 'bumpers',
    evenOdd: true,
    d: 'm156 596.1c0 0-15.1 11.1-26.7 11.1-18.2 0-38.9 0-38.9 0-9.9 0-17.9-8.5-17.9-19 0 0-4.5-62.5-4.5-125.4 0-63.2 4.5-127 4.5-127 0-10.5 8-19 17.9-19 0 0 29.4 0 38.8 0 9.3 0 26.8 7.9 26.8 7.9zm-45-249v54.2c0 6.6 5.4 12 12 12h1c6.6 0 12-5.4 12-12v-54.2c0-6.7-5.4-12-12-12h-1c-6.6 0-12 5.3-12 12zm0 176.5v54.2c0 6.7 5.4 12 12 12h1c6.6 0 12-5.3 12-12v-54.2c0-6.7-5.4-12-12-12h-1c-6.6 0-12 5.3-12 12z',
  },
  {
    key: 'body_sol_on_camurluk',
    legacyId: 'fender_left_front',
    category: 'fenders',
    evenOdd: true,
    d: 'm743.9 72.7c0 14.2 0 90.5 80.4 88.6 83.8-2.1 74.8-83.4 74.8-93.8 0-4.3 3.8-3.3 7.9-3.3 4.1 0 43 3.3 46 4 3 0.6 3.7 5.1 3 11.8-0.7 6.7 14.5 48.3-4 65-18.5 16.7-60.5 31.3-114 35.5-77.7 6.1-112.9 24.6-112.9 24.6 0 0 24-55.9 8-141.9h11z',
  },
  {
    key: 'body_sol_arka_camurluk',
    legacyId: 'fender_left_rear',
    category: 'fenders',
    evenOdd: true,
    d: 'm379.1 157.8c0 0-15 5-31.9 5-49.3 0-71.5-47.8-71.5-69.5 0-7.7-1.6-9.5-6.9-9.5-5.4 0-17.5 4.2-22.6 12.1-5.1 7.9-18.9 26.5-25.4 36.2-3.7 5.6 0.9 19.8 0.9 19.8 0 0 1.7 18.1 4.4 24.1 2.7 6 3.9 13.8 2.6 19.8-1.3 6-2 6.5 4.4 11.2 6.3 4.6 29.6 6 42.8 5.1 13.1-0.8 51.8 4.7 65.3 1.9 13.4-2.8 16.3-12.3 37.9-56.2z',
  },
  {
    key: 'body_sag_on_camurluk',
    legacyId: 'fender_right_front',
    category: 'fenders',
    evenOdd: true,
    d: 'm743.9 854.4c0-14.4 0-91.9 80.4-89.9 83.8 2.1 74.8 84.6 74.8 95.2 0 4.3 3.8 3.3 7.9 3.3 4.1 0 43-3.4 46-4 3-0.6 3.7-5.2 3-12-0.7-6.8 14.5-49.1-4-66-18.5-16.9-60.5-31.7-114-36-77.7-6.2-113-25-113-25 0 0 24 56.8 8 144h11z',
  },
  {
    key: 'body_sag_arka_camurluk',
    legacyId: 'fender_right_rear',
    category: 'fenders',
    evenOdd: true,
    d: 'm379 768c0 0-15-5-32-5-49.3 0-71.4 48.5-71.4 70.5 0 7.8-1.6 9.6-7 9.6-5.4 0-17.5-4.2-22.6-12.2-5.1-8-18.9-26.9-25.3-36.7-3.8-5.8 0.8-20.1 0.8-20.1 0 0 1.7-18.4 4.4-24.5 2.7-6.1 3.9-14 2.6-20.1-1.3-6.1-2-6.6 4.4-11.4 6.3-4.7 29.7-6.1 42.8-5.2 13.2 0.8 51.9-4.7 65.3-1.9 13.4 2.8 16.4 12.5 38 57z',
  },
  {
    key: 'body_sol_on_kapi',
    legacyId: 'door_left_front',
    category: 'doors',
    evenOdd: true,
    d: 'm552.1 66.2c0 0 5.7 111.2-15 201 0 0 74.9 0.9 145-40.4 35.6-21 37.6-26.8 43-52.2 5.3-25.4 7.6-72.3 1-108.4z',
  },
  {
    key: 'body_sol_arka_kapi',
    legacyId: 'door_left_rear',
    category: 'doors',
    evenOdd: true,
    d: 'm528.1 267.2c0 0 19.7-81.2 15-200 0 0-75.1 0-91 0-8.7 0-18.2 0.3-22 29.5-5 39.6-27 51.5-38 59.2-8.6 6-21 39.6-24 47.3-2.9 7.7-3.7 21.1 7 30.5 10.7 9.4 37.4 23.6 69 28.6 31.6 4.9 84 4.9 84 4.9z',
  },
  {
    key: 'body_sag_on_kapi',
    legacyId: 'door_right_front',
    category: 'doors',
    evenOdd: true,
    d: 'm552 861c0 0 5.7-112.9-15-204 0 0 74.9-0.9 145 41 35.6 21.3 37.7 27.2 43 53 5.3 25.8 7.7 73.4 1 110z',
  },
  {
    key: 'body_sag_arka_kapi',
    legacyId: 'door_right_rear',
    category: 'doors',
    evenOdd: true,
    d: 'm528 657c0 0 19.7 82.5 15 203 0 0-75.1 0-91 0-8.7 0-18.2-0.3-22-30-5.1-40.1-27-52.2-38-60-8.6-6.1-21-40.2-24-48-3-7.8-3.7-21.4 7-31 10.7-9.6 37.4-24 69-29 31.6-5 84-5 84-5z',
  },
  {
    key: 'window_on_cam',
    legacyId: 'window_front',
    category: 'windows',
    evenOdd: true,
    d: 'm622 378c-2.8-9.3 2.8-16.1 19-21 16.2-4.9 53.2-15.1 63-18 9.8-2.9 21.3-7.5 27 9 5.7 16.5 15 64.5 15 115 0 50.5-9.3 97-15 113.4-5.7 16.3-17.2 11.8-27 8.9-9.8-2.9-46.8-13-63-17.9-16.2-4.9-21.8-11.7-19-20.9 2.8-9.2 8-53 8-83.5 0-30.5-5.2-75.7-8-85z',
  },
  {
    key: 'window_arka_cam',
    legacyId: 'window_rear',
    category: 'windows',
    evenOdd: true,
    d: 'm405 388c2.7-10.5 2.4-16-8-21-10.4-5-22.3-11.3-31-15-8.7-3.7-17.1-9.2-21 9-3.9 18.2-8 89-8 102 0 13 4.1 82.3 8 100.4 3.9 18.1 12.3 12.6 21 9 8.7-3.7 20.6-9.9 31-14.9 10.4-5.1 10.7-10.5 8-20.9-2.7-10.5-4-54.3-4-73.6 0-19.3 1.3-64.5 4-75z',
  },
  {
    key: 'window_sunroof',
    legacyId: 'window_sunroof',
    category: 'windows',
    d: 'm505 383h72c5 0 9 4 9 9v139c0 5-4 9-9 9h-72c-5 0-9-4-9-9v-139c0-5 4-9 9-9z',
  },
  {
    key: 'window_sol_on_kapi',
    legacyId: 'window_left_front_door',
    category: 'windows',
    evenOdd: true,
    d: 'm555.1 219.9c-3.5 19.9-4.9 31-6 34.5-1.1 3.5 0.7 6.4 11 4.9 10.3-1.5 97.5-16.4 145-56.1 0 0-12.9 0-20 0-7.2 0-13-6.6-13-12.9 0-6.2 2.9-13.7 14-13.7 11.1 0 23 0 23 0 0 0-3.5-4-13-4-9.6 0-29-1.2-29 17.7 0 0-76.5 2.9-89 4-12.5 1.1-18.8 1.7-23 25.6z',
  },
  {
    key: 'window_sol_arka_kapi',
    legacyId: 'window_left_rear_door',
    category: 'windows',
    evenOdd: true,
    d: 'm374.1 214c0-8.6 6.8-15.8 23-15.8 16.3 0 103 0 114 0 11 0 17 0 17 12.8 0 12.9-5.7 42.5-7 46.3-1.2 3.9-6.4 4-15 4-8.6 0-68.8-4.4-87-10.8-18.1-6.5-45-20.3-45-36.5z',
  },
  {
    key: 'window_sag_on_kapi',
    legacyId: 'window_right_front_door',
    category: 'windows',
    evenOdd: true,
    d: 'm555 705c-3.5-20.2-4.9-31.4-6-35-1.1-3.6 0.7-6.5 11-5 10.3 1.5 97.5 16.7 145 57 0 0-12.8 0-20 0-7.2 0-13 6.6-13 13 0 6.4 2.9 14 14 14 11.1 0 23 0 23 0 0 0-3.5 4-13 4-9.5 0-29 1.3-29-18 0 0-76.5-2.8-89-4-12.5-1.2-18.8-1.8-23-26z',
  },
  {
    key: 'window_sag_arka_kapi',
    legacyId: 'window_right_rear_door',
    category: 'windows',
    evenOdd: true,
    d: 'm374 711c0 8.7 6.7 16 23 16 16.3 0 103 0 114 0 11 0 17 0.1 17-13 0-13.1-5.8-43.1-7-47-1.2-3.9-6.4-4-15-4-8.6 0-68.8 4.4-87 11-18.2 6.6-45 20.6-45 37z',
  },
];

/** Non-selectable outline layers (from the legacy app). */
export const VEHICLE_OUTLINES: readonly { d: string; evenOdd?: boolean }[] = [
  {
    evenOdd: true,
    d: 'm220.3 558.1c3.1 11.6 7.5 20.6 13.7 27.1 18 19 38.2 35.8 93 35.8 54.8 0 371 0 371 0 0 0-11.9 9.5-14 18.9-2.1 9.4-2.4 15.2 13 13.9 20.5-1.7 28.1-29.1 30-33.8 0 0 113.3 0 146 0 46.8 0 64.7-25 74-42.7 9.3-17.7 28-58.8 28-115.3 0-56.5-18.7-97.2-28-115-9.3-17.8-27.2-43-74-43-32.7 0-146 0-146 0-1.9-4.7-9.5-32.3-30-34-15.4-1.3-15.1 4.5-13 14 2.1 9.5 14 19 14 19 0 0-316.2 0-371 0-54.8 0-75 17-93 36-18 19-21 61-21 124 0 31.1 0.7 56.6 3.6 76.8 0 0-9.7 0-11.6 0-1.9 0-5 0.7-5 4.2 0 3.5 0 5.8 0 8 0 2.2-0.2 6 6 6 6.2 0 14.3 0.1 14.3 0.1z',
  },
  {
    d: 'm171 311.6c0 0-23.5-8.7-36-8.7-12.5 0-52 0-52 0-13.3 0-24 9.3-24 20.8 0 0-6 69.9-6 139.2 0 68.9 6 137.4 6 137.4 0 11.5 10.7 20.8 24 20.8 0 0 27.8 0 52.2 0 15.6 0 35.8-12.1 35.8-12.1z',
  },
  {
    d: 'm1023 462v-158.9c0 0-13 0-13-24.2 0-24.2 3.1-31.9 14-31.9 10.9 0 21.3 1.7 23 12 1.7 10.2 5 34.8 5 34.8 16.2-9.2 84-9 84-9 15.5 0 28 12.5 28 27.9v149.5 149.8c0 15.5-12.5 28-28 28 0 0-67.8 0.3-84-9 0 0-3.3 24.7-5 35-1.7 10.3-12.1 12-23 12-10.9 0-14-7.7-14-32 0-24.3 13-24.3 13-24.3v-159.5',
  },
  {
    evenOdd: true,
    d: 'm212.2 129.3c-4.3 6.4 1 22.6 1 22.6 0 0 1.9 20.7 5 27.6 3.1 6.9 4.5 15.8 3 22.7-1.5 6.8-2.3 7.4 5 12.8 7.2 5.3 33.9 6.8 49 5.9 15-1 15.8 2.8 24 7.9 8.1 5 64.3 49.7 178.9 54.2 169.7 6.5 272.8-77.4 289-81.8 16.1-4.4 72.8-14.3 99.9-17.7 27.1-3.5 52.3-4.3 85-26.6 21.4-14.7 22.5-37.3 16-58.2-6.5-20.9-3.6-26.6-5-31.5-1.4-4.9-0.8-6.8-9-7.9-8.2-1.1-52.4-2.5-58-2.9-5.6-0.5-7 2.4-7 11.8 0 8.8 5.9 80-65 81.8-68 1.6-67.9-63-67.9-74.9 0-12 1.4-18.7-7-18.7-8.5 0-317.4 0-324 0-6.5 0-6 3.6-6 11.8 0 8.1 5.2 83.7-67.9 83.7-49.1 0-67-42.2-67-67 0-8.8-1.9-10.8-8-10.8-6.2 0-29.2 4.8-35 13.8-5.9 9-21.6 30.3-29 41.4z',
  },
  {
    evenOdd: true,
    d: 'm212 797c-4.3-6.6 1-23 1-23 0 0 1.9-21 5-28 3.1-7 4.5-16 3-23-1.5-7-2.3-7.6 5-13 7.3-5.4 34-7 49-6 15 1 15.8-2.9 24-8 8.2-5.1 64.3-50.5 179-55 169.8-6.7 272.9 78.5 289 83 16.1 4.5 72.9 14.5 100 18 27.1 3.5 52.3 4.3 85 27 21.4 14.8 22.5 37.8 16 59-6.5 21.2-3.6 27-5 32-1.4 5-0.8 6.9-9 8-8.2 1.1-52.4 2.5-58 3-5.6 0.5-7-2.5-7-12 0-8.9 5.9-81.2-65-83-68-1.7-68 63.9-68 76 0 12.1 1.4 19-7 19-8.4 0-317.5 0-324 0-6.5 0-6-3.7-6-12 0-8.3 5.2-85-68-85-49.1 0-67 42.8-67 68 0 8.9-1.9 11-8 11-6.1 0-29.2-4.9-35-14-5.8-9.1-21.6-30.8-29-42z',
  },
];

/** Draw order: windows on top (door windows sit over the door panels). */
export const VEHICLE_RENDER_ORDER: readonly VehiclePart[] = (['bumpers', 'fenders', 'body', 'doors', 'windows'] as const).flatMap(
  (c) => VEHICLE_PARTS.filter((p) => p.category === c),
);

const BY_KEY = new Map(VEHICLE_PARTS.map((p) => [p.key, p]));
const BY_LEGACY = new Map(VEHICLE_PARTS.map((p) => [p.legacyId, p.key]));

/** Backend key of a raw key (legacy ids included); null when unknown. */
export function canonicalPartKey(raw: string): string | null {
  const k = raw.trim();
  if (!k) return null;
  if (BY_KEY.has(k)) return k;
  return BY_LEGACY.get(k) ?? null;
}

/**
 * Splits a key list into diagram parts (`known`: unique, canonical) and the rest
 * (`unknown`: as given). An unknown key never throws.
 */
export function splitPartKeys(keys: readonly string[] | null | undefined): { known: string[]; unknown: string[] } {
  const known = new Set<string>();
  const unknown = new Set<string>();
  for (const raw of keys ?? []) {
    if (typeof raw !== 'string') continue;
    const k = canonicalPartKey(raw);
    if (k) known.add(k);
    else if (raw.trim()) unknown.add(raw.trim());
  }
  return { known: VEHICLE_PARTS.filter((p) => known.has(p.key)).map((p) => p.key), unknown: [...unknown] };
}

/** Selectable part keys; all of them without `allowed`. */
export function selectablePartKeys(allowed?: readonly string[] | null): Set<string> {
  if (!allowed) return new Set(VEHICLE_PARTS.map((p) => p.key));
  return new Set(splitPartKeys(allowed).known);
}

export type PresetGroup = 'body' | 'windows';

export type VehiclePartPreset = { id: string; group: PresetGroup; keys: readonly string[] };

const BODY_KEYS = VEHICLE_PARTS.filter((p) => p.category !== 'windows').map((p) => p.key);
const WINDOW_KEYS = VEHICLE_PARTS.filter((p) => p.category === 'windows').map((p) => p.key);
const GROUP_KEYS: Record<PresetGroup, readonly string[]> = { body: BODY_KEYS, windows: WINDOW_KEYS };

/** Quick-select presets (same content as the legacy app's `vehicle-part-presets`). */
export const VEHICLE_PART_PRESETS: readonly VehiclePartPreset[] = [
  { id: 'body_hood', group: 'body', keys: ['body_kaput'] },
  { id: 'body_front_three', group: 'body', keys: ['body_kaput', 'body_sol_on_camurluk', 'body_sag_on_camurluk'] },
  {
    id: 'body_front_four',
    group: 'body',
    keys: ['body_kaput', 'body_on_tampon', 'body_sol_on_camurluk', 'body_sag_on_camurluk'],
  },
  { id: 'body_full', group: 'body', keys: BODY_KEYS },
  { id: 'body_full_except_roof', group: 'body', keys: BODY_KEYS.filter((k) => k !== 'body_tavan') },
  {
    id: 'body_full_except_bumpers',
    group: 'body',
    keys: BODY_KEYS.filter((k) => k !== 'body_on_tampon' && k !== 'body_arka_tampon'),
  },
  { id: 'windows_front', group: 'windows', keys: ['window_on_cam'] },
  { id: 'windows_front_two', group: 'windows', keys: ['window_sol_on_kapi', 'window_sag_on_kapi'] },
  { id: 'windows_except_front', group: 'windows', keys: WINDOW_KEYS.filter((k) => k !== 'window_on_cam') },
  { id: 'windows_all', group: 'windows', keys: WINDOW_KEYS },
];

function presetKeys(preset: VehiclePartPreset, selectable: ReadonlySet<string>): string[] {
  return preset.keys.filter((k) => selectable.has(k));
}

/** A preset is active when the selection in its group equals it exactly. */
export function isPresetActive(
  selected: ReadonlySet<string>,
  preset: VehiclePartPreset,
  selectable: ReadonlySet<string>,
): boolean {
  const want = presetKeys(preset, selectable);
  if (!want.length) return false;
  const inGroup = GROUP_KEYS[preset.group].filter((k) => selected.has(k));
  return inGroup.length === want.length && want.every((k) => selected.has(k));
}

/** Applies a preset: replaces the selection of its group, keeps the other group; pressing an active preset clears its group. */
export function applyPreset(
  selected: ReadonlySet<string>,
  preset: VehiclePartPreset,
  selectable: ReadonlySet<string>,
): Set<string> {
  const want = presetKeys(preset, selectable);
  if (!want.length) return new Set(selected);
  const group = new Set(GROUP_KEYS[preset.group]);
  const next = new Set([...selected].filter((k) => !group.has(k)));
  if (isPresetActive(selected, preset, selectable)) return next;
  for (const k of want) next.add(k);
  return next;
}

/** Whether the preset has a selectable part under this restriction (else its button is hidden). */
export function presetAvailable(preset: VehiclePartPreset, selectable: ReadonlySet<string>): boolean {
  return presetKeys(preset, selectable).length > 0;
}

/** Keys in the fixed diagram order. */
export function orderedKeys(keys: Iterable<string>): string[] {
  const set = new Set(keys);
  return VEHICLE_PARTS.filter((p) => set.has(p.key)).map((p) => p.key);
}

const rgb = (hex: string) => {
  const m = /^#([0-9a-f]{6})$/i.exec(hex);
  if (!m) return null;
  const n = parseInt(m[1], 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
};

/** `a` over `b` at `amount` (0..1). A color that is not #rrggbb comes back as `a`. */
export function mixHex(a: string, b: string, amount: number) {
  const x = rgb(a);
  const y = rgb(b);
  if (!x || !y) return a;
  const hex = x.map((c, i) => Math.round(c * amount + y[i] * (1 - amount)).toString(16).padStart(2, '0'));
  return `#${hex.join('')}`;
}

/** Relative luminance (WCAG) of #rrggbb; a color it cannot read counts as light. */
function luminance(hex: string) {
  const c = rgb(hex);
  if (!c) return 1;
  const [r, g, b] = c.map((v) => {
    const u = v / 255;
    return u <= 0.03928 ? u / 12.92 : ((u + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/**
 * Diagram colors from the theme tokens. Light: solid brand fills on pale panels.
 * Dark: a full-strength brand fill glares on a dark card, so a selected panel is the brand
 * mixed into the card (glass less than body) and only its outline keeps the full color.
 */
export function schemeColors(t: { muted: string; border: string; mutedForeground: string; primary: string; success: string; card: string }) {
  if (luminance(t.card) >= 0.2) {
    return { dark: false, body: t.muted, window: t.border, stroke: t.mutedForeground, strokeOn: t.mutedForeground, bodyOn: t.primary, windowOn: t.success };
  }
  return {
    dark: true,
    body: t.muted,
    window: t.border,
    stroke: mixHex(t.mutedForeground, t.card, 0.55),
    strokeOn: t.primary,
    bodyOn: mixHex(t.primary, t.card, 0.45),
    windowOn: mixHex(t.primary, t.card, 0.28),
  };
}
