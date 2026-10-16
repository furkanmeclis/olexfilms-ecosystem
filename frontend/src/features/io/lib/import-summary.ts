import type {
  ImportJob,
  ImportPreviewRow,
  ImportPreviewSummary,
  ImportRowError,
} from "@/features/io/types";

/**
 * Card / filter class of a preview row. Staged importers send the row
 * status (new, duplicate, conflict …); the classic preview only knows
 * valid / invalid rows.
 */
export type ImportRowClass = string;

export type ImportStatusTone = "success" | "warning" | "danger" | "default";

/** Display order of the status cards. */
const STATUS_ORDER = [
  "new",
  "link",
  "valid",
  "applied",
  "duplicate",
  "conflict",
  "invalid",
  "undone",
  "undo_rejected",
];

const STATUS_TONES: Record<string, ImportStatusTone> = {
  new: "success",
  link: "success",
  valid: "success",
  applied: "success",
  duplicate: "warning",
  conflict: "warning",
  undo_rejected: "warning",
  invalid: "danger",
  undone: "default",
};

/** Row statuses that are (or were) written by the import. */
const WRITE_STATUSES = new Set(["new", "link", "valid"]);

export function importStatusTone(status: string): ImportStatusTone {
  return STATUS_TONES[status] ?? "default";
}

export function isStagedSummary(summary?: ImportPreviewSummary | null) {
  return Boolean(summary?.counts && Object.keys(summary.counts).length);
}

export type ImportStatusCount = { status: ImportRowClass; count: number };

/**
 * Status cards of a summary: the non-zero staged counts (new, duplicate …)
 * or valid / invalid for the classic preview.
 */
export function importStatusCounts(
  summary?: ImportPreviewSummary | null,
): ImportStatusCount[] {
  if (!summary) return [];
  if (!isStagedSummary(summary)) {
    return [
      { status: "valid", count: summary.valid ?? 0 },
      { status: "invalid", count: summary.invalid ?? 0 },
    ];
  }
  const entries = Object.entries(summary.counts ?? {}).filter(
    ([, count]) => (count ?? 0) > 0,
  );
  const rank = (status: string) => {
    const i = STATUS_ORDER.indexOf(status);
    return i < 0 ? STATUS_ORDER.length : i;
  };
  return entries
    .map(([status, count]) => ({ status, count: count ?? 0 }))
    .sort((a, b) => rank(a.status) - rank(b.status));
}

export type ImportTableRow = {
  index: number;
  status: ImportRowClass;
  statusLabel?: string;
  data: Record<string, unknown>;
  target?: Record<string, unknown>;
  errors: ImportRowError[];
};

/**
 * Preview table rows: the summary rows with their errors; rows that only
 * appear in `errors` (classic preview) are added as invalid.
 */
export function importTableRows(
  summary?: ImportPreviewSummary | null,
): ImportTableRow[] {
  if (!summary) return [];
  const errorsByRow = new Map<number, ImportRowError[]>();
  for (const err of summary.errors ?? []) {
    const list = errorsByRow.get(err.index) ?? [];
    list.push(err);
    errorsByRow.set(err.index, list);
  }
  const rows = new Map<number, ImportTableRow>();
  for (const row of summary.rows ?? []) {
    rows.set(row.index, toTableRow(row, errorsByRow.get(row.index) ?? []));
  }
  for (const [index, errors] of errorsByRow) {
    if (!rows.has(index)) {
      rows.set(index, { index, status: "invalid", data: {}, errors });
    }
  }
  return [...rows.values()].sort((a, b) => a.index - b.index);
}

function toTableRow(
  row: ImportPreviewRow,
  errors: ImportRowError[],
): ImportTableRow {
  const status = row.status || (errors.length ? "invalid" : "valid");
  return {
    index: row.index,
    status,
    statusLabel: row.status_label,
    data: row.data ?? {},
    target: row.target,
    errors,
  };
}

/** Rows the import would write and rows it skips. */
export function importPlan(summary?: ImportPreviewSummary | null) {
  if (!summary) {
    return { create: 0, update: 0, importable: 0, skipped: 0, known: false };
  }
  if (!isStagedSummary(summary)) {
    const importable = summary.valid ?? 0;
    return {
      create: 0,
      update: 0,
      importable,
      skipped: Math.max(0, (summary.total ?? 0) - importable),
      known: false,
    };
  }
  const counts = summary.counts ?? {};
  const create = counts.new ?? 0;
  const update = counts.link ?? 0;
  let importable = 0;
  for (const [status, count] of Object.entries(counts)) {
    if (WRITE_STATUSES.has(status)) importable += count ?? 0;
  }
  return {
    create,
    update,
    importable,
    skipped: Math.max(0, (summary.total ?? 0) - importable),
    known: true,
  };
}

/** Whether a staged undo left rows in place (undo_rejected). */
export function isPartiallyUndone(
  job?: Pick<ImportJob, "status" | "preview_summary"> | null,
) {
  return (
    job?.status === "rolled_back" &&
    (job.preview_summary?.counts?.undo_rejected ?? 0) > 0
  );
}

/** Job statuses the result step keeps polling. */
export function isImportJobRunning(status?: string) {
  return status === "queued" || status === "applying";
}

/** Error message: the backend's localized text, else the code's i18n key. */
export function importErrorMessage(
  err: ImportRowError,
  t: (key: string) => string,
): string {
  if (err.error) return err.error;
  if (err.code) {
    const key = `imports.error_codes.${err.code}`;
    const text = t(key);
    if (text !== key) return text;
    return err.code;
  }
  return t("imports.row_error_unknown");
}

