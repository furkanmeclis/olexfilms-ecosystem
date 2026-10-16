import type { ImportFormat } from "@/features/io/types";

/** Rows read in the browser for the column step's sample values. */
export const IMPORT_SAMPLE_ROWS = 3;

export const IMPORT_ACCEPT = ".xlsx,.csv,.tsv,.tab,.json";

export type ImportFileHead = {
  /** Column headers of the first row (trimmed, empty ones dropped). */
  headers: string[];
  /** First data rows keyed by header. */
  samples: Record<string, string>[];
};

const EXTENSION_FORMATS: Record<string, ImportFormat> = {
  xlsx: "xlsx",
  csv: "csv",
  tsv: "tsv",
  tab: "tsv",
  json: "json",
};

/** Import format from the file name extension; null when unsupported. */
export function detectImportFormat(filename: string): ImportFormat | null {
  const dot = filename.lastIndexOf(".");
  if (dot < 0) return null;
  return EXTENSION_FORMATS[filename.slice(dot + 1).toLowerCase()] ?? null;
}

/**
 * Splits delimited text into records (RFC 4180 quotes, CRLF), stopping
 * after `maxRecords` non-empty records.
 */
export function parseDelimited(
  text: string,
  delimiter: string,
  maxRecords: number,
): string[][] {
  const records: string[][] = [];
  let record: string[] = [];
  let field = "";
  let quoted = false;
  const pushRecord = () => {
    record.push(field);
    field = "";
    if (record.some((cell) => cell.trim() !== "")) records.push(record);
    record = [];
  };
  for (let i = 0; i < text.length && records.length < maxRecords; i++) {
    const ch = text[i];
    if (quoted) {
      if (ch === '"' && text[i + 1] === '"') {
        field += '"';
        i++;
      } else if (ch === '"') {
        quoted = false;
      } else {
        field += ch;
      }
      continue;
    }
    if (ch === '"' && field === "") {
      quoted = true;
    } else if (ch === delimiter) {
      record.push(field);
      field = "";
    } else if (ch === "\n" || ch === "\r") {
      if (ch === "\r" && text[i + 1] === "\n") i++;
      pushRecord();
    } else {
      field += ch;
    }
  }
  if (records.length < maxRecords && (field !== "" || record.length)) {
    pushRecord();
  }
  return records;
}

function cellText(value: unknown): string {
  if (value == null) return "";
  if (value instanceof Date) {
    return Number.isNaN(value.getTime()) ? "" : value.toISOString();
  }
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

/** Header row + sample rows (as the backend reads them: first row = header). */
export function headFromMatrix(
  matrix: unknown[][],
  sampleRows = IMPORT_SAMPLE_ROWS,
): ImportFileHead {
  const [headerRow, ...rest] = matrix;
  if (!headerRow) return { headers: [], samples: [] };
  const columns = headerRow
    .map((h, index) => ({ header: cellText(h).trim(), index }))
    .filter((c) => c.header !== "");
  const samples = rest.slice(0, sampleRows).map((row) => {
    const out: Record<string, string> = {};
    for (const { header, index } of columns) {
      out[header] = cellText(row?.[index]);
    }
    return out;
  });
  return { headers: columns.map((c) => c.header), samples };
}

/** Header + sample rows of a JSON array of objects. */
export function headFromJson(
  data: unknown,
  sampleRows = IMPORT_SAMPLE_ROWS,
): ImportFileHead {
  if (!Array.isArray(data)) return { headers: [], samples: [] };
  const objects = data.filter(
    (item): item is Record<string, unknown> =>
      Boolean(item) && typeof item === "object" && !Array.isArray(item),
  );
  if (!objects.length) return { headers: [], samples: [] };
  const headers = Object.keys(objects[0]);
  const samples = objects.slice(0, sampleRows).map((item) => {
    const out: Record<string, string> = {};
    for (const header of headers) out[header] = cellText(item[header]);
    return out;
  });
  return { headers, samples };
}

async function readXlsxHead(file: File): Promise<ImportFileHead> {
  const XLSX = await import("xlsx");
  const workbook = XLSX.read(await file.arrayBuffer(), {
    type: "array",
    cellDates: true,
    sheetRows: IMPORT_SAMPLE_ROWS + 1,
  });
  const sheetName = workbook.SheetNames[0];
  const sheet = sheetName ? workbook.Sheets[sheetName] : undefined;
  if (!sheet) return { headers: [], samples: [] };
  const matrix = XLSX.utils.sheet_to_json<unknown[]>(sheet, {
    header: 1,
    raw: false,
    defval: "",
    blankrows: false,
  });
  return headFromMatrix(matrix);
}

/**
 * Reads the header row and the first rows of an import file in the browser
 * (SheetJS for Excel). Unreadable files return no headers; the server
 * mapping is used then.
 */
export async function readImportFileHead(
  file: File,
  format: ImportFormat,
): Promise<ImportFileHead> {
  try {
    if (format === "xlsx") return await readXlsxHead(file);
    if (format === "csv" || format === "tsv") {
      const text = (await file.slice(0, 256 * 1024).text()).replace(/^﻿/, "");
      const matrix = parseDelimited(
        text,
        format === "tsv" ? "\t" : ",",
        IMPORT_SAMPLE_ROWS + 1,
      );
      return headFromMatrix(matrix);
    }
    return headFromJson(JSON.parse(await file.text()) as unknown);
  } catch {
    return { headers: [], samples: [] };
  }
}
