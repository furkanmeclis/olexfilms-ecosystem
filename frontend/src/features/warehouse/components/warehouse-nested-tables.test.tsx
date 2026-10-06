// @vitest-environment jsdom
import type { ColumnDef, Row } from "@tanstack/react-table";
import { createElement, type ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const captured = vi.hoisted(() => ({
  tables: [] as Record<string, unknown>[],
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityTable: (props: Record<string, unknown>) => {
    captured.tables.push(props);
    return null;
  },
}));

import { CLIENT_SIDE_MANUAL } from "@/components/entity/client-side";

import type {
  EodReport,
  StockCountLine,
  StockCountScan,
  StockEntryLine,
} from "../services/warehouse.service";
import {
  bulkResolutionChoices,
  CountLinesTable,
  CountScansTable,
} from "./count-tables";
import { EntryLinesTable, placementFilter } from "./entry-lines-table";
import { EodProductsTable } from "./eod-tables";
import { mount, render, unmount, type Mounted } from "./test-helpers";
import { TransferLinesTable } from "./transfer-lines-table";

type Props<T> = {
  columns: ColumnDef<T, unknown>[];
  data: T[];
  manual?: unknown;
  initialState?: { sorting?: unknown };
  features?: { persistKey?: string; rowSelection?: boolean };
  state?: {
    rowSelection?: Record<string, boolean>;
    onRowSelectionChange?: (v: Record<string, boolean>) => void;
  };
  bulkActions?: {
    id: string;
    onClick: (selected: T[]) => void;
    disabled?: (selected: T[]) => boolean;
  }[];
};

let m: Mounted;
beforeEach(() => {
  captured.tables = [];
  m = mount();
});
afterEach(() => {
  unmount(m);
  vi.clearAllMocks();
});

const last = <T,>() =>
  captured.tables[captured.tables.length - 1] as unknown as Props<T>;
const ids = <T,>() =>
  last<T>().columns.map((c) =>
    "accessorKey" in c && c.accessorKey ? String(c.accessorKey) : c.id,
  );
const col = <T,>(id: string) =>
  last<T>().columns.find(
    (c) => c.id === id || ("accessorKey" in c && c.accessorKey === id),
  );

const LINE = (uuid: string, located: boolean) =>
  ({
    uuid,
    barcode: `OLEX-${uuid}`,
    quantity: 1,
    product: { uuid: "p-1", sku: "PPF-190", name: "Olex PPF 190" },
    location: located ? { uuid: "loc-1", code: "01", full_code: "A-01" } : null,
    label_url: `/v1/stock/labels/units.pdf?barcode=OLEX-${uuid}`,
  }) as unknown as StockEntryLine;

describe("EntryLinesTable (TEC-376)", () => {
  it("client-side table; selection on an editable draft feeds place", async () => {
    const onSelectedChange = vi.fn();
    await render(
      m,
      createElement(EntryLinesTable, {
        lines: [LINE("l1", false), LINE("l2", true)],
        editable: true,
        selected: ["l2"],
        onSelectedChange,
        onRemove: vi.fn(),
        removing: false,
      }),
    );
    const table = last<StockEntryLine>();
    expect(table.manual).toEqual(CLIENT_SIDE_MANUAL);
    expect(table.features).toEqual(
      expect.objectContaining({
        persistKey: "tenant-warehouse-entry-lines-v1",
        rowSelection: true,
      }),
    );
    expect(ids()).toEqual([
      "__select",
      "barcode",
      "product",
      "quantity",
      "location",
      "actions",
    ]);
    expect(table.state?.rowSelection).toEqual({ l2: true });
    table.state?.onRowSelectionChange?.({ l1: true, l2: true });
    expect(onSelectedChange).toHaveBeenCalledWith(["l1", "l2"]);

    const filterFn = col<StockEntryLine>("location")?.filterFn as (
      row: Row<StockEntryLine>,
      id: string,
      value: unknown,
    ) => boolean;
    const row = (l: StockEntryLine) => ({ original: l }) as Row<StockEntryLine>;
    expect(filterFn(row(LINE("x", true)), "location", "placed")).toBe(true);
    expect(filterFn(row(LINE("x", true)), "location", "unplaced")).toBe(false);
    expect(filterFn(row(LINE("x", false)), "location", "unplaced")).toBe(true);
  });

  it("read-only: no selection column", async () => {
    await render(
      m,
      createElement(EntryLinesTable, {
        lines: [LINE("l1", true)],
        editable: false,
        selected: [],
        onSelectedChange: vi.fn(),
        onRemove: vi.fn(),
        removing: false,
      }),
    );
    expect(ids()[0]).toBe("barcode");
    expect(last().features?.rowSelection).toBe(false);
    expect(last().state).toBeUndefined();
  });

  it("placementFilter treats an empty value as no filter", () => {
    expect(placementFilter(null, undefined)).toBe(true);
    expect(placementFilter({ uuid: "l" }, "")).toBe(true);
  });
});

describe("TransferLinesTable (TEC-376)", () => {
  it("selection only while targets can be set; remove only on a draft", async () => {
    await render(
      m,
      createElement(TransferLinesTable, {
        lines: [],
        selectable: true,
        removable: false,
        selected: [],
        onSelectedChange: vi.fn(),
        onRemove: vi.fn(),
        removing: false,
      }),
    );
    expect(ids()).toEqual([
      "__select",
      "barcode",
      "product",
      "source",
      "target",
    ]);
    expect(last().features?.persistKey).toBe(
      "tenant-warehouse-transfer-lines-v1",
    );
  });
});

const COUNT_LINE = (
  uuid: string,
  allowed: StockCountLine["allowed_resolutions"],
) =>
  ({
    uuid,
    result: "missing",
    allowed_resolutions: allowed,
    resolution: null,
    product: { uuid: "p-1", sku: "PPF-190", name: "Olex PPF 190" },
    unit: null,
    expected_location: null,
    counted_location: null,
    expected_quantity: 1,
    counted_quantity: 0,
    expected_meters: null,
    counted_meters: null,
  }) as unknown as StockCountLine;

describe("CountLinesTable (TEC-376)", () => {
  it("bulk sets one resolution on the selected lines that allow it", async () => {
    const onChoices = vi.fn();
    const lines = [
      COUNT_LINE("a", ["ignore", "void_missing"]),
      COUNT_LINE("b", ["ignore", "relocate"]),
    ];
    await render(
      m,
      createElement(CountLinesTable, {
        lines,
        canApprove: true,
        choices: {},
        onChoices,
      }),
    );
    const table = last<StockCountLine>();
    expect(table.features?.rowSelection).toBe(true);
    expect(table.bulkActions?.map((a) => a.id)).toEqual([
      "resolution-ignore",
      "resolution-relocate",
      "resolution-void_missing",
      "resolution-increase_unlocated",
    ]);
    const voidMissing = table.bulkActions![2];
    expect(voidMissing.disabled?.([lines[1]])).toBe(true);
    expect(voidMissing.disabled?.(lines)).toBe(false);
    voidMissing.onClick(lines);
    expect(onChoices).toHaveBeenCalledWith({ a: "void_missing" });
    expect(
      col("result")?.meta?.filterOptions?.map((o) => o.value),
    ).not.toContain("matched");
  });

  it("read-only: no selection, no bulk actions", async () => {
    await render(
      m,
      createElement(CountLinesTable, {
        lines: [COUNT_LINE("a", ["ignore"])],
        canApprove: false,
        choices: {},
        onChoices: vi.fn(),
      }),
    );
    expect(last().bulkActions).toBeUndefined();
    expect(ids()[0]).toBe("product");
  });

  it("bulkResolutionChoices skips lines that do not allow it", () => {
    expect(
      bulkResolutionChoices(
        [COUNT_LINE("a", ["ignore"]), COUNT_LINE("b", ["relocate"])],
        "relocate",
      ),
    ).toEqual({ b: "relocate" });
  });
});

describe("CountScansTable (TEC-376)", () => {
  it("newest first, kind filter, remove while counting", async () => {
    const onRemove = vi.fn();
    const scan = {
      uuid: "s-1",
      kind: "serial",
      raw_code: "OFW:UNIT:OLEX-1",
      unit: { uuid: "u-1", barcode: "OLEX-1" },
      product: null,
      location: null,
      quantity: 1,
      meters: null,
      created_at: "2026-10-01T09:00:00Z",
    } as unknown as StockCountScan;
    await render(
      m,
      createElement(CountScansTable, {
        scans: [scan],
        isLoading: false,
        editable: true,
        onRemove,
        removing: false,
      }),
    );
    const table = last<StockCountScan>();
    expect(table.initialState?.sorting).toEqual([
      { id: "created_at", desc: true },
    ]);
    expect(col("kind")?.meta?.filterOptions?.map((o) => o.value)).toEqual([
      "serial",
      "fixed",
      "product",
    ]);
    const cell = col<StockCountScan>("actions")?.cell as (
      ctx: unknown,
    ) => ReactElement<{ children: ReactElement<{ onClick: () => void }> }>;
    cell({ row: { original: scan } }).props.children.props.onClick();
    expect(onRemove).toHaveBeenCalledWith(scan);
  });
});

describe("EodProductsTable (TEC-376)", () => {
  it("client-side table with a group facet", async () => {
    const products = [
      {
        product_uuid: "p-1",
        type: "entry",
        group: "entry",
        sku: "PPF-190",
        product_name: "Olex PPF 190",
        movement_count: 2,
        quantity_in: 3,
        quantity_out: 1,
      },
    ] as unknown as EodReport["summary"]["products"];
    await render(m, createElement(EodProductsTable, { products }));
    expect(last().manual).toEqual(CLIENT_SIDE_MANUAL);
    expect(last().data).toBe(products);
    expect(col("group")?.meta?.filterVariant).toBe("faceted");
    const net = col<(typeof products)[number]>("net") as unknown as {
      accessorFn: (r: (typeof products)[number]) => number;
    };
    expect(net.accessorFn(products[0])).toBe(2);
  });
});
