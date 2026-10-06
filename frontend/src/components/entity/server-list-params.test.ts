import type { ColumnDef } from "@tanstack/react-table";
import { describe, expect, it } from "vitest";

import { createColumn } from "@/components/tables/create-column";

import {
  columnFiltersToParams,
  filterParamSpecsFromColumns,
  filterValueToParams,
  sortFieldsFromColumns,
  sortParamToSorting,
  sortingToSortParam,
} from "./server-list-params";

type Row = {
  name: string;
  status: string;
  active: boolean;
  created_at: string;
  amount: number;
};

const columns = [
  createColumn<Row>({
    accessorKey: "name",
    labelKey: "users.columns.name",
    filterVariant: "text",
    param: "name",
    sortParam: "surname",
  }),
  createColumn<Row>({
    accessorKey: "status",
    labelKey: "users.columns.status",
    filterVariant: "faceted",
    param: "status",
  }),
  createColumn<Row>({
    id: "kind",
    labelKey: "users.columns.role",
    filterVariant: "select",
    param: "kind",
  }),
  createColumn<Row>({
    id: "tags",
    labelKey: "users.columns.roles",
    filterVariant: "multi-select",
    param: "tag",
  }),
  createColumn<Row>({
    accessorKey: "active",
    labelKey: "users.columns.auth_methods",
    filterVariant: "boolean",
    param: "active",
  }),
  createColumn<Row>({
    accessorKey: "created_at",
    labelKey: "users.columns.created_at",
    filterVariant: "date-range",
    param: "created",
  }),
  createColumn<Row>({
    accessorKey: "amount",
    labelKey: "users.columns.uuid",
    filterVariant: "number-range",
    param: "amount",
  }),
  // Filterable but not mapped → never sent (legacy pages map by hand).
  createColumn<Row>({
    id: "unmapped",
    labelKey: "users.columns.actions",
    filterVariant: "text",
  }),
] as ColumnDef<Row, unknown>[];

const specs = filterParamSpecsFromColumns(columns);

describe("filterParamSpecsFromColumns", () => {
  it("collects only columns with meta.param", () => {
    expect(Object.keys(specs).sort()).toEqual(
      [
        "active",
        "amount",
        "created_at",
        "kind",
        "name",
        "status",
        "tags",
      ].sort(),
    );
    expect(specs.created_at).toMatchObject({
      param: "created",
      filterVariant: "date-range",
    });
  });
});

describe("columnFiltersToParams", () => {
  it("text → trimmed string, empty omitted", () => {
    expect(
      columnFiltersToParams([{ id: "name", value: "  ayşe " }], specs),
    ).toEqual({ name: "ayşe" });
    expect(columnFiltersToParams([{ id: "name", value: "  " }], specs)).toEqual(
      {},
    );
  });

  it("select → single value", () => {
    expect(columnFiltersToParams([{ id: "kind", value: "a" }], specs)).toEqual({
      kind: "a",
    });
  });

  it("faceted and multi-select → CSV (single value too)", () => {
    expect(
      columnFiltersToParams(
        [
          { id: "status", value: ["active", "disabled"] },
          { id: "tags", value: ["x"] },
        ],
        specs,
      ),
    ).toEqual({ status: "active,disabled", tag: "x" });
    expect(columnFiltersToParams([{ id: "status", value: [] }], specs)).toEqual(
      {},
    );
  });

  it("boolean → true|false", () => {
    expect(
      columnFiltersToParams([{ id: "active", value: true }], specs),
    ).toEqual({ active: "true" });
    expect(
      columnFiltersToParams([{ id: "active", value: false }], specs),
    ).toEqual({ active: "false" });
  });

  it("date-range → <param>_from / <param>_to (open ends omitted)", () => {
    expect(
      columnFiltersToParams(
        [{ id: "created_at", value: ["2026-10-01", "2026-10-06"] }],
        specs,
      ),
    ).toEqual({ created_from: "2026-10-01", created_to: "2026-10-06" });
    expect(
      columnFiltersToParams(
        [{ id: "created_at", value: [undefined, "2026-10-06"] }],
        specs,
      ),
    ).toEqual({ created_to: "2026-10-06" });
  });

  it("number-range → <param>_min / <param>_max (0 kept)", () => {
    expect(
      columnFiltersToParams([{ id: "amount", value: [0, 250] }], specs),
    ).toEqual({ amount_min: "0", amount_max: "250" });
    expect(
      columnFiltersToParams([{ id: "amount", value: [10, undefined] }], specs),
    ).toEqual({ amount_min: "10" });
  });

  it("skips unmapped columns", () => {
    expect(
      columnFiltersToParams([{ id: "unmapped", value: "x" }], specs),
    ).toEqual({});
  });

  it("honours paramFormat overrides and custom functions", () => {
    expect(
      filterValueToParams(
        { param: "status", format: "string", filterVariant: "faceted" },
        ["a", "b"],
      ),
    ).toEqual({ status: "a" });
    expect(
      filterValueToParams(
        {
          param: "period",
          format: (value) => ({
            date_from: (value as string[])[0],
            date_to: undefined,
          }),
        },
        ["2026-01-01"],
      ),
    ).toEqual({ date_from: "2026-01-01" });
  });
});

describe("sort param", () => {
  const sortFields = sortFieldsFromColumns(columns);

  it("emits a single field, '-' for desc, mapped via meta.sortParam", () => {
    expect(sortingToSortParam([{ id: "status", desc: false }])).toBe("status");
    expect(
      sortingToSortParam(
        [
          { id: "name", desc: true },
          { id: "status", desc: false },
        ],
        sortFields,
      ),
    ).toBe("-surname");
    expect(sortingToSortParam([])).toBeUndefined();
  });

  it("parses field / -field back to the column id", () => {
    expect(sortParamToSorting("-surname", sortFields)).toEqual([
      { id: "name", desc: true },
    ]);
    expect(sortParamToSorting("created_at,-id")).toEqual([
      { id: "created_at", desc: false },
    ]);
    expect(sortParamToSorting(null)).toEqual([]);
  });
});
