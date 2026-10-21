import { describe, expect, it } from "vitest";

import {
  addLine,
  buildCreateBody,
  lineExceedsMax,
  lineIndexOfField,
  lineQuantityValid,
  toggleUnitLine,
  transferErrorMessage,
  transferStatusTone,
  transitionAsksReason,
  transitionButtons,
  validQuantity,
} from "@/features/transfers/lib/transfers";
import type { StockTransfer } from "@/features/transfers/services/transfers.service";
import { ApiError } from "@/lib/api/errors";

const t = (key: string) =>
  (
    ({
      "transfers.errors.TRANSFER_NOT_SIBLING": "not sibling",
      "transfers.errors.TRANSFER_UNIT_RESERVED": "reserved",
    }) as Record<string, string>
  )[key] ?? key;

describe("picked stock units (TEC-870)", () => {
  const serial = {
    barcode: "R-1",
    unit_kind: "serial" as const,
    quantity: 1,
    remaining_meters: "12.50",
    product: { name: "Roll film" },
  };
  const fixed = {
    barcode: "F-1",
    unit_kind: "fixed" as const,
    quantity: 5,
    remaining_meters: null,
    product: { name: "Clip" },
  };

  it("adds a serial unit whole and a fixed barcode at 1 capped by its stock", () => {
    const lines = toggleUnitLine(toggleUnitLine([], serial), fixed);
    expect(lines).toEqual([
      {
        barcode: "R-1",
        quantity: "",
        fixed: false,
        meters: "12.50",
        label: "Roll film",
      },
      { barcode: "F-1", quantity: "1", fixed: true, max: 5, label: "Clip" },
    ]);
    expect(buildCreateBody("org", lines, "")).toEqual({
      to_org_uuid: "org",
      items: [{ barcode: "R-1" }, { barcode: "F-1", quantity: 1 }],
    });
  });

  it("toggles a barcode off, also a scanned one, never twice", () => {
    let lines = toggleUnitLine([], fixed);
    lines = toggleUnitLine(lines, fixed);
    expect(lines).toEqual([]);
    lines = addLine([], "F-1");
    expect(toggleUnitLine(lines, fixed)).toEqual([]);
    lines = addLine(toggleUnitLine([], fixed), " F-1 ");
    expect(lines).toHaveLength(1);
  });

  it("validates picked quantities", () => {
    const f = { barcode: "F", quantity: "5", fixed: true, max: 5 };
    expect(lineQuantityValid(f)).toBe(true);
    expect(lineQuantityValid({ ...f, quantity: "6" })).toBe(false);
    expect(lineExceedsMax({ ...f, quantity: "6" })).toBe(true);
    expect(lineQuantityValid({ ...f, quantity: "" })).toBe(false);
    expect(lineExceedsMax({ ...f, quantity: "" })).toBe(false);
    expect(lineQuantityValid({ ...f, quantity: "0" })).toBe(false);
    expect(
      lineQuantityValid({ barcode: "S", quantity: "", fixed: false }),
    ).toBe(true);
    expect(
      lineQuantityValid({ barcode: "S", quantity: "2", fixed: false }),
    ).toBe(false);
    expect(buildCreateBody("org", [{ ...f, quantity: "6" }], "")).toBeNull();
  });
});

describe("transfer lines (TEC-197)", () => {
  it("adds trimmed barcodes once", () => {
    let lines = addLine([], "  B-1 ");
    lines = addLine(lines, "B-1");
    lines = addLine(lines, " ");
    lines = addLine(lines, "B-2");
    expect(lines.map((l) => l.barcode)).toEqual(["B-1", "B-2"]);
  });

  it("validates quantities", () => {
    expect(validQuantity("")).toBe(true);
    expect(validQuantity("3")).toBe(true);
    expect(validQuantity("0")).toBe(false);
    expect(validQuantity("-1")).toBe(false);
    expect(validQuantity("1.5")).toBe(false);
  });

  it("builds the request body", () => {
    expect(
      buildCreateBody("", [{ barcode: "B", quantity: "" }], ""),
    ).toBeNull();
    expect(buildCreateBody("org", [], "")).toBeNull();
    expect(
      buildCreateBody("org", [{ barcode: "B", quantity: "x" }], ""),
    ).toBeNull();
    expect(
      buildCreateBody(
        "org",
        [
          { barcode: "B", quantity: "" },
          { barcode: "F", quantity: " 4 " },
        ],
        "  hi ",
      ),
    ).toEqual({
      to_org_uuid: "org",
      note: "hi",
      items: [{ barcode: "B" }, { barcode: "F", quantity: 4 }],
    });
  });

  it("maps an items[i] field to the line", () => {
    expect(lineIndexOfField("items[2].barcode")).toBe(2);
    expect(lineIndexOfField("to_org_uuid")).toBe(-1);
    expect(lineIndexOfField(undefined)).toBe(-1);
  });
});

describe("transfer status helpers", () => {
  it("orders buttons by flow from available_transitions", () => {
    const r = {
      available_transitions: ["rejected", "approved"],
    } as unknown as StockTransfer;
    expect(transitionButtons(r)).toEqual(["approved", "rejected"]);
    expect(
      transitionButtons({
        available_transitions: [],
      } as unknown as StockTransfer),
    ).toEqual([]);
  });

  it("asks a reason for reject and cancel only", () => {
    expect(transitionAsksReason("rejected")).toBe(true);
    expect(transitionAsksReason("cancelled")).toBe(true);
    expect(transitionAsksReason("shipped")).toBe(false);
  });

  it("colors statuses", () => {
    expect(transferStatusTone("received")).toBe("success");
    expect(transferStatusTone("shipped")).toBe("warning");
    expect(transferStatusTone("rejected")).toBe("danger");
    expect(transferStatusTone("requested")).toBe("default");
  });
});

describe("transferErrorMessage", () => {
  it("prefers the detail code, then the error code", () => {
    const detail = new ApiError({
      status: 400,
      code: "VALIDATION_ERROR",
      message: "x",
      details: [
        {
          field: "items[0].barcode",
          code: "TRANSFER_UNIT_RESERVED",
          message: "m",
        },
      ],
    });
    expect(transferErrorMessage(detail, t, "fallback")).toBe("reserved");
    const code = new ApiError({
      status: 422,
      code: "TRANSFER_NOT_SIBLING",
      message: "x",
    });
    expect(transferErrorMessage(code, t, "fallback")).toBe("not sibling");
    expect(transferErrorMessage(new Error("boom"), t, "fallback")).toBe(
      "fallback",
    );
  });
});
