import { describe, expect, it } from "vitest";

import type {
  StockCount,
  StockCountLine,
  WarehouseTransfer,
} from "@/features/warehouse/services/warehouse.service";

import {
  approveBody,
  canStart,
  countBody,
  countFormErrors,
  defaultResolution,
  EMPTY_COUNT_FORM,
  needsStartApproval,
  scopesFor,
} from "./counts";
import { isFutureDay, netQuantity, todayIn } from "./eod";
import { countMetersError, countQuantityError } from "./forms";
import {
  canCancel,
  canCompleteWithoutLocation,
  canPlace,
  canShip,
  untargetedLines,
} from "./transfers";

const t = (key: string) => key;

function tr(
  status: WarehouseTransfer["status"],
  targets: (string | null)[],
  toLocation = false,
): WarehouseTransfer {
  return {
    status,
    to_location: toLocation ? { uuid: "d", code: "D", full_code: "D" } : null,
    lines: targets.map((target, i) => ({
      uuid: `l-${i}`,
      target_location: target
        ? { uuid: target, code: target, full_code: target }
        : null,
    })),
  } as unknown as WarehouseTransfer;
}

describe("transfer rules (TEC-205)", () => {
  it("ship needs a draft with lines", () => {
    expect(canShip(tr("draft", []))).toBe(false);
    expect(canShip(tr("draft", [null]))).toBe(true);
    expect(canShip(tr("in_transit", [null]))).toBe(false);
  });

  it("receive without a scan needs every line targeted or a default location", () => {
    expect(canCompleteWithoutLocation(tr("in_transit", ["a", null]))).toBe(
      false,
    );
    expect(canCompleteWithoutLocation(tr("in_transit", ["a", "b"]))).toBe(true);
    expect(canCompleteWithoutLocation(tr("in_transit", [null], true))).toBe(
      true,
    );
    expect(canCompleteWithoutLocation(tr("draft", ["a"]))).toBe(false);
    expect(
      untargetedLines(tr("draft", ["a", null])).map((l) => l.uuid),
    ).toEqual(["l-1"]);
  });

  it("place and cancel only while open", () => {
    expect(canPlace(tr("draft", [null]))).toBe(true);
    expect(canPlace(tr("completed", [null]))).toBe(false);
    expect(canCancel(tr("in_transit", []))).toBe(true);
    expect(canCancel(tr("cancelled", []))).toBe(false);
  });
});

function line(over: Partial<StockCountLine>): StockCountLine {
  return {
    uuid: "l",
    result: "missing",
    resolution: null,
    allowed_resolutions: ["ignore", "void_missing"],
    ...over,
  } as StockCountLine;
}

describe("count rules (TEC-206)", () => {
  it("initial_placement has no product scope and needs a start approval", () => {
    expect(scopesFor("initial_placement")).not.toContain("product");
    expect(scopesFor("location_first")).toContain("product");
    const draft = {
      status: "draft",
      start_approval_required: true,
      start_approved_at: null,
    } as StockCount;
    expect(needsStartApproval(draft)).toBe(true);
    expect(canStart(draft)).toBe(false);
    const approved = { ...draft, start_approved_at: "2026-10-01T09:00:00Z" };
    expect(canStart(approved)).toBe(true);
  });

  it("defaults to the stored resolution, then ignore, then the first allowed", () => {
    expect(defaultResolution(line({ resolution: "void_missing" }))).toBe(
      "void_missing",
    );
    expect(defaultResolution(line({}))).toBe("ignore");
    expect(defaultResolution(line({ allowed_resolutions: ["relocate"] }))).toBe(
      "relocate",
    );
    expect(defaultResolution(line({ allowed_resolutions: [] }))).toBeNull();
  });

  it("approval body skips matched lines and reports unresolvable ones", () => {
    const lines = [
      line({ uuid: "a" }),
      line({ uuid: "b", result: "matched", allowed_resolutions: [] }),
      line({ uuid: "c", allowed_resolutions: [] }),
    ];
    expect(approveBody(lines, { a: "void_missing" })).toEqual({
      resolutions: [{ line_uuid: "a", resolution: "void_missing" }],
      missing: ["c"],
    });
    // A choice outside the allowed list is not sent.
    expect(
      approveBody([line({ uuid: "a" })], { a: "relocate" }).missing,
    ).toEqual(["a"]);
  });

  it("form errors and body follow the scope", () => {
    expect(countFormErrors(EMPTY_COUNT_FORM, t)).toEqual({
      warehouse_uuid: "warehouse.validation.warehouse",
    });
    const v = {
      ...EMPTY_COUNT_FORM,
      warehouse_uuid: "w",
      scope_type: "location" as const,
      room_uuid: "r",
    };
    expect(countFormErrors(v, t)).toEqual({
      location_uuid: "warehouse.validation.location",
    });
    expect(
      countFormErrors(
        { ...v, method: "initial_placement", scope_type: "product" },
        t,
      ).scope_type,
    ).toBe("warehouse.validation.scope");
    expect(countBody({ ...v, location_uuid: "l", note: "  " })).toEqual({
      warehouse_uuid: "w",
      method: "location_first",
      visibility: "blind",
      scope_type: "location",
      location_uuid: "l",
      note: null,
    });
  });

  it("validates the scan quantity and meters", () => {
    expect(countQuantityError("", t)).toBeNull();
    expect(countQuantityError("3", t)).toBeNull();
    expect(countQuantityError("0", t)).toBe("warehouse.validation.quantity");
    expect(countQuantityError("1.5", t)).toBe("warehouse.validation.quantity");
    expect(countMetersError("8.50", t)).toBeNull();
    expect(countMetersError("8,5", t)).toBe("warehouse.validation.meters");
  });
});

describe("end-of-day helpers (TEC-207)", () => {
  it("today follows the organization's time zone", () => {
    const late = new Date("2026-10-01T22:30:00Z");
    expect(todayIn("Europe/Istanbul", late)).toBe("2026-10-02");
    expect(todayIn("UTC", late)).toBe("2026-10-01");
  });

  it("refuses a future day and nets in/out", () => {
    expect(isFutureDay("2026-10-03", "2026-10-02")).toBe(true);
    expect(isFutureDay("2026-10-02", "2026-10-02")).toBe(false);
    expect(netQuantity({ quantity_in: 3, quantity_out: 5 })).toBe(-2);
  });
});
