import { describe, expect, it } from "vitest";

import { Permission } from "@/config/permissions";
import {
  canConfirm,
  linesToPlace,
  unplacedLines,
} from "@/features/warehouse/lib/entries";
import {
  generateBody,
  normalizeCode,
  parseBarcodes,
  WAREHOUSE_CODE_RE,
} from "@/features/warehouse/lib/forms";
import {
  normalizeScan,
  scanKind,
  unitBarcode,
} from "@/features/warehouse/lib/scan";
import {
  visibleSections,
  WAREHOUSE_SECTIONS,
} from "@/features/warehouse/lib/sections";
import {
  allowedChildTypes,
  branchUuids,
  buildLocationTree,
  countDescendants,
  filterLocationTree,
} from "@/features/warehouse/lib/tree";
import {
  locationLabelsPath,
  type StockEntry,
  type StockEntryLine,
  type WarehouseLocation,
} from "@/features/warehouse/services/warehouse.service";

function loc(
  uuid: string,
  type: WarehouseLocation["type"],
  code: string,
  parent: string | null,
  sort = 0,
): WarehouseLocation {
  return {
    uuid,
    warehouse_uuid: "w",
    room_uuid: "r",
    parent_uuid: parent,
    type,
    code,
    full_code: `WH-R-${code}`,
    name: `Name ${code}`,
    active: true,
    sort_order: sort,
    created_at: "",
    updated_at: "",
  };
}

describe("location tree (TEC-201 rules)", () => {
  it("allows aisle/shelf at the root, shelf under aisle, bin under shelf", () => {
    expect(allowedChildTypes(null)).toEqual(["aisle", "shelf"]);
    expect(allowedChildTypes("aisle")).toEqual(["shelf"]);
    expect(allowedChildTypes("shelf")).toEqual(["bin"]);
    expect(allowedChildTypes("bin")).toEqual([]);
  });

  it("builds the tree by parent with siblings by sort order then code", () => {
    const tree = buildLocationTree([
      loc("b2", "bin", "02", "s1", 1),
      loc("b1", "bin", "01", "s1", 1),
      loc("s1", "shelf", "S1", "a1"),
      loc("a1", "aisle", "A", null, 5),
      loc("s0", "shelf", "S0", null, 1),
      loc("orphan", "bin", "X", "missing"),
    ]);
    expect(tree.map((n) => n.uuid)).toEqual(["orphan", "s0", "a1"]);
    const a1 = tree[2];
    expect(a1.children.map((n) => n.uuid)).toEqual(["s1"]);
    expect(a1.children[0].children.map((n) => n.code)).toEqual(["01", "02"]);
    expect(countDescendants(a1)).toBe(3);
    expect(branchUuids(tree)).toEqual(["a1", "s1"]);
  });

  it("filters by code, full code or name and keeps the ancestors", () => {
    const tree = buildLocationTree([
      loc("a1", "aisle", "A", null),
      loc("s1", "shelf", "S1", "a1"),
      loc("b1", "bin", "01", "s1"),
      loc("a2", "aisle", "B", null),
    ]);
    const hit = filterLocationTree(tree, "wh-r-01");
    expect(hit.map((n) => n.uuid)).toEqual(["a1"]);
    expect(hit[0].children[0].children.map((n) => n.uuid)).toEqual(["b1"]);
    expect(filterLocationTree(tree, "name b").map((n) => n.uuid)).toEqual([
      "a2",
    ]);
    expect(filterLocationTree(tree, "  ")).toBe(tree);
    expect(filterLocationTree(tree, "zzz")).toEqual([]);
  });
});

describe("forms helpers", () => {
  it("normalizes codes like the backend", () => {
    expect(normalizeCode(" ab_1 ")).toBe("AB_1");
    expect(WAREHOUSE_CODE_RE.test("AB_1")).toBe(true);
    expect(WAREHOUSE_CODE_RE.test("A-1")).toBe(false);
    expect(WAREHOUSE_CODE_RE.test("")).toBe(false);
  });

  it("parses pasted barcodes, deduplicated", () => {
    expect(parseBarcodes("A1\nA2, A1 ;A3\t")).toEqual(["A1", "A2", "A3"]);
    expect(parseBarcodes("  ")).toEqual([]);
  });

  it("sends meters only for rolls and the prefix only when set", () => {
    const v = { product_uuid: "p", quantity: "3", meters: "15", prefix: "" };
    expect(generateBody(v, false)).toEqual({ product_uuid: "p", quantity: 3 });
    expect(generateBody({ ...v, prefix: "OLX" }, true)).toEqual({
      product_uuid: "p",
      quantity: 3,
      meters: "15",
      prefix: "OLX",
    });
  });
});

