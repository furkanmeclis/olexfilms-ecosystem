import { describe, expect, it } from "vitest";

import { createFormatter } from "@/lib/i18n/format";

import type { GridColumn, GridRow } from "../types";
import { gridPrefsKey, reconcileOrder } from "./column-prefs";
import {
  aggregate,
  columnTotals,
  defaultFilterKind,
  exportFileName,
  filterRows,
  formatCell,
  matchesRange,
  matchesText,
  toCsv,
  toSheetRows,
  toXlsxBlob,
  virtualWindow,
} from "./grid-utils";

const fmt = createFormatter({ locale: "tr", timeZone: "Europe/Istanbul" });
const ctx = {
  fmt,
  durations: { day: "g", hour: "sa", minute: "dk", second: "sn" },
};
const norm = (s: string) => s.replace(/ | /g, " ");

const columns: GridColumn[] = [
  { key: "dealer", label: "Bayi", type: "org" },
  { key: "status", label: "Durum", type: "status" },
  { key: "services", label: "Hizmet", type: "number", aggregate: "sum" },
  {
    key: "revenue",
    label: "Ciro",
    type: "money",
    aggregate: "sum",
    currency: "TRY",
  },
  { key: "margin", label: "Marj", type: "percent", aggregate: "avg" },
  { key: "prep", label: "Süre", type: "duration" },
  { key: "day", label: "Gün", type: "date" },
];

const rows: GridRow[] = [
  {
    dealer: "Olex Kadıköy",
    status: "active",
    services: 10,
    revenue: 1500.5,
    margin: 20,
    prep: 3600,
    day: "2026-11-01",
    _status: { status: { label: "Aktif", tone: "done" } },
  },
  {
    dealer: "Olex Çankaya",
    status: "overdue",
    services: 4,
    revenue: 250,
    margin: 10,
    prep: 5400,
    day: "2026-11-02",
    currency: "EUR",
    _status: { status: { label: "Gecikmiş", tone: "problem" } },
  },
  {
    dealer: "Olex İzmir",
    status: null,
    services: null,
    revenue: "99.5",
    margin: null,
  },
];

describe("grid formatting", () => {
  it("formats by column type", () => {
    expect(norm(formatCell(rows[0], columns[3], ctx))).toBe("₺1.500,50");
    expect(norm(formatCell(rows[1], columns[3], ctx))).toBe("€250,00");
    expect(norm(formatCell(rows[0], columns[4], ctx))).toBe("%20");
    expect(formatCell(rows[1], columns[5], ctx)).toBe("1 sa 30 dk");
    expect(formatCell(rows[0], columns[1], ctx)).toBe("Aktif");
    expect(formatCell(rows[2], columns[2], ctx)).toBe("—");
    expect(formatCell(rows[0], columns[6], ctx)).toBe("01.11.2026");
  });
  it("picks a filter kind from the type", () => {
    expect(defaultFilterKind(columns[0])).toBe("text");
    expect(defaultFilterKind(columns[1])).toBe("select");
    expect(defaultFilterKind(columns[3])).toBe("range");
  });
});

describe("grid aggregation", () => {
  it("sums and averages numeric values only", () => {
    expect(aggregate([1, 2, null, "3"], "sum")).toBe(6);
    expect(aggregate([2, 4, undefined], "avg")).toBe(3);
    expect(aggregate([null], "sum")).toBeNull();
    expect(aggregate([1], "none")).toBeNull();
  });
  it("computes column totals", () => {
    expect(columnTotals(rows, columns)).toEqual({
      services: 14,
      revenue: 1850,
      margin: 15,
    });
  });
});

describe("grid filters", () => {
  it("matches text case- and accent-insensitively", () => {
    expect(matchesText("Olex Çankaya", "cank", "tr")).toBe(true);
    expect(matchesText("İzmir", "izmir", "tr")).toBe(true);
    expect(matchesText(null, "x")).toBe(false);
  });
  it("matches ranges", () => {
    expect(matchesRange(5, { min: 1, max: 5 })).toBe(true);
    expect(matchesRange(6, { max: 5 })).toBe(false);
    expect(matchesRange(null, { min: 0 })).toBe(false);
    expect(matchesRange(null, {})).toBe(true);
  });
  it("combines column filters and the global search", () => {
    expect(
      filterRows(rows, columns, { services: { min: 5 } }, "", ctx),
    ).toHaveLength(1);
    expect(
      filterRows(rows, columns, { status: ["overdue"] }, "", ctx)[0].dealer,
    ).toBe("Olex Çankaya");
    expect(filterRows(rows, columns, {}, "gecikmiş", ctx)).toHaveLength(1);
    expect(
      filterRows(rows, columns, { dealer: "olex" }, "izmir", ctx),
    ).toHaveLength(1);
  });
});

describe("grid export", () => {
  const totals = { label: "Toplam", values: columnTotals(rows, columns) };
  it("writes CSV with BOM, raw numbers and a totals row", () => {
    const csv = toCsv(columns.slice(0, 4), rows.slice(0, 2), ctx, totals);
    expect(csv.startsWith("﻿")).toBe(true);
    const lines = csv.slice(1).trim().split("\r\n");
    expect(lines[0]).toBe("Bayi,Durum,Hizmet,Ciro");
    expect(lines[1]).toBe("Olex Kadıköy,Aktif,10,1500.5");
    expect(lines[3]).toBe("Toplam,,14,1850");
  });
  it("quotes CSV fields with separators", () => {
    const csv = toCsv([columns[0]], [{ dealer: 'A, "B"' }], ctx);
    expect(csv).toContain('"A, ""B"""');
  });
  it("keeps numbers numeric in sheets (percent as fraction)", () => {
    const sheet = toSheetRows(columns, rows.slice(0, 1), ctx, totals);
    expect(sheet[0][0]).toBe("Bayi");
    expect(sheet[1][2]).toBe(10);
    expect(sheet[1][4]).toBeCloseTo(0.2);
    expect(sheet[2][0]).toBe("Toplam");
  });
  it("builds a readable xlsx", async () => {
    const blob = await toXlsxBlob(columns, rows, ctx, "Rapor", totals);
    const XLSX = await import("xlsx");
    const wb = XLSX.read(new Uint8Array(await blob.arrayBuffer()), {
      type: "array",
      cellNF: true,
    });
    const ws = wb.Sheets[wb.SheetNames[0]];
    expect(wb.SheetNames[0]).toBe("Rapor");
    expect(ws.C2.v).toBe(10);
    expect(ws.D2.z).toBe("#,##0.00");
  });
  it("names export files safely", () => {
    expect(
      exportFileName(
        "Bayi Raporu / Kasım",
        "csv",
        new Date("2026-11-15T10:00:00Z"),
      ),
    ).toBe("bayi-raporu-kasim-2026-11-15.csv");
  });
});

describe("grid helpers", () => {
  it("windows rows for virtualisation", () => {
    expect(virtualWindow(1000, 3600, 360, 36, 5)).toEqual({
      start: 95,
      end: 115,
      padTop: 95 * 36,
      padBottom: (1000 - 115) * 36,
    });
    expect(virtualWindow(0, 0, 100, 36)).toEqual({
      start: 0,
      end: 0,
      padTop: 0,
      padBottom: 0,
    });
  });
  it("keys preferences per user and reconciles column order", () => {
    expect(gridPrefsKey("x", "u1")).not.toBe(gridPrefsKey("x", "u2"));
    expect(reconcileOrder(["b", "gone", "a"], ["a", "b", "c"])).toEqual([
      "b",
      "a",
      "c",
    ]);
  });
});
