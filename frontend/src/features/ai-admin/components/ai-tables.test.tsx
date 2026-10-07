// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";

type Row = Record<string, unknown>;

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<Row>>,
  orgParams: [] as Record<string, unknown>[],
  usageCalls: [] as { scope: string; params: Record<string, unknown> }[],
  exportMenu: null as null | {
    exportPath?: string;
    query?: Record<string, string | undefined>;
    formats?: string[];
  },
  usageItems: [] as unknown[],
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: {
      dateTime: (v: string) => v,
      number: (v: number) => String(v),
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (
    props: Partial<DataTableProps<Row>> & { toolbarExtra?: ReactNode },
  ) => {
    captured.table = props;
    return props.toolbarExtra ?? null;
  },
}));
vi.mock("@/features/io/components/export-menu", () => ({
  ExportMenu: (props: typeof captured.exportMenu) => {
    captured.exportMenu = props;
    return null;
  },
}));
vi.mock("@/features/ai-admin/components/period-select", () => ({
  PeriodSelect: () => null,
}));
vi.mock("@/features/ai-admin/hooks/use-ai-admin", () => ({
  useAIOrgQuotas: (params: Record<string, unknown>) => {
    captured.orgParams.push(params);
    return {
      data: { items: [], total: 7, limit: 20, offset: 0 },
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    };
  },
  useUpdateAIOrgQuota: () => ({ mutate: vi.fn(), isPending: false }),
  useAIUsage: (scope: string, params: Record<string, unknown>) => {
    captured.usageCalls.push({ scope, params });
    return {
      data: {
        items: captured.usageItems,
        total: 3,
        limit: 20,
        offset: 0,
      },
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    };
  },
}));

import { OrgQuotasTable } from "@/features/ai-admin/components/org-quotas-table";
import { UsageTable } from "@/features/ai-admin/components/usage-table";
import { currentPeriod } from "@/features/ai-admin/lib/quota";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  captured.orgParams = [];
  captured.usageCalls = [];
  captured.exportMenu = null;
  captured.usageItems = [];
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function render(element: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, element));
  });
}

const column = (id: string) =>
  (captured.table?.columns as ColumnDef<Row, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

async function change(fn: () => void) {
  await act(async () => fn());
}

describe("OrgQuotasTable list params", () => {
  const last = () => captured.orgParams[captured.orgParams.length - 1];

  it("defaults to -usage of the current period, unique persistKey", async () => {
    await render(createElement(OrgQuotasTable));
    expect(last()).toEqual({
      limit: 20,
      offset: 0,
      sort: "-usage",
      period: currentPeriod(),
    });
    expect(captured.table?.rowCount).toBe(7);
    expect(captured.table?.features?.persistKey).toBe(
      "platform-ai-org-quotas-v1",
    );
  });

  it("maps column sorts to the backend whitelist", async () => {
    await render(createElement(OrgQuotasTable));
    for (const id of ["organization", "quota", "used"]) {
      expect(column(id)?.enableSorting, id).toBe(true);
    }
    for (const id of ["org_type", "percent", "request_count", "enabled"]) {
      expect(column(id)?.enableSorting, id).toBe(false);
    }
    await change(() =>
      captured.table?.state?.onSortingChange?.([
        { id: "organization", desc: false },
      ]),
    );
    expect(last().sort).toBe("name");
    await change(() =>
      captured.table?.state?.onSortingChange?.([{ id: "used", desc: true }]),
    );
    expect(last().sort).toBe("-usage");
    await change(() =>
      captured.table?.state?.onSortingChange?.([{ id: "quota", desc: false }]),
    );
    expect(last().sort).toBe("quota");
  });

  it("sends org_type as CSV and q from the search box", async () => {
    await render(createElement(OrgQuotasTable));
    await change(() =>
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "org_type", value: ["dealer", "distributor"] },
      ]),
    );
    expect(last()).toMatchObject({
      org_type: "dealer,distributor",
      offset: 0,
    });
    await change(() => captured.table?.state?.onGlobalFilterChange?.("acme"));
    expect(last().q).toBe("acme");
  });

  it('offers the "Kota düzenle" row action', async () => {
    await render(createElement(OrgQuotasTable));
    expect(column("actions")).toBeDefined();
  });
});

