import { describe, expect, it } from "vitest";

import { warehouseLocationConfirmCopy } from "@/features/warehouse/lib/import";
import { importStatusCounts } from "@/features/io/lib/import-summary";
import en from "@/locales/en/warehouse.json";
import tr from "@/locales/tr/warehouse.json";

describe("warehouse location import confirmation", () => {
  it.each([
    [
      tr,
      "2 oda, 3 koridor, 4 raf, 5 göz oluşturulacak; 6 satır zaten var, atlanacak.",
    ],
    [
      en,
      "2 rooms, 3 aisles, 4 shelves, 5 bins will be created; 6 existing rows will be skipped.",
    ],
  ])("reports unique node counts and skipped paths", (catalog, expected) => {
    const t = (key: string, params?: Record<string, string | number>) =>
      (catalog as Record<string, string>)[
        key.replace("warehouse.", "")
      ].replace(/{{(\w+)}}/g, (_, name: string) =>
        String(params?.[name] ?? ""),
      );
    const copy = warehouseLocationConfirmCopy({
      summary: {
        total: 10,
        valid: 4,
        invalid: 0,
        rows: [],
        errors: [],
        counts: { rooms: 2, aisles: 3, shelves: 4, bins: 5, exists: 6 },
      },
      t,
      tp: () => "",
    });
    expect(copy.sentence).toBe(expected);
  });

  it("keeps node totals out of row status cards", () => {
    expect(
      importStatusCounts({
        total: 2,
        valid: 1,
        invalid: 0,
        rows: [],
        errors: [],
        counts: { new: 1, exists: 1, rooms: 1, aisles: 1, shelves: 1, bins: 1 },
      }).map((card) => card.status),
    ).toEqual(["new", "exists"]);
  });
});
