import type { CellBase, Matrix } from "react-spreadsheet";
import * as XLSX from "xlsx";

import type { ExportFormat } from "@/features/io/types";

export const EXPORT_PREVIEW_MAX_ROWS = 500;

export type ParsedExportSheet = {
  data: Matrix<CellBase<string>>;
  totalRows: number;
  truncated: boolean;
  columnCount: number;
};

function formatCellValue(value: unknown): string {
  if (value == null || value === "") return "";
  if (value instanceof Date) {
    return Number.isNaN(value.getTime()) ? "" : value.toLocaleString();
  }
  return String(value);
}

function normalizeRows(rows: unknown[][]): unknown[][] {
  return rows.map((row) => (Array.isArray(row) ? row : []));
}

function rowsToMatrix(rows: unknown[][], maxRows: number): ParsedExportSheet {
  const normalized = normalizeRows(rows);
  const totalRows = normalized.length;
  const slice = normalized.slice(0, maxRows);
  const columnCount = slice.reduce((max, row) => Math.max(max, row.length), 0);

  const data: Matrix<CellBase<string>> = slice.map((row) =>
    Array.from({ length: columnCount }, (_, index) => ({
      value: formatCellValue(row[index]),
      readOnly: true,
    })),
  );

  return {
    data,
    totalRows,
    truncated: totalRows > maxRows,
    columnCount,
  };
}

export function parseExportArrayBuffer(
  buffer: ArrayBuffer,
  format: Exclude<ExportFormat, "pdf">,
  maxRows = EXPORT_PREVIEW_MAX_ROWS,
): ParsedExportSheet {
  const workbook =
    format === "csv"
      ? XLSX.read(stripUtf8Bom(new TextDecoder("utf-8").decode(buffer)), {
          type: "string",
          raw: false,
          cellDates: true,
        })
      : XLSX.read(new Uint8Array(buffer), {
          type: "array",
          raw: false,
          cellDates: true,
        });

  const sheetName = workbook.SheetNames[0];
  if (!sheetName) {
    return { data: [], totalRows: 0, truncated: false, columnCount: 0 };
  }

  const sheet = workbook.Sheets[sheetName];
  const rows = XLSX.utils.sheet_to_json<unknown[]>(sheet, {
    header: 1,
    defval: "",
    raw: false,
  });

  return rowsToMatrix(rows as unknown[][], maxRows);
}

export async function parseExportBlob(
  blob: Blob,
  format: Exclude<ExportFormat, "pdf">,
  maxRows = EXPORT_PREVIEW_MAX_ROWS,
): Promise<ParsedExportSheet> {
  const buffer = await blob.arrayBuffer();
  return parseExportArrayBuffer(buffer, format, maxRows);
}

function stripUtf8Bom(text: string): string {
  return text.charCodeAt(0) === 0xfeff ? text.slice(1) : text;
}

export function isSpreadsheetExportFormat(
  format: string,
): format is Exclude<ExportFormat, "pdf" | "json"> {
  const normalized = format.toLowerCase();
  return normalized === "csv" || normalized === "xlsx";
}

export function resolveSpreadsheetPreviewFormat(
  filename: string,
  mimeType = "",
): Exclude<ExportFormat, "pdf" | "json"> | null {
  const ext = filename.split(".").pop()?.toLowerCase() ?? "";
  if (ext === "csv" || ext === "tsv") return "csv";
  if (ext === "xlsx" || ext === "xls") return "xlsx";

  const mime = mimeType.toLowerCase();
  if (mime.includes("csv") || mime.includes("tab-separated-values")) {
    return "csv";
  }
  if (mime.includes("spreadsheet") || mime.includes("excel")) {
    return "xlsx";
  }
  return null;
}