describe("UsageTable list params", () => {
  const last = () => captured.usageCalls[captured.usageCalls.length - 1];

  it("tenant: -created_at default, no q, own persistKey and export path", async () => {
    await render(createElement(UsageTable, { scope: "tenant", slug: "acme" }));
    expect(last()).toEqual({
      scope: "tenant",
      params: { limit: 20, offset: 0, sort: "-created_at" },
    });
    expect(captured.table?.features?.persistKey).toBe("tenant-ai-usage-v1");
    expect(captured.table?.features?.globalFilter).toBe(false);
    expect(column("organization")).toBeUndefined();
    expect(captured.exportMenu?.exportPath).toBe("/v1/ai/usage/export");
    expect(captured.exportMenu?.formats).toEqual(["xlsx", "csv"]);
  });

  it("maps date, number and faceted filters and the token sort", async () => {
    await render(createElement(UsageTable, { scope: "platform" }));
    expect(captured.table?.features?.persistKey).toBe("platform-ai-usage-v1");
    for (const id of ["created_at", "tokens"]) {
      expect(column(id)?.enableSorting, id).toBe(true);
    }
    for (const id of ["user", "channel", "purpose", "pool", "model"]) {
      expect(column(id)?.enableSorting, id).toBe(false);
    }
    await change(() =>
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "created_at", value: ["2026-10-01", "2026-10-07"] },
        { id: "tokens", value: [100, 5000] },
        { id: "channel", value: ["panel", "whatsapp"] },
        { id: "purpose", value: ["chat"] },
        { id: "pool", value: ["system"] },
        { id: "model", value: ["claude-sonnet-5-5"] },
        { id: "user", value: ["u-1", "u-2"] },
        { id: "organization", value: ["o-1"] },
      ]),
    );
    await change(() =>
      captured.table?.state?.onSortingChange?.([{ id: "tokens", desc: true }]),
    );
    const filters = {
      created_from: "2026-10-01",
      created_to: "2026-10-07",
      tokens_min: "100",
      tokens_max: "5000",
      channel: "panel,whatsapp",
      purpose: "chat",
      pool: "system",
      model: "claude-sonnet-5-5",
      user: "u-1,u-2",
      organization: "o-1",
    };
    expect(last()).toEqual({
      scope: "platform",
      params: { limit: 20, offset: 0, sort: "-tokens", ...filters },
    });
    // The export queues the same filters and sort.
    expect(captured.exportMenu?.exportPath).toBe(
      "/v1/platform/ai/usage/export",
    );
    expect(captured.exportMenu?.query).toEqual({ ...filters, sort: "-tokens" });
  });

  it("keeps user and model options from the rows seen", async () => {
    captured.usageItems = [
      {
        id: 1,
        created_at: "2026-10-07T10:00:00Z",
        organization: { uuid: "o-1", name: "Acme", type: "dealer" },
        user: { uuid: "u-9", name: "Ada", surname: "Yılmaz" },
        pool: "org",
        channel: "panel",
        purpose: "chat",
        model: "claude-haiku-4-5",
        input_tokens: 1,
        output_tokens: 1,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        tokens: 2,
      },
    ];
    await render(
      createElement(UsageTable, {
        scope: "platform",
        modelOptions: [
          { value: "claude-sonnet-5-5", label: "claude-sonnet-5-5" },
        ],
      }),
    );
    expect(column("user")?.meta?.filterOptions).toEqual([
      { value: "u-9", label: "Ada Yılmaz" },
    ]);
    expect(column("model")?.meta?.filterOptions?.map((o) => o.value)).toEqual([
      "claude-haiku-4-5",
      "claude-sonnet-5-5",
    ]);
    expect(column("organization")?.meta?.filterOptions).toEqual([
      { value: "o-1", label: "Acme" },
    ]);
  });
});
