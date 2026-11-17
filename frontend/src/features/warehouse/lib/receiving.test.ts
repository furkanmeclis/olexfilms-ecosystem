import { describe, expect, it } from "vitest";

import {
  arrivalChecked,
  buildDecisions,
  timelineDetail,
  unknownExtras,
} from "@/features/warehouse/lib/receiving";
import type {
  StockEntry,
  StockEntryArrivalReport,
} from "@/features/warehouse/services/warehouse.service";

const line = (barcode: string, scanned = false) =>
  ({
    uuid: barcode,
    barcode,
    scanned_at: scanned ? "2026-11-17T10:00:00Z" : null,
    product: { uuid: "p", sku: "S", name: "Film" },
  }) as unknown as NonNullable<StockEntry["lines"]>[number];

const report = {
  expected: 3,
  scanned: 1,
  missing: [line("B"), line("C")],
  extras: [
    {
      barcode: "X",
      product: { uuid: "p", sku: "S", name: "Film" },
      scanned_at: "",
    },
    { barcode: "Z", product: null, scanned_at: "" },
  ],
} as unknown as StockEntryArrivalReport;

describe("TEC-1225 receiving decisions", () => {
  it("applies the arrival check to container and scanned entries", () => {
    const base = { lines: [line("A")] } as unknown as StockEntry;
    expect(arrivalChecked(base)).toBe(false);
    expect(arrivalChecked({ ...base, container_no: "MSCU1" })).toBe(true);
    expect(arrivalChecked({ ...base, lines: [line("A", true)] })).toBe(true);
    expect(arrivalChecked(base, report)).toBe(true);
  });

  it("leaves missing lines out and needs a choice for every extra", () => {
    const first = buildDecisions(report, true, {}, []);
    expect(first.pending).toEqual(["X", "Z"]);
    expect(first.decisions).toEqual([
      { barcode: "B", kind: "missing", decision: "left_out" },
      { barcode: "C", kind: "missing", decision: "left_out" },
    ]);
    const done = buildDecisions(report, true, { X: "added", Z: "rejected" }, [
      {
        barcode: "C",
        decision: "excluded",
        reason: " ezik ",
        attachmentUuids: ["ph"],
      },
    ]);
    expect(done.pending).toEqual([]);
    expect(done.decisions).toContainEqual({
      barcode: "C",
      kind: "damaged",
      decision: "excluded",
      reason: "ezik",
      attachment_uuids: ["ph"],
    });
    expect(
      done.decisions.some((d) => d.barcode === "C" && d.kind === "missing"),
    ).toBe(false);
  });

  it("needs a reason and a photo for damage", () => {
    const res = buildDecisions(report, false, { X: "added", Z: "rejected" }, [
      { barcode: "A", decision: "accepted", reason: "", attachmentUuids: [] },
    ]);
    expect(res.pending).toEqual(["A"]);
  });

  it("knows unknown extras and formats timeline details", () => {
    expect([...unknownExtras(report)]).toEqual(["Z"]);
    const t = (k: string) =>
      k === "warehouse.receiving.attachment_kind.waybill" ? "İrsaliye" : k;
    expect(timelineDetail({ barcode: "A", count: 4 }, t)).toBe("A · ×4");
    expect(timelineDetail({ kind: "waybill", filename: "x.pdf" }, t)).toBe(
      "İrsaliye · x.pdf",
    );
    expect(timelineDetail(null, t)).toBe("");
  });
});
