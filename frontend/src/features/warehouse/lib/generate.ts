import {
  WAREHOUSE_CODE_RE,
  normalizeCode,
} from "@/features/warehouse/lib/forms";
import {
  allowedChildTypes,
  type LocationNode,
} from "@/features/warehouse/lib/tree";
import type {
  WarehouseGenerateLevel,
  WarehouseLocationType,
} from "@/features/warehouse/services/warehouse.service";

/** Backend cap of one generate request (usecase MaxGenerated). */
export const GENERATE_MAX = 2000;
/** Above this many new locations the wizard asks for a second confirmation. */
export const GENERATE_CONFIRM_THRESHOLD = 500;
/** Range bounds of the backend (0 <= from <= to <= 9999, pad 0..4). */
export const RANGE_MAX = 9999;
export const PAD_MAX = 4;
/** Leaf codes shown in the preview (first ones + the last one). */
export const SAMPLE_SIZE = 6;

/**
 * One level of the bulk generation wizard as typed in the form: either a
 * numeric range rendered as prefix + zero-padded number (A01..A10) or a
 * list of codes. Numbers stay strings until validated.
 */
export type LevelDraft = {
  type: WarehouseLocationType;
  mode: "range" | "list";
  prefix: string;
  from: string;
  to: string;
  pad: string;
  codes: string;
};

export type LevelError =
  | { key: "codes_required" }
  | { key: "range" }
  | { key: "pad" }
  | { key: "code_invalid"; code: string }
  | { key: "duplicate"; code: string }
  | { key: "type" };

export type LevelResult =
  { ok: true; codes: string[] } | { ok: false; error: LevelError };

const INT_RE = /^\d+$/;

/** Splits the list field on commas, semicolons, whitespace and new lines. */
export function splitCodes(raw: string): string[] {
  return raw
    .split(/[\s,;]+/)
    .map((c) => c.trim())
    .filter(Boolean);
}

/**
 * Expands one level into its codes the way the backend does
 * (usecase levelCodes): `prefix + %0<pad>d` over from..to, or the list;
 * every code upper-cased, A-Z 0-9 _ (1-32), no repeats.
 */
export function levelCodes(lv: LevelDraft): LevelResult {
  let raw: string[];
  if (lv.mode === "list") {
    raw = splitCodes(lv.codes);
    if (raw.length === 0)
      return { ok: false, error: { key: "codes_required" } };
  } else {
    if (![lv.from, lv.to].every((v) => INT_RE.test(v.trim()))) {
      return { ok: false, error: { key: "range" } };
    }
    const from = Number(lv.from);
    const to = Number(lv.to);
    if (from > to || to > RANGE_MAX) {
      return { ok: false, error: { key: "range" } };
    }
    const padRaw = lv.pad.trim() === "" ? "0" : lv.pad.trim();
    if (!INT_RE.test(padRaw) || Number(padRaw) > PAD_MAX) {
      return { ok: false, error: { key: "pad" } };
    }
    if (to - from + 1 > GENERATE_MAX) {
      return { ok: false, error: { key: "range" } };
    }
    const pad = Number(padRaw);
    raw = [];
    for (let n = from; n <= to; n++) {
      raw.push(`${lv.prefix.trim()}${String(n).padStart(pad, "0")}`);
    }
  }
  const seen = new Set<string>();
  const codes: string[] = [];
  for (const r of raw) {
    const code = normalizeCode(r);
    if (!WAREHOUSE_CODE_RE.test(code)) {
      return { ok: false, error: { key: "code_invalid", code } };
    }
    if (seen.has(code)) return { ok: false, error: { key: "duplicate", code } };
    seen.add(code);
    codes.push(code);
  }
  return { ok: true, codes };
}

/** The request level of a draft (validated codes; ranges stay ranges). */
export function toGenerateLevel(lv: LevelDraft): WarehouseGenerateLevel {
  if (lv.mode === "list") {
    return { type: lv.type, codes: splitCodes(lv.codes).map(normalizeCode) };
  }
  return {
    type: lv.type,
    from: Number(lv.from),
    to: Number(lv.to),
    pad: lv.pad.trim() === "" ? 0 : Number(lv.pad),
    prefix: normalizeCode(lv.prefix),
  };
}

/** A fresh level of `type` with sensible defaults. */
export function newLevel(type: WarehouseLocationType): LevelDraft {
  if (type === "aisle") {
    return {
      type,
      mode: "list",
      prefix: "",
      from: "1",
      to: "2",
      pad: "0",
      codes: "A, B",
    };
  }
  return {
    type,
    mode: "range",
    prefix: "",
    from: "1",
    to: type === "shelf" ? "5" : "4",
    pad: "2",
    codes: "",
  };
}

/**
 * The default chain under a parent: room root → aisle › shelf › bin,
 * aisle → shelf › bin, shelf → bin.
 */
export function defaultLevels(
  parentType: WarehouseLocationType | null,
): LevelDraft[] {
  const out: LevelDraft[] = [];
  let prev = parentType;
  for (;;) {
    const next = allowedChildTypes(prev)[0];
    if (!next) break;
    out.push(newLevel(next));
    prev = next;
  }
  return out;
}

