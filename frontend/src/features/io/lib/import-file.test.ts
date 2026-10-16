import * as XLSX from "xlsx";
import { describe, expect, it } from "vitest";

import {
  detectImportFormat,
  headFromJson,
  headFromMatrix,
  parseDelimited,
  readImportFileHead,
} from "@/features/io/lib/import-file";

function xlsxFile(rows: unknown[][], name = "stock.xlsx") {
  const wb = XLSX.utils.book_new();
  XLSX.utils.book_append_sheet(wb, XLSX.utils.aoa_to_sheet(rows), "Sheet1");
  const buf = XLSX.write(wb, {
    type: "array",
    bookType: "xlsx",
  }) as ArrayBuffer;
  return new File([buf], name);
}

describe("detectImportFormat", () => {
  it("maps extensions case-insensitively", () => {
    expect(detectImportFormat("Stok Listesi.XLSX")).toBe("xlsx");
    expect(detectImportFormat("users.csv")).toBe("csv");
    expect(detectImportFormat("export.tsv")).toBe("tsv");
    expect(detectImportFormat("export.tab")).toBe("tsv");
    expect(detectImportFormat("roles.json")).toBe("json");
  });

  it("rejects unknown or missing extensions", () => {
    expect(detectImportFormat("old.xls")).toBeNull();
    expect(detectImportFormat("notes.txt")).toBeNull();
    expect(detectImportFormat("README")).toBeNull();
  });
});

describe("parseDelimited", () => {
  it("handles quotes, escaped quotes, CRLF and blank lines", () => {
    const text =
      'barcode,name\r\n"OLX-1","Film, ""mat"""\r\n\r\nOLX-2,"two\nlines"\nOLX-3,x\n';
    expect(parseDelimited(text, ",", 10)).toEqual([
      ["barcode", "name"],
      ["OLX-1", 'Film, "mat"'],
      ["OLX-2", "two\nlines"],
      ["OLX-3", "x"],
    ]);
  });

  it("stops after maxRecords", () => {
    expect(parseDelimited("a\tb\n1\t2\n3\t4\n", "\t", 2)).toEqual([
      ["a", "b"],
      ["1", "2"],
    ]);
  });
});

describe("headFromMatrix", () => {
  it("trims headers, drops empty ones and keys samples by header", () => {
    const head = headFromMatrix([
      [" Barkod ", "", "SKU"],
      ["OLX-1", "ignored", "PPF"],
      ["OLX-2"],
    ]);
    expect(head.headers).toEqual(["Barkod", "SKU"]);
    expect(head.samples).toEqual([
      { Barkod: "OLX-1", SKU: "PPF" },
      { Barkod: "OLX-2", SKU: "" },
    ]);
  });

  it("keeps at most three sample rows", () => {
    const head = headFromMatrix([["a"], ["1"], ["2"], ["3"], ["4"]]);
    expect(head.samples.map((r) => r.a)).toEqual(["1", "2", "3"]);
  });
});

describe("headFromJson", () => {
  it("reads the keys of the first object", () => {
    expect(
      headFromJson([{ email: "a@x", name: "A" }, { email: "b@x" }]),
    ).toEqual({
      headers: ["email", "name"],
      samples: [
        { email: "a@x", name: "A" },
        { email: "b@x", name: "" },
      ],
    });
    expect(headFromJson({ not: "an array" }).headers).toEqual([]);
  });
});

describe("readImportFileHead", () => {
  it("reads Excel headers that the server did not map", async () => {
    const file = xlsxFile([
      ["Barkod", "Ürün kodu", "Raf", "Not"],
      ["OLX-000001", "PPF-GLOSS", "A-01", "ilk"],
      ["OLX-000002", "PPF-GLOSS", "A-02", ""],
    ]);
    const head = await readImportFileHead(file, "xlsx");
    expect(head.headers).toEqual(["Barkod", "Ürün kodu", "Raf", "Not"]);
    expect(head.samples[0]).toEqual({
      Barkod: "OLX-000001",
      "Ürün kodu": "PPF-GLOSS",
      Raf: "A-01",
      Not: "ilk",
    });
    expect(head.samples).toHaveLength(2);
  });

  it("strips the UTF-8 BOM of CSV files", async () => {
    const file = new File(["﻿email,name\na@x.com,Ayşe\n"], "u.csv");
    const head = await readImportFileHead(file, "csv");
    expect(head.headers).toEqual(["email", "name"]);
    expect(head.samples).toEqual([{ email: "a@x.com", name: "Ayşe" }]);
  });

  it("returns no headers for an unreadable file", async () => {
    const file = new File(["{broken"], "x.json");
    expect(await readImportFileHead(file, "json")).toEqual({
      headers: [],
      samples: [],
    });
  });
});
