import {
  numberValue,
  PERFORMANCE_METRICS,
} from "@/features/performance/lib/performance";
import type {
  PerformanceBonusApprovalInput,
  PerformanceBonusRuleInput,
  PerformanceMember,
  PerformanceStaffTarget,
  TargetOrganization,
} from "@/features/performance/services/performance.service";

/** Staff target / bonus rule metrics inside a dealer. */
export const STAFF_METRICS = ["services_count", "service_revenue"] as const;
export type StaffMetric = (typeof STAFF_METRICS)[number];

export const BONUS_KINDS = ["fixed", "percent_of_revenue"] as const;
export type BonusKind = (typeof BONUS_KINDS)[number];

export const BONUS_STATUSES = [
  "calculated",
  "approved",
  "posted",
  "cancelled",
] as const;

export const RULE_OPERATORS = [
  "lt",
  "lte",
  "gt",
  "gte",
  "below_median_pct",
] as const;
export type RuleOperator = (typeof RULE_OPERATORS)[number];

/** Weak dealer rule metrics: target achievement + the monthly metrics. */
export const RULE_METRICS = [
  "target_achievement",
  ...PERFORMANCE_METRICS,
] as const;

/** Months of the team targets grid around the picked month. */
export const TEAM_MONTHS_BEFORE = 2;
export const TEAM_MONTHS_AFTER = 3;

/**
 * Only center rules open tasks (tasks are a center module, QUESTIONS S19);
 * a distributor rule only notifies.
 */
export function canOpenTask(orgType: string | null | undefined) {
  return orgType === "center";
}

/**
 * Organizations a target may be set for: the center sets one for any
 * distributor or dealer of the brand, a distributor only for its own
 * dealers (backend validTargetOrg).
 */
export function targetOrganizationOptions(
  orgs: readonly TargetOrganization[],
  orgType: string | null | undefined,
  activeOrgUuid: string | null | undefined,
) {
  return orgs.filter((org) => {
    if (org.uuid === activeOrgUuid) return false;
    if (orgType === "center") {
      return org.type === "distributor" || org.type === "dealer";
    }
    if (orgType === "distributor") {
      return org.type === "dealer" && org.parent?.uuid === activeOrgUuid;
    }
    return false;
  });
}

export type BonusApprovalResult =
  | { body: PerformanceBonusApprovalInput }
  | { error: "amount_invalid" | "note_required" };

/**
 * Approval body of a bonus: an unchanged (or empty) amount approves the
 * calculated amount; a changed amount needs a note (backend
 * "note is required when amount is adjusted").
 */
export function bonusApproval(
  calculated: string,
  amount: string,
  note: string,
): BonusApprovalResult {
  const trimmedNote = note.trim();
  const raw = amount.trim();
  const original = numberValue(calculated);
  const next = numberValue(raw);
  if (raw === "" || (next != null && original != null && next === original)) {
    return { body: trimmedNote ? { note: trimmedNote } : {} };
  }
  if (next == null || next <= 0) return { error: "amount_invalid" };
  if (!trimmedNote) return { error: "note_required" };
  return { body: { amount: raw, note: trimmedNote } };
}

export type BonusRuleForm = {
  name: string;
  metric: StaffMetric;
  threshold_pct: string;
  kind: BonusKind;
  amount: string;
  percent: string;
  active: boolean;
};

/** Validated bonus rule body, or null when a required field is missing. */
export function bonusRuleInput(
  form: BonusRuleForm,
): PerformanceBonusRuleInput | null {
  const threshold = numberValue(form.threshold_pct);
  if (!form.name.trim() || threshold == null || threshold <= 0) return null;
  if (threshold > 1000) return null;
  if (form.kind === "fixed") {
    const amount = numberValue(form.amount);
    if (amount == null || amount <= 0) return null;
    return {
      name: form.name.trim(),
      metric: form.metric,
      threshold_pct: form.threshold_pct.trim(),
      kind: "fixed",
      amount: form.amount.trim(),
      active: form.active,
    };
  }
  const percent = numberValue(form.percent);
  if (percent == null || percent <= 0 || percent > 100) return null;
  return {
    name: form.name.trim(),
    metric: form.metric,
    threshold_pct: form.threshold_pct.trim(),
    kind: "percent_of_revenue",
    percent: form.percent.trim(),
    active: form.active,
  };
}

/** `YYYY-MM` shifted by n months. */
export function shiftMonth(period: string, n: number) {
  const [y, m] = period.split("-").map(Number);
  const d = new Date(Date.UTC(y, m - 1 + n, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

/** Months of the team targets grid, oldest first. */
export function teamMonths(period: string) {
  const out: string[] = [];
  for (let i = -TEAM_MONTHS_BEFORE; i <= TEAM_MONTHS_AFTER; i++) {
    out.push(shiftMonth(period, i));
  }
  return out;
}

export type TeamRow = {
  uuid: string;
  name: string;
  /** Target of the metric per month. */
  months: Record<string, PerformanceStaffTarget | undefined>;
};

/**
 * Staff × month rows of one metric: every member, plus users with targets
 * who are no longer listed as members.
 */
export function teamRows(
  members: readonly PerformanceMember[],
  targets: readonly PerformanceStaffTarget[],
  metric: StaffMetric,
): TeamRow[] {
  const rows = new Map<string, TeamRow>();
  for (const m of members) {
    if (!m.uuid) continue;
    rows.set(m.uuid, { uuid: m.uuid, name: m.name ?? "", months: {} });
  }
  for (const target of targets) {
    if (target.metric !== metric || !target.user_uuid || !target.period) {
      continue;
    }
    let row = rows.get(target.user_uuid);
    if (!row) {
      row = {
        uuid: target.user_uuid,
        name: target.user_name ?? "",
        months: {},
      };
      rows.set(target.user_uuid, row);
    }
    row.months[target.period] = target;
  }
  return [...rows.values()];
}

/** Column id of a month cell of the team grid. */
export function monthColumnId(period: string) {
  return `m_${period}`;
}

export function monthOfColumn(columnId: string) {
  return columnId.startsWith("m_") ? columnId.slice(2) : null;
}

/** Progress bar width (0–100) of an achievement percentage. */
export function achievementWidth(pct: unknown) {
  const n = numberValue(pct);
  if (n == null || n <= 0) return 0;
  return Math.min(100, n);
}