/**
 * Cuts the chain at the first level whose type does not fit under the
 * previous one (after the user changes a level's type).
 */
export function fixLevelChain(
  parentType: WarehouseLocationType | null,
  levels: LevelDraft[],
): LevelDraft[] {
  const out: LevelDraft[] = [];
  let prev = parentType;
  for (const lv of levels) {
    if (!allowedChildTypes(prev).includes(lv.type)) break;
    out.push(lv);
    prev = lv.type;
  }
  return out;
}

export type GenerationConflict = {
  full_code: string;
  existing: WarehouseLocationType;
  wanted: WarehouseLocationType;
};

export type GenerationPlan = {
  /** Per-level validation; the preview needs every level ok. */
  levelErrors: (LevelError | null)[];
  /** Codes per level (empty for an invalid level). */
  levelCounts: number[];
  /** Every node the request touches (new + reused), like the backend. */
  total: number;
  /** Nodes of the last level (e.g. the bins). */
  leaves: number;
  /** New locations / existing ones the backend reuses. */
  created: number;
  existing: number;
  /** Same code under the same parent with another type: the backend refuses. */
  conflicts: GenerationConflict[];
  /** First leaf full codes and the last one. */
  sample: string[];
  tooMany: boolean;
  needsExtraConfirm: boolean;
  ok: boolean;
};

/**
 * Preview of a generation under `parent` (null = the room root): code
 * counts, a sample of the resulting full codes and how the request meets
 * the existing tree `roots` of the room (reused vs new vs type conflict).
 * `baseCode` is the parent's full_code (or WAREHOUSE-ROOM at the root).
 */
export function planGeneration({
  levels,
  parent,
  baseCode,
  roots,
}: {
  levels: LevelDraft[];
  parent: LocationNode | null;
  baseCode: string;
  roots: LocationNode[];
}): GenerationPlan {
  const results = levels.map(levelCodes);
  const levelErrors: (LevelError | null)[] = results.map((r) =>
    r.ok ? null : r.error,
  );
  const chainOk =
    levels.length > 0 &&
    fixLevelChain(parent?.type ?? null, levels).length === levels.length;
  if (levels.length > 0 && !chainOk) {
    levelErrors[levelErrors.length - 1] ??= { key: "type" };
  }
  const codes = results.map((r) => (r.ok ? r.codes : []));
  const levelCounts = codes.map((c) => c.length);

  let total = 0;
  let width = 1;
  for (const n of levelCounts) {
    width *= n;
    total += width;
  }
  const leaves = levels.length > 0 ? width : 0;
  const valid = chainOk && levelErrors.every((e) => e === null);
  const tooMany = total > GENERATE_MAX;

  const plan: GenerationPlan = {
    levelErrors,
    levelCounts,
    total: valid ? total : 0,
    leaves: valid ? leaves : 0,
    created: 0,
    existing: 0,
    conflicts: [],
    sample: [],
    tooMany: valid && tooMany,
    needsExtraConfirm: false,
    ok: false,
  };
  if (!valid || tooMany) return plan;

  plan.sample = sampleLeaves(baseCode, codes);

  // Walk the request over the existing tree: a code that already sits under
  // the same parent with the same type is reused (as the backend does).
  const walk = (
    level: number,
    children: LocationNode[] | null,
    prefix: string,
  ) => {
    const byCode = new Map((children ?? []).map((c) => [c.code, c]));
    for (const code of codes[level]) {
      const fullCode = `${prefix}-${code}`;
      const hit = byCode.get(code);
      if (hit && hit.type !== levels[level].type) {
        plan.conflicts.push({
          full_code: hit.full_code,
          existing: hit.type,
          wanted: levels[level].type,
        });
        continue;
      }
      if (hit) plan.existing += 1;
      else plan.created += 1;
      if (level + 1 < codes.length) {
        walk(level + 1, hit ? hit.children : null, fullCode);
      }
    }
  };
  walk(0, parent ? parent.children : roots, baseCode);

  plan.needsExtraConfirm = plan.created > GENERATE_CONFIRM_THRESHOLD;
  plan.ok = plan.conflicts.length === 0 && plan.created + plan.existing > 0;
  return plan;
}

/** The first SAMPLE_SIZE leaf full codes and, when more, the last one. */
function sampleLeaves(baseCode: string, codes: string[][]): string[] {
  const count = codes.reduce((n, c) => n * c.length, 1);
  if (codes.length === 0 || count === 0) return [];
  const at = (index: number) => {
    const parts: string[] = [];
    let rest = index;
    for (let i = codes.length - 1; i >= 0; i--) {
      parts.unshift(codes[i][rest % codes[i].length]);
      rest = Math.floor(rest / codes[i].length);
    }
    return [baseCode, ...parts].join("-");
  };
  const out: string[] = [];
  for (let i = 0; i < Math.min(SAMPLE_SIZE, count); i++) out.push(at(i));
  if (count > SAMPLE_SIZE) out.push(at(count - 1));
  return out;
}
