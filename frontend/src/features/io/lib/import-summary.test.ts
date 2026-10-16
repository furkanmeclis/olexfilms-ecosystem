import { describe, expect, it } from "vitest";

import en from "@/locales/en/imports.json";
import tr from "@/locales/tr/imports.json";
import {
  genericConfirmCopy,
  importConfirmCopy,
  importErrorMessage,
  importPlan,
  importStatusCounts,
  importTableRows,
  importTargetParts,
  isPartiallyUndone,
} from "@/features/io/lib/import-summary";
import type { ImportPreviewSummary } from "@/features/io/types";

type Catalog = Record<string, string>;

/** t / tp over one imports catalog (en/tr plural: one | other). */
function translator(catalog: Catalog) {
  const fill = (text: string, params?: Record<string, string | number>) =>
    text.replace(/{{\s*(\w+)\s*}}/g, (_, k: string) =>
      String(params?.[k] ?? ""),
    );
  const lookup = (key: string) => catalog[key.replace(/^imports\./, "")];
  const t = (key: string, params?: Record<string, string | number>) => {
    const text = lookup(key);
    return text === undefined ? key : fill(text, params);
  };
  const tp = (
    key: string,
    count: number,
    params?: Record<string, string | number>,
  ) => {
    const text =
      lookup(`${key}_${count === 1 ? "one" : "other"}`) ??
      lookup(`${key}_other`);
    return text === undefined ? key : fill(text, { count, ...params });
  };
  return { t, tp };
}

/** Stock import preview as the staged importer returns it (TEC-158). */
const stockPreview: ImportPreviewSummary = {
  total: 435,
  valid: 412,
  invalid: 23,
  batch_uuid: "0b9c4c1e-0000-4000-8000-00000000b001",
  counts: {
    new: 412,
    duplicate: 15,
    invalid: 6,
    conflict: 2,
    applied: 0,
    undone: 0,
    undo_rejected: 0,
  },
  rows: [
    {
      index: 1,
      data: { barcode: "OLX-1", product_sku: "PPF-GLOSS" },
      status: "new",
      status_label: "Yeni",
      target: {
        organization_uuid: "o1",
        organization_name: "Merkez Depo",
        owner_type: "warehouse_location",
        location_uuid: "l1",
        location_code: "A-01",
        product_uuid: "p1",
        product_sku: "PPF-GLOSS",
        product_name: "Olex PPF Gloss",
      },
    },
    {
      index: 2,
      data: { barcode: "OLX-2", product_sku: "NOPE" },
      status: "invalid",
      target: { organization_name: "Merkez Depo" },
    },
  ],
  errors: [
    {
      index: 2,
      field: "product_sku",
      code: "STOCK_IMPORT_PRODUCT_NOT_FOUND",
      error: "Ürün bulunamadı",
    },
  ],
};

describe("importPlan", () => {
  it("counts new (and linked) rows as importable for staged imports", () => {
    expect(importPlan(stockPreview)).toEqual({
      create: 412,
      update: 0,
      importable: 412,
      skipped: 23,
      known: true,
    });
    expect(
      importPlan({
        total: 5,
        valid: 4,
        invalid: 1,
        counts: { new: 3, link: 1, invalid: 1 },
      }),
    ).toMatchObject({ create: 3, update: 1, importable: 4, skipped: 1 });
  });

  it("uses valid / total for the classic preview", () => {
    expect(importPlan({ total: 10, valid: 8, invalid: 2 })).toEqual({
      create: 0,
      update: 0,
      importable: 8,
      skipped: 2,
      known: false,
    });
  });
});

describe("importStatusCounts", () => {
  it("keeps the non-zero staged counts in display order", () => {
    expect(importStatusCounts(stockPreview)).toEqual([
      { status: "new", count: 412 },
      { status: "duplicate", count: 15 },
      { status: "conflict", count: 2 },
      { status: "invalid", count: 6 },
    ]);
  });

  it("falls back to valid / invalid", () => {
    expect(importStatusCounts({ total: 3, valid: 2, invalid: 1 })).toEqual([
      { status: "valid", count: 2 },
      { status: "invalid", count: 1 },
    ]);
  });
});

