import type { Formatter } from "@/lib/i18n/format";

import {
  EMPTY,
  formatValue,
  isNumber,
  type DurationLabels,
  type ValueType,
} from "../format";
import type {
  GridAggregate,
  GridColumn,
  GridFilterKind,
  GridRow,
  StatusCell,
} from "../types";

export const NUMERIC_TYPES: readonly ValueType[] = [
  "number",
  "money",
  "percent",
  "duration",
];

export function isNumericColumn(col: Pick<GridColumn, "type">): boolean {
  return NUMERIC_TYPES.includes(col.type ?? "text");
}

export function columnAlign(
  col: Pick<GridColumn, "align" | "type">,
): "left" | "right" {
  return col.align ?? (isNumericColumn(col) ? "right" : "left");
}

export function defaultFilterKind(
  col: Pick<GridColumn, "filter" | "type">,
): GridFilterKind {
  if (col.filter) return col.filter;
  if (isNumericColumn(col)) return "range";
  if (col.type === "status") return "select";
  if (col.type === "link") return "none";
  return "text";
}

export function canGroupBy(
  col: Pick<GridColumn, "groupable" | "type">,
): boolean {
  if (col.groupable !== undefined) return col.groupable;
  return ["text", "status", "org", "date"].includes(col.type ?? "text");
}

/** Raw value of a cell (sort, filter, aggregate, export). */
export function cellValue<R extends GridRow>(
  row: R,
  col: GridColumn<R>,
): unknown {
  return col.accessor ? col.accessor(row) : row[col.key];
}

/** Numeric reading of a cell (numeric strings from JSON included). */
export function numericValue(v: unknown): number | null {
  if (isNumber(v)) return v;
  if (typeof v === "string" && v.trim() !== "" && Number.isFinite(Number(v)))
    return Number(v);
  return null;
}

/** Status label + tone of a status cell (`_status[col]` or raw value). */
export function statusOf<R extends GridRow>(
  row: R,
  col: GridColumn<R>,
): StatusCell | null {
  const raw = cellValue(row, col);
  const status = row._status?.[col.key];
  if (status) return status;
  if (raw === null || raw === undefined || raw === "") return null;
  return { label: String(raw), tone: "neutral" };
}

export type CellFormatContext = {
  fmt: Formatter;
  durations: DurationLabels;
};

/** Display text of a cell. Also used by CSV, search and tooltips. */
export function formatCell<R extends GridRow>(
  row: R,
  col: GridColumn<R>,
  ctx: CellFormatContext,
): string {
  const v = cellValue(row, col);
  return formatTyped(v, col, ctx, row);
}

/** Display text of a (possibly aggregated) value of a column. */
export function formatTyped<R extends GridRow>(
  v: unknown,
  col: GridColumn<R>,
  { fmt, durations }: CellFormatContext,
  row?: R,
): string {
  const type = col.type ?? "text";
  if (v === null || v === undefined || v === "") return EMPTY;
  switch (type) {
    case "number":
    case "money":
    case "percent":
    case "duration": {
      const n = numericValue(v);
      if (n === null) return String(v);
      return formatValue(
        n,
        {
          kind: type,
          currency:
            (typeof row?.currency === "string" && row.currency) || col.currency,
          maximumFractionDigits: col.maximumFractionDigits,
        },
        fmt,
        durations,
      );
    }
    case "date":
      return fmt.date(v as string);
    case "datetime":
      return fmt.dateTime(v as string);
    case "status":
      return row ? (statusOf(row, col)?.label ?? String(v)) : String(v);
    default:
      if (typeof v === "object") {
        const o = v as { label?: unknown; name?: unknown };
        return String(o.label ?? o.name ?? "");
      }
      return String(v);
  }
}

/** sum / avg over the numeric values; null when there are none. */
export function aggregate(
  values: readonly unknown[],
  kind: GridAggregate | undefined,
): number | null {
  if (!kind || kind === "none") return null;
  let sum = 0;
  let n = 0;
  for (const raw of values) {
    const v = numericValue(raw);
    if (v === null) continue;
    sum += v;
    n++;
  }
  if (n === 0) return null;
  return kind === "avg" ? sum / n : sum;
}