describe("scan payloads (TEC-202/203)", () => {
  it("normalizes scanner output and the QR prefixes", () => {
    expect(normalizeScan(" ofw:unit:olex-1\r\n")).toBe("OFW:UNIT:olex-1");
    expect(normalizeScan("\tOLEX-00000001")).toBe("OLEX-00000001");
    expect(normalizeScan("   ")).toBe("");
  });

  it("classifies and unwraps codes", () => {
    expect(scanKind("OFW:LOC:WH-R-A")).toBe("location");
    expect(scanKind("OFW:UNIT:X")).toBe("unit");
    expect(scanKind("PPF-190")).toBe("other");
    expect(unitBarcode("OFW:UNIT:OLEX-1")).toBe("OLEX-1");
    expect(unitBarcode("OLEX-1")).toBe("OLEX-1");
  });

  it("builds the location label sheet path", () => {
    expect(locationLabelsPath({ room: "r1" })).toBe(
      "/v1/warehouse/labels/locations.pdf?room=r1",
    );
    expect(locationLabelsPath({ locations: ["a", "b"] })).toBe(
      "/v1/warehouse/labels/locations.pdf?location=a&location=b",
    );
  });
});

describe("stock entry helpers (TEC-204)", () => {
  const line = (uuid: string, placed: boolean) =>
    ({
      uuid,
      location: placed ? { uuid: "l", code: "01", full_code: "WH-R-01" } : null,
    }) as StockEntryLine;
  const entry = (
    lines: StockEntryLine[],
    status: StockEntry["status"] = "draft",
  ) => ({ status, lines }) as StockEntry;

  it("places the selection, else the unplaced lines", () => {
    const e = entry([line("1", true), line("2", false), line("3", false)]);
    expect(unplacedLines(e).map((l) => l.uuid)).toEqual(["2", "3"]);
    expect(linesToPlace(e, ["1"])).toEqual(["1"]);
    expect(linesToPlace(e, [])).toEqual(["2", "3"]);
    expect(linesToPlace(entry([line("1", true)]), [])).toEqual([]);
  });

  it("confirms a draft only when every line is placed", () => {
    expect(canConfirm(entry([]))).toBe(false);
    expect(canConfirm(entry([line("1", true), line("2", false)]))).toBe(false);
    expect(canConfirm(entry([line("1", true)]))).toBe(true);
    expect(canConfirm(entry([line("1", true)], "confirmed"))).toBe(false);
  });
});

describe("section registry (slice 1 + slice 2)", () => {
  const can = (granted: string[]) => (p: string) => granted.includes(p);

  it("links every slice-2 screen (TEC-232)", () => {
    const planned = WAREHOUSE_SECTIONS.filter((s) => s.slice === "TEC-232");
    expect(planned.map((s) => s.id)).toEqual([
      "transfers",
      "counts",
      "end_of_day",
    ]);
    expect(planned.map((s) => s.href?.("acme"))).toEqual([
      "/t/acme/warehouse/transfers",
      "/t/acme/warehouse/counts",
      "/t/acme/warehouse/end-of-day",
    ]);
    expect(
      WAREHOUSE_SECTIONS.filter((s) => s.slice === "TEC-231").every(
        (s) => s.href,
      ),
    ).toBe(true);
  });

  it("shows barcodes to the center only", () => {
    const all = [Permission.WarehouseRead, Permission.StockRead];
    expect(visibleSections(can(all), "center").map((s) => s.id)).toContain(
      "barcodes",
    );
    expect(
      visibleSections(can(all), "distributor").map((s) => s.id),
    ).not.toContain("barcodes");
    expect(visibleSections(can(all), "dealer")).toEqual([]);
    expect(visibleSections(can([]), "center")).toEqual([]);
  });
});
