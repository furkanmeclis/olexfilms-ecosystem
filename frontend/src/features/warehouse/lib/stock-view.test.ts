import { describe, expect, it } from "vitest";

import {
  openLines,
  partialApproveBody,
  approveBody,
} from "@/features/warehouse/lib/counts";
import {
  reorderInput,
  reorderLevelTone,
} from "@/features/warehouse/lib/stock-view";
import type { StockCountLine } from "@/features/warehouse/services/warehouse.service";

const form = {
  warehouse_uuid: "w",
  product_uuid: "p",
  min_quantity: "2",
  reorder_point: "5",
  max_quantity: "10",
  warning_days: "",
  cover_days: "",
};

describe("reorderInput (TEC-1229)", () => {
  it("accepts min <= point <= max", () => {
    expect(reorderInput(form)).toEqual({
      ok: true,
      value: {
        warehouse_uuid: "w",
        product_uuid: "p",
        min_quantity: 2,
        reorder_point: 5,
        max_quantity: 10,
        warning_days: null,
        cover_days: null,
      },
    });
  });
  it("refuses inconsistent or empty thresholds", () => {
    expect(reorderInput({ ...form, reorder_point: "1" })).toMatchObject({
      ok: false,
      error: "order",
    });
    expect(reorderInput({ ...form, max_quantity: "3" })).toMatchObject({
      ok: false,
      error: "order",
    });
    expect(reorderInput({ ...form, min_quantity: "x" })).toMatchObject({
      ok: false,
      error: "number",
    });
    expect(reorderInput({ ...form, warning_days: "0" })).toMatchObject({
      ok: false,
      error: "days",
    });
    expect(
      reorderInput({
        ...form,
        min_quantity: "",
        reorder_point: "",
        max_quantity: "",
      }),
    ).toMatchObject({ ok: false, error: "empty" });
    expect(reorderInput({ ...form, product_uuid: "" })).toMatchObject({
      ok: false,
      error: "required",
    });
  });
  it("tones the levels", () => {
    expect(reorderLevelTone("below_min")).toBe("danger");
    expect(reorderLevelTone("ok")).toBe("success");
  });
});

function line(over: Partial<StockCountLine>): StockCountLine {
  return {
    uuid: "l",
    line_kind: "fixed",
    unit: null,
    product: { uuid: "p", sku: "S", name: "N" },
    expected_location: null,
    counted_location: null,
    expected_quantity: 5,
    counted_quantity: 4,
    expected_meters: null,
    counted_meters: null,
    result: "qty_variance",
    resolution: null,
    allowed_resolutions: ["ignore", "increase_unlocated"],
    note: null,
    resolved_at: null,
    recount_status: null,
    recount_rounds: 0,
    recounted_at: null,
    first_count: null,
    ...over,
  };
}

describe("recount and partial approval (TEC-1229)", () => {
  const lines = [
    line({ uuid: "a" }),
    line({ uuid: "b", recount_status: "requested", allowed_resolutions: [] }),
    line({
      uuid: "c",
      resolved_at: "2026-11-17T10:00:00Z",
      resolution: "ignore",
      allowed_resolutions: [],
    }),
  ];
  it("keeps resolved lines out and blocks the full approval while a recount waits", () => {
    expect(openLines(lines).map((l) => l.uuid)).toEqual(["a", "b"]);
    expect(approveBody(lines, {}).missing).toEqual(["b"]);
  });
  it("partially approves only the selected open lines", () => {
    expect(partialApproveBody(lines, {}, ["a", "b", "c"])).toEqual([
      { line_uuid: "a", resolution: "ignore" },
    ]);
  });
});
