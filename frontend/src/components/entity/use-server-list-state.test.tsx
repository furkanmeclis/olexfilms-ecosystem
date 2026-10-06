// @vitest-environment jsdom
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createColumn } from "@/components/tables/create-column";

import {
  useServerListState,
  type UseServerListStateOptions,
} from "./use-server-list-state";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

type Row = { name: string; status: string; created_at: string };

const columns = [
  createColumn<Row>({
    accessorKey: "name",
    labelKey: "users.columns.name",
    filterVariant: "text",
    param: "name",
  }),
  createColumn<Row>({
    accessorKey: "status",
    labelKey: "users.columns.status",
    filterVariant: "faceted",
    param: "status",
  }),
  createColumn<Row>({
    accessorKey: "created_at",
    labelKey: "users.columns.created_at",
    filterVariant: "date-range",
    param: "created",
  }),
] as ColumnDef<Row, unknown>[];

type HookResult = ReturnType<typeof useServerListState<Row>>;

let container: HTMLDivElement;
let root: Root;
let result: HookResult;

function Harness({
  options,
  onResult,
}: {
  options: UseServerListStateOptions<Row>;
  onResult: (value: HookResult) => void;
}) {
  onResult(useServerListState<Row>(options));
  return null;
}

function render(options: UseServerListStateOptions<Row>) {
  act(() => {
    root.render(
      createElement(Harness, {
        options,
        onResult: (value) => {
          result = value;
        },
      }),
    );
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  window.localStorage.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.useRealTimers();
});

describe("useServerListState", () => {
  it("keeps the legacy defaults for existing callers", () => {
    render({});
    expect(result.params).toEqual({
      limit: 20,
      offset: 0,
      sort: "-created_at",
    });
    expect(result.sorting).toEqual([{ id: "created_at", desc: true }]);
  });

  it("initialSort null sends no sort; a later default (meta) is followed", () => {
    render({ initialSort: null });
    expect(result.params.sort).toBeUndefined();
    expect(result.sorting).toEqual([]);

    render({ initialSort: "name" });
    expect(result.params.sort).toBe("name");
    expect(result.sorting).toEqual([{ id: "name", desc: false }]);
  });

  it("user sort wins over a changing default and resets the page", () => {
    render({ columns, initialSort: "-created_at" });
    act(() => result.onPaginationChange({ pageIndex: 2, pageSize: 20 }));
    expect(result.params.offset).toBe(40);

    act(() => result.onSortingChange([{ id: "name", desc: true }]));
    expect(result.params.sort).toBe("-name");
    expect(result.params.offset).toBe(0);

    render({ columns, initialSort: "status" });
    expect(result.params.sort).toBe("-name");
  });

  it("maps column filters to params; selects immediate, text debounced", () => {
    render({ columns });
    act(() => result.onPaginationChange({ pageIndex: 1, pageSize: 20 }));
    act(() =>
      result.onColumnFiltersChange([
        { id: "status", value: ["active", "pending"] },
        { id: "created_at", value: ["2026-10-01", undefined] },
        { id: "name", value: "ali" },
      ]),
    );
    expect(result.params).toMatchObject({
      offset: 0,
      status: "active,pending",
      created_from: "2026-10-01",
    });
    expect(result.params.name).toBeUndefined();

    act(() => {
      vi.advanceTimersByTime(300);
    });
    expect(result.params.name).toBe("ali");
    expect(result.filterParams).toEqual({
      status: "active,pending",
      created_from: "2026-10-01",
      name: "ali",
    });
  });

  it("explicit filterParams work without columns", () => {
    render({ filterParams: { level: { param: "level", format: "csv" } } });
    act(() => result.onColumnFiltersChange([{ id: "level", value: ["a"] }]));
    expect(result.params.level).toBe("a");
  });

  it("debounces q and resets offset on search", () => {
    render({});
    act(() => result.onPaginationChange({ pageIndex: 3, pageSize: 20 }));
    act(() => result.onGlobalFilterChange(" foo "));
    expect(result.params.offset).toBe(0);
    expect(result.params.q).toBeUndefined();
    act(() => {
      vi.advanceTimersByTime(300);
    });
    expect(result.params.q).toBe("foo");
  });

  it("restores the persisted page size for persistKey", () => {
    window.localStorage.setItem(
      "list-v1",
      JSON.stringify({ pagination: { pageIndex: 4, pageSize: 50 } }),
    );
    render({ persistKey: "list-v1" });
    expect(result.params).toMatchObject({ limit: 50, offset: 0 });
  });
});