describe("importTableRows", () => {
  it("attaches errors and adds error-only rows as invalid", () => {
    const rows = importTableRows({
      total: 3,
      valid: 2,
      invalid: 1,
      rows: [
        { index: 1, data: { email: "a@x" } },
        { index: 3, data: { email: "c@x" } },
      ],
      errors: [{ index: 2, field: "email", error: "invalid email" }],
    });
    expect(rows.map((r) => [r.index, r.status])).toEqual([
      [1, "valid"],
      [2, "invalid"],
      [3, "valid"],
    ]);
    expect(rows[1].errors[0].field).toBe("email");
  });

  it("keeps the staged row status and target", () => {
    const rows = importTableRows(stockPreview);
    expect(rows[1]).toMatchObject({ index: 2, status: "invalid" });
    expect(rows[1].errors[0].code).toBe("STOCK_IMPORT_PRODUCT_NOT_FOUND");
    expect(rows[0].target?.location_code).toBe("A-01");
  });
});

describe("confirmation copy", () => {
  it("stock (tr): units, target and skipped rows", () => {
    const { t, tp } = translator(tr as Catalog);
    const copy = importConfirmCopy("tenant.stock.units", {
      summary: stockPreview,
      t,
      tp,
    });
    expect(copy.sentence).toBe(
      "412 birim Merkez Depo stoğuna eklenecek ve tek bir stok girişi belgesi oluşacak. 23 satır atlanacak.",
    );
    expect(copy.action).toBe("412 birimi içe aktar");
  });

  it("stock (en) pluralizes and leaves out a zero skip", () => {
    const { t, tp } = translator(en as Catalog);
    const summary = {
      ...stockPreview,
      total: 1,
      counts: { new: 1 },
    };
    const copy = importConfirmCopy("tenant.stock.units", { summary, t, tp });
    expect(copy.sentence).toBe(
      "1 unit will be added to the Merkez Depo stock and a single stock entry document will be created.",
    );
    expect(copy.action).toBe("Import 1 unit");
  });

  it("generic staged: created, updated and skipped", () => {
    const { t, tp } = translator(tr as Catalog);
    const copy = genericConfirmCopy({
      summary: {
        total: 10,
        valid: 8,
        invalid: 2,
        counts: { new: 5, link: 3, invalid: 2 },
      },
      t,
      tp,
    });
    expect(copy.sentence).toBe(
      "5 kayıt oluşturulacak, 3 kayıt güncellenecek. 2 satır atlanacak.",
    );
    expect(copy.action).toBe("8 satırı içe aktar");
  });

  it("generic classic preview does not guess create vs update", () => {
    const { t, tp } = translator(en as Catalog);
    const copy = importConfirmCopy("platform.users", {
      summary: { total: 12, valid: 12, invalid: 0 },
      t,
      tp,
    });
    expect(copy.sentence).toBe("12 rows will be imported.");
    expect(copy.action).toBe("Import 12 rows");
  });

  it("a caller can override the copy", () => {
    const { t, tp } = translator(en as Catalog);
    const copy = importConfirmCopy(
      "tenant.stock.units",
      { summary: stockPreview, t, tp },
      () => ({ sentence: "custom", action: "go" }),
    );
    expect(copy).toEqual({ sentence: "custom", action: "go" });
  });
});

describe("helpers", () => {
  it("prefers the backend message, then the code key", () => {
    const t = (key: string) =>
      key === "imports.error_codes.X" ? "translated" : key;
    expect(
      importErrorMessage({ index: 1, error: "from backend", code: "X" }, t),
    ).toBe("from backend");
    expect(importErrorMessage({ index: 1, error: "", code: "X" }, t)).toBe(
      "translated",
    );
    expect(importErrorMessage({ index: 1, error: "", code: "Y" }, t)).toBe("Y");
  });

  it("formats the target without ids", () => {
    expect(importTargetParts(stockPreview.rows?.[0].target)).toEqual([
      { kind: "product", value: "PPF-GLOSS · Olex PPF Gloss" },
      { kind: "location", value: "A-01" },
    ]);
    expect(
      importTargetParts({ organization_name: "Merkez Depo", fleet_uuid: "f" }),
    ).toEqual([{ kind: "organization", value: "Merkez Depo" }]);
  });

  it("detects a partial undo", () => {
    expect(
      isPartiallyUndone({
        status: "rolled_back",
        preview_summary: {
          total: 2,
          valid: 2,
          invalid: 0,
          counts: { undone: 1, undo_rejected: 1 },
        },
      }),
    ).toBe(true);
    expect(
      isPartiallyUndone({
        status: "rolled_back",
        preview_summary: {
          total: 2,
          valid: 2,
          invalid: 0,
          counts: { undone: 2 },
        },
      }),
    ).toBe(false);
  });
});
