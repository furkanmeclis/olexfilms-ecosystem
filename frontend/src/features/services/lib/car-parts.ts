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

/** Legacy quick picks; each one changes only its own group. */
export const PART_PRESETS = [
  {
    id: "front_three",
    group: "body",
    parts: ["body_sag_on_camurluk", "body_kaput", "body_sol_on_camurluk"],
  },
  {
    id: "front_four",
    group: "body",
    parts: [
      "body_sag_on_camurluk",
      "body_kaput",
      "body_on_tampon",
      "body_sol_on_camurluk",
    ],
  },
  { id: "hood_only", group: "body", parts: ["body_kaput"] },
  { id: "all_windows", group: "window", parts: [...WINDOW_PARTS] },
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

/** A preset is active when its group holds exactly its (available) parts. */
export function isPresetActive(
  selected: readonly string[],
  preset: PartPreset,
  available: readonly string[],
): boolean {
  const parts = presetParts(preset, available);
  if (parts.length === 0) return false;
  const inGroup = selected.filter((p) => partGroup(p) === preset.group);
  return (
    inGroup.length === parts.length && parts.every((p) => inGroup.includes(p))
  );
}

/**
 * Applies a preset: replaces the selection of its group with the preset's
 * parts (the other group is kept); applying an active preset clears it.
 */
export function applyPreset(
  selected: readonly string[],
  preset: PartPreset,
  available: readonly string[],
): string[] {
  const others = selected.filter((p) => partGroup(p) !== preset.group);
  if (isPresetActive(selected, preset, available)) return others;
  return [...others, ...presetParts(preset, available)];
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
