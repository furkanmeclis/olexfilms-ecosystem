// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";

const captured = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<{ uuid: string }>>[],
  syncRunsPage: vi.fn(),
  outboundsPage: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key }),
  useFormatter: () => ({ dateTime: (v: string) => v }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (props: Partial<DataTableProps<{ uuid: string }>>) => {
    captured.tables.push(props);
    return null;
  },
}));
vi.mock(
  "@/features/integrations/glorian/services/glorian.service",
  async (orig) => ({
    ...(await orig<object>()),
    glorianService: {
      syncRunsPage: captured.syncRunsPage,
      outboundsPage: captured.outboundsPage,
    },
  }),
);

import { GlorianOutbounds } from "./glorian-outbounds";
import { GlorianSyncRuns } from "./glorian-sync-runs";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  captured.syncRunsPage.mockReset();
  captured.outboundsPage.mockReset();
  const page = { items: [], total: 310, limit: 20, offset: 0 };
  captured.syncRunsPage.mockResolvedValue(page);
  captured.outboundsPage.mockResolvedValue(page);
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function flush() {
  for (let i = 0; i < 4; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render(node: ReturnType<typeof createElement>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await flush();
}

const lastTable = () => captured.tables[captured.tables.length - 1];
const lastArg = (fn: ReturnType<typeof vi.fn>) =>
  fn.mock.calls[fn.mock.calls.length - 1][0];

describe("GlorianSyncRuns params", () => {
  it("maps kind/status CSV, started range, sort and paging", async () => {
    await render(createElement(GlorianSyncRuns, { canManage: false }));
    expect(lastTable().rowCount).toBe(310);
    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "kind", value: ["pull_stock", "reconcile"] },
        { id: "status", value: ["failed"] },
        { id: "started_at", value: ["2026-10-01", "2026-10-02"] },
      ]);
      lastTable().state?.onSortingChange?.([
        { id: "finished_at", desc: false },
      ]);
    });
    await flush();
    await act(async () => {
      lastTable().state?.onPaginationChange?.({ pageIndex: 1, pageSize: 20 });
    });
    await flush();
    expect(lastArg(captured.syncRunsPage)).toEqual({
      kind: "pull_stock,reconcile",
      status: "failed",
      started_from: "2026-10-01",
      started_to: "2026-10-02",
      sort: "finished_at",
      limit: 20,
      offset: 20,
    });
  });
});

describe("GlorianOutbounds params", () => {
  it("maps state CSV, q, updated range and sort", async () => {
    await render(createElement(GlorianOutbounds, { canManage: true }));
    expect(lastArg(captured.outboundsPage)).toEqual({
      state: "held",
      limit: 20,
      offset: 0,
    });
    expect(lastTable().features?.rowSelection).toBe(true);
    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "state", value: ["held", "failed"] },
        { id: "updated_at", value: [undefined, "2026-10-05"] },
      ]);
      lastTable().state?.onGlobalFilterChange?.("ORD-77");
      lastTable().state?.onSortingChange?.([{ id: "attempts", desc: true }]);
    });
    await flush();
    expect(lastArg(captured.outboundsPage)).toEqual({
      state: "held,failed",
      updated_to: "2026-10-05",
      q: "ORD-77",
      sort: "-attempts",
      limit: 20,
      offset: 0,
    });
  });

  it("lists every state once the facet is cleared", async () => {
    await render(createElement(GlorianOutbounds, { canManage: false }));
    expect(lastTable().features?.rowSelection).toBe(false);
    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([]);
    });
    await flush();
    expect(lastArg(captured.outboundsPage)).toEqual({ limit: 20, offset: 0 });
  });
});
