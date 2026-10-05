import type {
  SystemSetting,
  SystemSettingValue,
} from "@/features/system-settings/services/system-settings.service";

/**
 * Warranty claim labor rule (TEC-337 settings, TEC-340 UI): who carries the
 * labor of a warranty re-application. `shared` splits it by a percentage.
 * The keys only exist once the backend catalog ships them; until then the
 * card stays hidden and the hub is unchanged.
 */
export const LABOR_RULE_KEY = "warranty_claims.labor_rule";
export const LABOR_AMOUNT_KEY = "warranty_claims.labor_amount";
export const LABOR_PERCENT_KEY = "warranty_claims.labor_share_percent";
export const LABOR_KEYS = [
  LABOR_RULE_KEY,
  LABOR_AMOUNT_KEY,
  LABOR_PERCENT_KEY,
] as const;

export const LABOR_RULES = ["dealer", "center", "shared"] as const;
export type LaborRule = (typeof LABOR_RULES)[number];

export function isLaborRule(value: unknown): value is LaborRule {
  return (LABOR_RULES as readonly unknown[]).includes(value);
}

export type LaborSettings = {
  rule: SystemSetting;
  amount?: SystemSetting;
  percent?: SystemSetting;
};

/** Splits the catalog: labor keys go to the card, the rest stays generic. */
export function splitLaborSettings(items: readonly SystemSetting[]): {
  labor: LaborSettings | null;
  rest: SystemSetting[];
} {
  const byKey = new Map(items.map((s) => [s.key, s] as const));
  const rule = byKey.get(LABOR_RULE_KEY);
  if (!rule) return { labor: null, rest: [...items] };
  const keys = new Set<string>(LABOR_KEYS);
  return {
    labor: {
      rule,
      amount: byKey.get(LABOR_AMOUNT_KEY),
      percent: byKey.get(LABOR_PERCENT_KEY),
    },
    rest: items.filter((s) => !keys.has(s.key)),
  };
}

export type LaborDraft = { rule: LaborRule; amount: string; percent: string };

function text(value: SystemSettingValue | undefined) {
  return value === undefined || value === null ? "" : String(value);
}

export function toLaborDraft(labor: LaborSettings): LaborDraft {
  return {
    rule: isLaborRule(labor.rule.value) ? labor.rule.value : "dealer",
    amount: text(labor.amount?.value),
    percent: text(labor.percent?.value),
  };
}

export type LaborErrors = {
  amount?: "number" | "min";
  percent?: "integer" | "range";
};

const DECIMAL = /^\d+(?:[.,]\d{1,2})?$/;

/** Mirrors the expected catalog checks; the server stays authoritative. */
export function validateLaborDraft(
  draft: LaborDraft,
  labor: LaborSettings,
): LaborErrors {
  const errors: LaborErrors = {};
  const amount = draft.amount.trim();
  if (labor.amount && amount !== "") {
    if (amount.startsWith("-")) errors.amount = "min";
    else if (
      labor.amount.kind === "int"
        ? !/^\d+$/.test(amount)
        : !DECIMAL.test(amount)
    )
      errors.amount = "number";
  }
  if (draft.rule === "shared" && labor.percent) {
    const percent = draft.percent.trim();
    if (!/^-?\d+$/.test(percent)) errors.percent = "integer";
    else {
      const n = Number(percent);
      if (n < 0 || n > 100) errors.percent = "range";
    }
  }
  return errors;
}

export function hasLaborErrors(errors: LaborErrors) {
  return errors.amount !== undefined || errors.percent !== undefined;
}

/** Typed value for a key: ints as numbers, everything else as text. */
function toValue(setting: SystemSetting, raw: string): SystemSettingValue {
  const trimmed = raw.trim();
  if (setting.kind === "int") return Number(trimmed);
  return trimmed.replace(",", ".");
}

/**
 * The PUTs the save needs: only changed keys, and the percentage only while
 * the rule is `shared` (otherwise it is irrelevant and kept as stored).
 */
export function laborChanges(
  draft: LaborDraft,
  labor: LaborSettings,
): { key: string; value: SystemSettingValue }[] {
  const initial = toLaborDraft(labor);
  const out: { key: string; value: SystemSettingValue }[] = [];
  if (draft.rule !== initial.rule) {
    out.push({ key: labor.rule.key, value: draft.rule });
  }
  if (
    labor.amount &&
    draft.amount.trim() !== "" &&
    draft.amount.trim() !== initial.amount
  ) {
    out.push({
      key: labor.amount.key,
      value: toValue(labor.amount, draft.amount),
    });
  }
  if (
    draft.rule === "shared" &&
    labor.percent &&
    draft.percent.trim() !== initial.percent
  ) {
    out.push({
      key: labor.percent.key,
      value: toValue(labor.percent, draft.percent),
    });
  }
  return out;
}