/** Column totals for the footer row: key → aggregate. */
export function columnTotals<R extends GridRow>(
  rows: readonly R[],
  columns: readonly GridColumn<R>[],
): Record<string, number | null> {
  const out: Record<string, number | null> = {};
  for (const col of columns) {
    if (!col.aggregate || col.aggregate === "none") continue;
    out[col.key] = aggregate(
      rows.map((r) => cellValue(r, col)),
      col.aggregate,
    );
  }
  return out;
}

// ---- filters -------------------------------------------------------------

export type RangeFilterValue = { min?: number | null; max?: number | null };
export type SelectFilterValue = string[];

export function fold(s: string, locale?: string): string {
  return s.toLocaleLowerCase(locale).normalize("NFD").replace(/\p{M}/gu, "");
}

export function matchesText(
  cell: unknown,
  query: string,
  locale?: string,
): boolean {
  const q = fold(query.trim(), locale);
  if (!q) return true;
  if (cell === null || cell === undefined) return false;
  return fold(String(cell), locale).includes(q);
}

export function matchesRange(cell: unknown, range: RangeFilterValue): boolean {
  const min = isNumber(range.min) ? range.min : null;
  const max = isNumber(range.max) ? range.max : null;
  if (min === null && max === null) return true;
  const v = numericValue(cell);
  if (v === null) return false;
  if (min !== null && v < min) return false;
  if (max !== null && v > max) return false;
  return true;
}

export function matchesSelect(
  cell: unknown,
  values: SelectFilterValue,
): boolean {
  if (!values.length) return true;
  return values.includes(
    cell === null || cell === undefined ? "" : String(cell),
  );
}

/** Rows passing every column filter + the global search (pure, for tests). */
export function filterRows<R extends GridRow>(
  rows: readonly R[],
  columns: readonly GridColumn<R>[],
  filters: Record<string, unknown>,
  search: string,
  ctx: CellFormatContext,
): R[] {
  return rows.filter((row) => {
    for (const col of columns) {
      const f = filters[col.key];
      if (f === undefined) continue;
      const kind = defaultFilterKind(col);
      const v = cellValue(row, col);
      if (kind === "range" && !matchesRange(v, f as RangeFilterValue))
        return false;
      if (kind === "select" && !matchesSelect(v, f as SelectFilterValue))
        return false;
      if (
        kind === "text" &&
        !matchesText(formatCell(row, col, ctx), f as string, ctx.fmt.locale)
      )
        return false;
    }
    if (!search.trim()) return true;
    return columns.some((col) =>
      matchesText(searchText(row, col, ctx), search, ctx.fmt.locale),
    );
  });
}

/** Text a global search looks at: formatted and raw value. */
export function searchText<R extends GridRow>(
  row: R,
  col: GridColumn<R>,
  ctx: CellFormatContext,
): string {
  const raw = cellValue(row, col);
  const formatted = formatCell(row, col, ctx);
  return raw === null || raw === undefined || typeof raw === "object"
    ? formatted
    : `${formatted} ${String(raw)}`;
}

// ---- export --------------------------------------------------------------

