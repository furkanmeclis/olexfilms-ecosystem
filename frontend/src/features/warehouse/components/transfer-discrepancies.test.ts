import { describe, expect, it } from "vitest";

import type { WarehouseTransfer } from "@/features/warehouse/services/warehouse.service";

import { completeBody } from "./transfer-discrepancies";

describe("TEC-1227 transfer completion body", () => {
  it("reports unticked lines as missing with their note and the surplus", () => {
    const transfer = {
      uuid: "t1",
      lines: [
        { uuid: "l1", barcode: "A" },
        { uuid: "l2", barcode: "B" },
      ],
    } as unknown as WarehouseTransfer;
    expect(
      completeBody(transfer, new Set(["l1"]), { l2: " koli yok " }, ["X"]),
    ).toEqual({
      missing: [{ line_uuid: "l2", note: "koli yok" }],
      surplus: [{ barcode: "X" }],
    });
    expect(completeBody(transfer, new Set(["l1", "l2"]), {}, [])).toEqual({
      missing: [],
      surplus: [],
    });
  });
});