type Tp = (
  key: string,
  count: number,
  params?: Record<string, string | number>,
) => string;

export type ImportConfirmCopy = {
  /** What happens, in one or two sentences. */
  sentence: string;
  /** Submit button label carrying the count. */
  action: string;
};

export type ImportConfirmContext = {
  summary?: ImportPreviewSummary | null;
  t: (key: string, params?: Record<string, string | number>) => string;
  tp: Tp;
};

/** Builds the confirmation copy of one import resource. */
export type ImportConfirmBuilder = (
  ctx: ImportConfirmContext,
) => ImportConfirmCopy;

function skippedSentence(skipped: number, tp: Tp) {
  return skipped > 0 ? ` ${tp("imports.confirm_step.skipped", skipped)}` : "";
}

/**
 * Generic confirmation: "N records will be created, M updated, K rows
 * skipped." The classic preview does not know create vs update, so it
 * says how many rows are imported.
 */
export const genericConfirmCopy: ImportConfirmBuilder = ({
  summary,
  t,
  tp,
}) => {
  const plan = importPlan(summary);
  const parts: string[] = [];
  if (plan.known) {
    if (plan.create > 0)
      parts.push(tp("imports.confirm_step.will_create", plan.create));
    if (plan.update > 0)
      parts.push(tp("imports.confirm_step.will_update", plan.update));
  }
  if (!parts.length)
    parts.push(tp("imports.confirm_step.will_import", plan.importable));
  const head = parts.join(t("imports.confirm_step.list_separator"));
  return {
    // Languages that start with the verb (ru: "будет создана …") get a
    // capital first letter.
    sentence: `${head.charAt(0).toLocaleUpperCase()}${head.slice(1)}${t("imports.confirm_step.sentence_end")}${skippedSentence(plan.skipped, tp)}`,
    action: tp("imports.confirm_step.action", plan.importable),
  };
};

/** Most common string value of a target field over the preview rows. */
export function dominantTargetValue(
  summary: ImportPreviewSummary | null | undefined,
  key: string,
  statuses: string[] = ["new"],
): string | null {
  const tally = new Map<string, number>();
  for (const row of summary?.rows ?? []) {
    if (row.status && !statuses.includes(row.status)) continue;
    const value = row.target?.[key];
    if (typeof value !== "string" || !value) continue;
    tally.set(value, (tally.get(value) ?? 0) + 1);
  }
  let best: string | null = null;
  let bestCount = 0;
  for (const [value, count] of tally) {
    if (count > bestCount) {
      best = value;
      bestCount = count;
    }
  }
  return best;
}

/**
 * New rows with a location code, or null when the preview does not list
 * every new row (the backend caps the preview rows).
 */
export function locatedNewRows(
  summary?: ImportPreviewSummary | null,
): number | null {
  const rows = (summary?.rows ?? []).filter((row) => row.status === "new");
  if (rows.length !== (summary?.counts?.new ?? 0)) return null;
  return rows.filter((row) => Boolean(row.target?.location_code)).length;
}

/**
 * Stock import (TEC-158, TEC-759): "412 units will be added to Merkez Depo
 * stock and one stock entry document is recorded. 40 of them go straight
 * to the location in the file. 23 rows will be skipped."
 */
export const stockConfirmCopy: ImportConfirmBuilder = ({ summary, t, tp }) => {
  const plan = importPlan(summary);
  const target =
    dominantTargetValue(summary, "organization_name") ??
    t("imports.confirm_step.stock_target_fallback");
  const located = locatedNewRows(summary) ?? 0;
  const locatedSentence =
    located > 0 ? ` ${tp("imports.confirm_step.stock_located", located)}` : "";
  return {
    sentence: `${tp("imports.confirm_step.stock_sentence", plan.importable, { target })}${locatedSentence}${skippedSentence(plan.skipped, tp)}`,
    action: tp("imports.confirm_step.stock_action", plan.importable),
  };
};

/** Resource-specific confirmation copy; others use `genericConfirmCopy`. */
export const IMPORT_CONFIRM_COPY: Record<string, ImportConfirmBuilder> = {
  "tenant.stock.units": stockConfirmCopy,
};

export function importConfirmCopy(
  resource: string,
  ctx: ImportConfirmContext,
  override?: ImportConfirmBuilder,
): ImportConfirmCopy {
  const build = override ?? IMPORT_CONFIRM_COPY[resource] ?? genericConfirmCopy;
  return build(ctx);
}

export type ImportTargetPart = { kind: string; value: string };

/**
 * Human-readable parts of a row target (ids are left out): product,
 * location (else organization), effective date and other names / codes.
 */
export function importTargetParts(
  target?: Record<string, unknown> | null,
): ImportTargetPart[] {
  if (!target) return [];
  const text = (key: string) => {
    const v = target[key];
    return typeof v === "string" || typeof v === "number" ? String(v) : "";
  };
  const parts: ImportTargetPart[] = [];
  const used = new Set<string>();
  const take = (kind: string, ...keys: string[]) => {
    const values = keys.map(text).filter(Boolean);
    keys.forEach((k) => used.add(k));
    if (values.length) parts.push({ kind, value: values.join(" · ") });
  };
  take("product", "product_sku", "product_name");
  if (text("location_code")) {
    take("location", "location_code");
    used.add("organization_name");
  } else {
    take("organization", "organization_name");
  }
  take("effective_from", "effective_from");
  for (const key of Object.keys(target)) {
    if (used.has(key) || !/(_name|_code|plate)$/.test(key)) continue;
    take("other", key);
  }
  return parts;
}
