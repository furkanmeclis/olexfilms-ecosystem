import { describe, expect, it } from "vitest";

import { parseBarcodeLines } from "./stock-bulk.service";

describe("parseBarcodeLines (TEC-1008)", () => {
  it("keeps one entry per line, drops blanks and a header row", () => {
    expect(
      parseBarcodeLines("Barkod;Metre\r\nOLX1;30\n\n  OLX2  \nOLX3\t12,5\n"),
    ).toEqual(["OLX1;30", "OLX2", "OLX3\t12,5"]);
  });
});
