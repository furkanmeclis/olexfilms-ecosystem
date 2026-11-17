import { describe, expect, it } from "vitest";

import {
  adjustmentStatusTone,
  canCancelAdjustment,
  isDraft,
  needsPhoto,
  normalizeMeters,
} from "@/features/warehouse/lib/adjustments";

describe("stock adjustments (TEC-1226)", () => {
  it("normalizes typed meters", () => {
    expect(normalizeMeters("2,5")).toBe("2.5");
    expect(normalizeMeters(" 12.25 ")).toBe("12.25");
    expect(normalizeMeters("0")).toBeUndefined();
    expect(normalizeMeters("1.234")).toBeUndefined();
    expect(normalizeMeters("abc")).toBeUndefined();
    expect(normalizeMeters("")).toBeUndefined();
  });

  it("a damage document needs a photo", () => {
    expect(needsPhoto({ type: "damage", photos: [] })).toBe(true);
    expect(needsPhoto({ type: "damage", photos: [{}] })).toBe(false);
    expect(needsPhoto({ type: "loss", photos: [] })).toBe(false);
  });

  it("status helpers", () => {
    expect(isDraft({ status: "draft" })).toBe(true);
    expect(isDraft({ status: "pending_approval" })).toBe(false);
    expect(canCancelAdjustment({ status: "pending_approval" })).toBe(true);
    expect(canCancelAdjustment({ status: "posted" })).toBe(false);
    expect(adjustmentStatusTone("posted")).toBe("success");
    expect(adjustmentStatusTone("pending_approval")).toBe("warning");
  });
});
