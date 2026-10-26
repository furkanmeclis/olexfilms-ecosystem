import { describe, expect, it } from "vitest";

import {
  deliveryForm,
  deliveryPatch,
  supplierTemplate,
} from "@/features/warehouse/components/container-entry";
import type {
  EntryImportMappingTemplate,
  StockEntry,
} from "@/features/warehouse/services/warehouse.service";

const entry = {
  uuid: "e1",
  supplier: { uuid: "s1", name: "Guangzhou" },
  container_no: "MSKU1",
  waybill_no: null,
  arrival_date: "2026-10-20",
  default_location: null,
} as unknown as StockEntry;

describe("container entry (TEC-1007)", () => {
  it("sends only the changed delivery fields; an emptied field clears", () => {
    const before = deliveryForm(entry);
    expect(before.supplier_uuid).toBe("s1");
    const patch = deliveryPatch(before, {
      ...before,
      container_no: "",
      waybill_no: " WB-9 ",
    });
    expect(patch).toEqual({ container_no: "", waybill_no: "WB-9" });
    expect(deliveryPatch(before, before)).toEqual({});
  });

  it("picks the saved template of the entry's supplier", () => {
    const templates = [
      { uuid: "t1", name: "A", supplier: null, mapping: {} },
      {
        uuid: "t2",
        name: "B",
        supplier: { uuid: "s1", name: "G" },
        mapping: { Item: "supplier_code" },
      },
    ] as unknown as EntryImportMappingTemplate[];
    expect(supplierTemplate(templates, "s1")?.uuid).toBe("t2");
    expect(supplierTemplate(templates, "s2")).toBeUndefined();
    expect(supplierTemplate(templates, undefined)).toBeUndefined();
  });
});
