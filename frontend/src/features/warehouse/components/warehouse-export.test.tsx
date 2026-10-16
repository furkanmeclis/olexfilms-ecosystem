// @vitest-environment jsdom
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const captured = vi.hoisted(() => ({ menus: [] as Record<string, unknown>[] }));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "tr",
    t: (key: string) => key,
    format: { number: (v: number) => String(v), dateTime: (v: string) => v },
  }),
}));
vi.mock("@/features/io/components/export-menu", () => ({
  ExportMenu: (props: Record<string, unknown>) => {
    captured.menus.push(props);
    return null;
  },
}));

import type { WarehouseScanResult } from "../services/warehouse.service";
import { ScanResultCard } from "./scan-result-card";
import { mount, render, unmount, type Mounted } from "./test-helpers";
import { listExportQuery } from "./warehouse-export";

describe("listExportQuery (TEC-761)", () => {
  it("keeps the filters, q and sort and adds the selection", () => {
    expect(
      listExportQuery({ status: "draft" }, { q: "ppf", sort: "-created_at" }, [
        "a",
        "b",
      ]),
    ).toEqual({ status: "draft", q: "ppf", sort: "-created_at", uuid: "a,b" });
    expect(listExportQuery({}, {}, []).uuid).toBeUndefined();
  });
});

describe("ScanResultCard exports (TEC-761)", () => {
  let m: Mounted;
  beforeEach(() => {
    captured.menus = [];
    m = mount();
  });
  afterEach(() => unmount(m));

  const last = () => captured.menus[captured.menus.length - 1];

  it("a unit exports its movement history", async () => {
    await render(
      m,
      createElement(ScanResultCard, {
        slug: "acme",
        result: {
          type: "unit",
          code: "OLX-1",
          matched_by: "barcode",
          unit: { uuid: "u-1", barcode: "OLX 1", status: "placed" },
        } as unknown as WarehouseScanResult,
      }),
    );
    expect(last()?.exportPath).toBe(
      "/v1/stock/units/by-barcode/OLX%201/export",
    );
    expect(last()?.jobsHref).toBe("/t/acme/exports");
  });

  it("a location exports its stock; without slug nothing", async () => {
    const result = {
      type: "location",
      code: "OFW:LOC:A-1",
      matched_by: "qr",
      location: {
        uuid: "l-1",
        full_code: "A-1",
        name: "Aisle",
        type: "aisle",
        active: true,
        path: [],
      },
    } as unknown as WarehouseScanResult;
    await render(m, createElement(ScanResultCard, { slug: "acme", result }));
    expect(last()?.exportPath).toBe("/v1/stock/locations/l-1/products/export");
    captured.menus = [];
    await render(m, createElement(ScanResultCard, { result }));
    expect(captured.menus).toHaveLength(0);
  });
});
