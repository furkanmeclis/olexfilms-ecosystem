/**
 * Vehicle part keys (TEC-182; legacy CarPartEnum, 13 body + 7 window).
 * Product categories list the keys a service may apply in
 * `available_parts` (TEC-145); service items carry the applied subset
 * (TEC-179 `applied_parts`, checked against the item's category).
 */
export const BODY_PARTS = [
  "body_kaput",
  "body_tavan",
  "body_bagaj",
  "body_on_tampon",
  "body_arka_tampon",
  "body_sol_on_camurluk",
  "body_sol_on_kapi",
  "body_sol_arka_kapi",
  "body_sol_arka_camurluk",
  "body_sag_on_camurluk",
  "body_sag_on_kapi",
  "body_sag_arka_kapi",
  "body_sag_arka_camurluk",
] as const;

export const WINDOW_PARTS = [
  "window_on_cam",
  "window_arka_cam",
  "window_sunroof",
  "window_sol_on_kapi",
  "window_sol_arka_kapi",
  "window_sag_on_kapi",
  "window_sag_arka_kapi",
] as const;

export type PartGroup = "body" | "window";

export const PART_GROUPS: Record<PartGroup, readonly string[]> = {
  body: BODY_PARTS,
  window: WINDOW_PARTS,
};

const KNOWN = new Set<string>([...BODY_PARTS, ...WINDOW_PARTS]);

export function isKnownPart(part: string): boolean {
  return KNOWN.has(part);
}

export function partGroup(part: string): PartGroup | null {
  if (part.startsWith("body_")) return "body";
  if (part.startsWith("window_")) return "window";
  return null;
}

const bodyExcept = (...skip: string[]) =>
  BODY_PARTS.filter((p) => !skip.includes(p));
const windowsExcept = (...skip: string[]) =>
  WINDOW_PARTS.filter((p) => !skip.includes(p));

/**
 * One-click selections (TEC-864; the first three follow the legacy quick
 * picks). They add to the selection and combine (TEC-1076); parts a
 * category does not offer are skipped. The mapping is product-owned:
 * change it here.
 */
export const PART_PRESETS = [
  { id: "hood_only", group: "body", parts: ["body_kaput"] },
  {
    id: "front_three",
    group: "body",
    parts: ["body_kaput", "body_sol_on_camurluk", "body_sag_on_camurluk"],
  },
  {
    id: "front_four",
    group: "body",
    parts: [
      "body_kaput",
      "body_on_tampon",
      "body_sol_on_camurluk",
      "body_sag_on_camurluk",
    ],
  },
  { id: "full_body", group: "body", parts: [...BODY_PARTS] },
  {
    id: "full_body_no_bumpers",
    group: "body",
    parts: bodyExcept("body_on_tampon", "body_arka_tampon"),
  },
  {
    id: "full_body_no_roof",
    group: "body",
    parts: bodyExcept("body_tavan"),
  },
  { id: "windshield", group: "window", parts: ["window_on_cam"] },
  { id: "sunroof", group: "window", parts: ["window_sunroof"] },
  {
    id: "front_two_windows",
    group: "window",
    parts: ["window_sol_on_kapi", "window_sag_on_kapi"],
  },
  { id: "all_windows", group: "window", parts: [...WINDOW_PARTS] },
  {
    id: "all_windows_no_windshield",
    group: "window",
    parts: windowsExcept("window_on_cam"),
  },
  {
    id: "all_windows_no_roof",
    group: "window",
    parts: windowsExcept("window_sunroof"),
  },
] as const satisfies readonly {
  id: string;
  group: PartGroup;
  parts: readonly string[];
}[];

export type PartPreset = (typeof PART_PRESETS)[number];

/** Union of the categories' available_parts, in first-seen order. */
export function availablePartsOf(
  categories: readonly { available_parts?: readonly string[] | null }[],
): string[] {
  const out: string[] = [];
  for (const c of categories) {
    for (const p of c.available_parts ?? []) {
      if (!out.includes(p)) out.push(p);
    }
  }
  return out;
}

export function togglePart(selected: readonly string[], part: string) {
  return selected.includes(part)
    ? selected.filter((p) => p !== part)
    : [...selected, part];
}

/** The preset's parts that may be picked. */
export function presetParts(
  preset: PartPreset,
  available: readonly string[],
): string[] {
  return preset.parts.filter((p) => available.includes(p));
}

/**
 * A preset is active when all of its (available) parts are selected. TEC-1076
 * (revize 9.1): presets combine, e.g. "front two windows" + "windshield" are
 * both active at once; a larger selection keeps its sub-presets lit.
 */
export function isPresetActive(
  selected: readonly string[],
  preset: PartPreset,
  available: readonly string[],
): boolean {
  const parts = presetParts(preset, available);
  return parts.length > 0 && parts.every((p) => selected.includes(p));
}

/**
 * Applies a preset on top of the selection (TEC-1076: multi-select): an
 * inactive preset adds its parts, an active one removes them; every other
 * part stays.
 */
export function applyPreset(
  selected: readonly string[],
  preset: PartPreset,
  available: readonly string[],
): string[] {
  const parts = presetParts(preset, available);
  if (isPresetActive(selected, preset, available)) {
    return selected.filter((p) => !parts.includes(p));
  }
  return [...selected, ...parts.filter((p) => !selected.includes(p))];
}

/**
 * The presets worth showing for the pickable parts: those with at least one
 * available part, minus later ones that resolve to the same parts as an
 * earlier one of their group (e.g. "all windows" and "all windows except
 * the roof" when the category has no sunroof).
 */
export function visiblePresets(available: readonly string[]): PartPreset[] {
  const seen = new Set<string>();
  const out: PartPreset[] = [];
  for (const preset of PART_PRESETS) {
    const parts = presetParts(preset, available);
    if (parts.length === 0) continue;
    const key = preset.group + ":" + [...parts].sort().join(",");
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(preset);
  }
  return out;
}

/** Parts of a new item: the selection limited to its category's parts. */
export function partsForItem(
  selected: readonly string[],
  productParts: readonly string[] | null | undefined,
): string[] {
  const allowed = productParts ?? [];
  return selected.filter((p) => allowed.includes(p));
}

/** Union of the parts already applied by the service items. */
export function partsFromItems(
  items: readonly { applied_parts: readonly string[] }[] | null | undefined,
): string[] {
  return availablePartsOf(
    (items ?? []).map((i) => ({ available_parts: i.applied_parts })),
  );
}