function csvEscape(text: string): string {
  return /[",\n\r;]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
}

/** Plain export value: numbers stay numbers, the rest is display text. */
export function exportValue<R extends GridRow>(
  row: R,
  col: GridColumn<R>,
  ctx: CellFormatContext,
): string | number | null {
  const v = cellValue(row, col);
  if (v === null || v === undefined || v === "") return null;
  if (isNumericColumn(col)) return numericValue(v) ?? formatCell(row, col, ctx);
  if (col.type === "date" || col.type === "datetime") return String(v);
  return formatCell(row, col, ctx);
}

/**
 * RFC 4180 CSV (comma, CRLF, UTF-8 BOM for Excel). Numbers are written
 * unformatted with "." decimals so spreadsheets read them as numbers.
 */
export function toCsv<R extends GridRow>(
  columns: readonly GridColumn<R>[],
  rows: readonly R[],
  ctx: CellFormatContext,
  totals?: { label: string; values: Record<string, number | null> },
): string {
  const lines = [columns.map((c) => csvEscape(c.label)).join(",")];
  for (const row of rows) {
    lines.push(
      columns
        .map((c) => {
          const v = exportValue(row, c, ctx);
          return v === null ? "" : csvEscape(String(v));
        })
        .join(","),
    );
  }
  if (totals && Object.keys(totals.values).length) {
    lines.push(
      columns
        .map((c, i) => {
          const v = totals.values[c.key];
          if (isNumber(v)) return String(v);
          return i === 0 ? csvEscape(totals.label) : "";
        })
        .join(","),
    );
  }
  return `﻿${lines.join("\r\n")}\r\n`;
}

/** Number format codes per column type (XLSX cell `z`). */
export function xlsxNumberFormat(
  col: Pick<GridColumn, "type" | "maximumFractionDigits">,
): string | undefined {
  switch (col.type) {
    case "money":
      return "#,##0.00";
    case "percent":
      return "0.0%";
    case "number":
      return col.maximumFractionDigits === 0 ? "#,##0" : "#,##0.##";
    default:
      return undefined;
  }
}

/** Array-of-arrays sheet (header first). Percent → fraction for "0.0%". */
export function toSheetRows<R extends GridRow>(
  columns: readonly GridColumn<R>[],
  rows: readonly R[],
  ctx: CellFormatContext,
  totals?: { label: string; values: Record<string, number | null> },
): Array<Array<string | number | null>> {
  const body = rows.map((row) =>
    columns.map((c) => {
      const v = exportValue(row, c, ctx);
      return c.type === "percent" && isNumber(v) ? v / 100 : v;
    }),
  );
  const out: Array<Array<string | number | null>> = [
    columns.map((c) => c.label),
    ...body,
  ];
  if (totals && Object.keys(totals.values).length) {
    out.push(
      columns.map((c, i) => {
        const v = totals.values[c.key];
        if (isNumber(v)) return c.type === "percent" ? v / 100 : v;
        return i === 0 ? totals.label : null;
      }),
    );
  }
  return out;
}

/** Builds an .xlsx Blob with the `xlsx` package (loaded on demand). */
export async function toXlsxBlob<R extends GridRow>(
  columns: readonly GridColumn<R>[],
  rows: readonly R[],
  ctx: CellFormatContext,
  sheetName: string,
  totals?: { label: string; values: Record<string, number | null> },
): Promise<Blob> {
  const XLSX = await import("xlsx");
  const aoa = toSheetRows(columns, rows, ctx, totals);
  const ws = XLSX.utils.aoa_to_sheet(aoa);
  columns.forEach((col, ci) => {
    const z = xlsxNumberFormat(col);
    if (!z) return;
    for (let ri = 1; ri < aoa.length; ri++) {
      const ref = XLSX.utils.encode_cell({ r: ri, c: ci });
      const cell = ws[ref] as { t?: string; z?: string } | undefined;
      if (cell && cell.t === "n") cell.z = z;
    }
  });
  ws["!cols"] = columns.map((c) => ({ wch: Math.max(10, c.label.length + 2) }));
  const wb = XLSX.utils.book_new();
  XLSX.utils.book_append_sheet(wb, ws, sheetName.slice(0, 31) || "Sheet1");
  const out = XLSX.write(wb, {
    type: "array",
    bookType: "xlsx",
  }) as ArrayBuffer;
  return new Blob([out], {
    type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  });
}

/** `name-2026-11-15.ext` with a safe base name. */
export function exportFileName(
  base: string,
  ext: string,
  now = new Date(),
): string {
  const safe =
    base
      .replace(/ı/g, "i")
      .normalize("NFD")
      .replace(/\p{M}/gu, "")
      .replace(/[^a-zA-Z0-9._-]+/g, "-")
      .replace(/^-+|-+$/g, "")
      .toLowerCase() || "report";
  const d = now.toISOString().slice(0, 10);
  return `${safe}-${d}.${ext}`;
}

// ---- virtual window ------------------------------------------------------

/** Visible row window for fixed-height rows. */
export function virtualWindow(
  count: number,
  scrollTop: number,
  viewport: number,
  rowHeight: number,
  overscan = 8,
): { start: number; end: number; padTop: number; padBottom: number } {
  if (count <= 0) return { start: 0, end: 0, padTop: 0, padBottom: 0 };
  const first = Math.floor(Math.max(0, scrollTop) / rowHeight);
  const visible = Math.ceil(viewport / rowHeight);
  const start = Math.max(0, first - overscan);
  const end = Math.min(count, first + visible + overscan);
  return {
    start,
    end,
    padTop: start * rowHeight,
    padBottom: (count - end) * rowHeight,
  };
}
