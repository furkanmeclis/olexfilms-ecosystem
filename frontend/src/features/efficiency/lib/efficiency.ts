import type {
  EfficiencyDimension,
  EfficiencyPeriod,
  EfficiencyTrendRow,
  ExpectationInput,
  PartConsumptionExpectation,
} from "@/features/efficiency/services/efficiency.service";

/** Sysconfig default of `efficiency.warning_waste_ratio`. */
export const DEFAULT_WARNING_WASTE_RATIO = 0.15;

export const EFFICIENCY_PERIOD_DAYS = [30, 90, 180, 365] as const;
export type EfficiencyPeriodDays = (typeof EFFICIENCY_PERIOD_DAYS)[number];

const ALL_DIMENSIONS: EfficiencyDimension[] = [
  "dealer",
  "staff",
  "product",
  "body_type",
  "part",
];

/**
 * Breakdown tabs of an organization type. A dealer has no dealer breakdown
 * (it is the only dealer) and the API refuses its staff breakdown (TEC-488).
 */
export function efficiencyDimensions(
  orgType: string | null | undefined,
): EfficiencyDimension[] {
  if (orgType === "dealer") {
    return ALL_DIMENSIONS.filter((d) => d !== "dealer" && d !== "staff");
  }
  return ALL_DIMENSIONS;
}

function isoDay(date: Date) {
  return date.toISOString().slice(0, 10);
}

/** The last `days` days up to today (UTC). */
export function efficiencyPeriod(
  days: number,
  now: Date = new Date(),
): EfficiencyPeriod {
  const from = new Date(now.getTime() - days * 24 * 60 * 60 * 1000);
  return { period_from: isoDay(from), period_to: isoDay(now) };
}

/** Decimal string / number → number; null when missing or not numeric. */
export function ratioValue(value: unknown): number | null {
  if (value == null || value === "") return null;
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
}

export function metersValue(value: unknown): number {
  return ratioValue(value) ?? 0;
}

/** A waste ratio as a locale percentage; "—" without an expectation. */
export function formatWaste(
  value: unknown,
  formatPercent: (ratio: number, digits?: number) => string,
): string {
  const ratio = ratioValue(value);
  if (ratio == null) return "—";
  return formatPercent(ratio, 1);
}

/** Rows above the warning threshold are highlighted. */
export function isAboveWarning(value: unknown, threshold: number): boolean {
  const ratio = ratioValue(value);
  return ratio != null && ratio > threshold;
}

export const WARNING_ROW_CLASS = "bg-destructive/5 hover:bg-destructive/10";

export function warningRowClassName(
  value: unknown,
  threshold: number,
): string | undefined {
  return isAboveWarning(value, threshold) ? WARNING_ROW_CLASS : undefined;
}

/**
 * Percent filter value (e.g. 15) → API ratio param (0.15); the waste ratio
 * columns filter in percent.
 */
export function percentRangeParams(param: string) {
  return (value: unknown): Record<string, string | undefined> => {
    const [min, max] = Array.isArray(value) ? value : [];
    const toRatio = (v: unknown) => {
      const n = ratioValue(v);
      return n == null ? undefined : String(n / 100);
    };
    return { [`${param}_min`]: toRatio(min), [`${param}_max`]: toRatio(max) };
  };
}

/** Period totals for the KPI cards from the monthly trend rows. */
export function trendTotals(rows: EfficiencyTrendRow[]) {
  let services = 0;
  let meters = 0;
  let weighted = 0;
  let weight = 0;
  for (const row of rows) {
    services += row.services;
    meters += metersValue(row.meters);
    const ratio = ratioValue(row.waste_ratio);
    if (ratio != null) {
      weighted += ratio * Math.max(row.services, 1);
      weight += Math.max(row.services, 1);
    }
  }
  return {
    services,
    meters,
    wasteRatio: weight > 0 ? weighted / weight : null,
  };
}

export function expectationInput(
  row: PartConsumptionExpectation,
): ExpectationInput {
  return {
    product_uuid: row.product_uuid,
    category_uuid: row.category_uuid,
    body_type: row.body_type,
    part_key: row.part_key,
    expected_meters: row.expected_meters,
  };
}

/**
 * Inline edit of an expectation row. Any edit makes the row a manual
 * definition (the network refresh no longer overwrites it), as the API does.
 * Returns null when the value is not valid for the column.
 */
export function applyExpectationEdit(
  row: PartConsumptionExpectation,
  columnId: string,
  value: unknown,
): PartConsumptionExpectation | null {
  if (columnId === "expected_meters") {
    const n = ratioValue(String(value ?? "").replace(",", "."));
    if (n == null || n <= 0) return null;
    return {
      ...row,
      expected_meters: n.toFixed(2),
      source: "manual",
      sample_size: 1,
    };
  }
  if (columnId === "body_type") {
    const body = String(value ?? "").trim();
    return {
      ...row,
      body_type: body || null,
      source: "manual",
      sample_size: 1,
    };
  }
  return null;
}

function csvCell(value: unknown) {
  const text = value == null ? "" : String(value);
  return /[",\n\r]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
}

/**
 * CSV of expectation rows. The headers match the import columns, so an
 * edited file imports back (rows with a uuid update that definition).
 */
export function expectationsCsv(rows: PartConsumptionExpectation[]): string {
  const header = [
    "uuid",
    "product",
    "category",
    "body_type",
    "part_key",
    "expected_meters",
    "source",
    "sample_size",
  ];
  const lines = rows.map((row) =>
    [
      row.uuid,
      row.product_name,
      row.category_name,
      row.body_type,
      row.part_key,
      row.expected_meters,
      row.source,
      row.sample_size,
    ]
      .map(csvCell)
      .join(","),
  );
  return [header.join(","), ...lines].join("\n") + "\n";
}
