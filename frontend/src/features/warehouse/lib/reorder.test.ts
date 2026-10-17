import { describe, expect, it } from "vitest";

import { moveUuid, withSortOrder } from "@/features/warehouse/lib/reorder";

const row = (uuid: string, sort_order: number, parent = "p") => ({
  uuid,
  sort_order,
  parent,
});

describe("moveUuid (TEC-764 drag and drop)", () => {
  it("moves the dragged id to the drop position", () => {
    expect(moveUuid(["a", "b", "c", "d"], "a", "c")).toEqual([
      "b",
      "c",
      "a",
      "d",
    ]);
    expect(moveUuid(["a", "b", "c", "d"], "d", "b")).toEqual([
      "a",
      "d",
      "b",
      "c",
    ]);
  });

  it("returns null when nothing moves", () => {
    expect(moveUuid(["a", "b"], "a", "a")).toBeNull();
    expect(moveUuid(["a", "b"], "a", "x")).toBeNull();
  });
});

describe("withSortOrder (optimistic update)", () => {
  it("renumbers the moved rows and reorders them in place", () => {
    const items = [row("a", 0), row("b", 1), row("c", 2)];
    expect(withSortOrder(items, ["c", "a", "b"])).toEqual([
      row("c", 0),
      row("a", 1),
      row("b", 2),
    ]);
  });

  it("leaves rows outside the order (other parents) untouched", () => {
    const items = [
      row("x", 5, "q"),
      row("a", 0),
      row("y", 6, "q"),
      row("b", 1),
    ];
    expect(withSortOrder(items, ["b", "a"])).toEqual([
      row("x", 5, "q"),
      row("b", 0),
      row("y", 6, "q"),
      row("a", 1),
    ]);
  });

  it("does not mutate the input", () => {
    const items = [row("a", 0), row("b", 1)];
    withSortOrder(items, ["b", "a"]);
    expect(items).toEqual([row("a", 0), row("b", 1)]);
  });
});
