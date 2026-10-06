// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type { DealerModuleRow, PlatformModule } from "@/features/modules/types";

type AnyRow = DealerModuleRow | PlatformModule;

const captured = vi.hoisted(() => ({
  tables: [] as (Partial<DataTableProps<AnyRow>> & {
    toolbarExtra?: ReactNode;
  })[],
  dealers: vi.fn(),
  bulk: vi.fn(),
  list: vi.fn(),
  platformList: vi.fn(),
  platformPatch: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: { dateTime: (v: string) => v },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({
    uuid: "org-1",
    slug: "dist",
    type: "distributor",
  }),
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (
    props: Partial<DataTableProps<AnyRow>> & { toolbarExtra?: ReactNode },
  ) => {
    captured.tables.push(props);
    return props.toolbarExtra ?? null;
  },
}));
vi.mock("@/features/modules/services/modules.service", () => ({
  modulesService: {
    list: captured.list,
    dealers: captured.dealers,
    bulk: captured.bulk,
    platformList: captured.platformList,
    platformPatch: captured.platformPatch,
  },
}));

import { DealerModules } from "./features-page";
import { PlatformModulesPage } from "./platform-modules-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  for (const fn of [
    captured.dealers,
    captured.bulk,
    captured.list,
    captured.platformList,
    captured.platformPatch,
  ])
    fn.mockReset();
  captured.list.mockResolvedValue({
    organization_type: "distributor",
    enabled: ["tasks"],
    items: [
      { key: "organizations", level: "core", enabled: true },
      { key: "tasks", level: "standard", enabled: true },
    ],
  });
  captured.dealers.mockResolvedValue({
    items: [],
    total: 75,
    limit: 20,
    offset: 0,
  });
  captured.bulk.mockResolvedValue({ updated: 2 });
  captured.platformList.mockResolvedValue({
    items: [
      {
        key: "tasks",
        level: "standard",
        enabled: true,
        default_enabled: false,
        paid: false,
      },
    ],
  });
  captured.platformPatch.mockResolvedValue({});
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
const lastDealersCall = () =>
  captured.dealers.mock.calls[captured.dealers.mock.calls.length - 1][0];
const column = (id: string) =>
  (lastTable().columns as ColumnDef<AnyRow, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("DealerModules (server DataTable)", () => {
  it("pages, sorts by name and passes the total", async () => {
    await render(
      createElement(DealerModules, { slug: "dist", orgUuid: "org-1" }),
    );
    expect(lastDealersCall()).toEqual({ limit: 20, offset: 0, sort: "name" });
    expect(lastTable().rowCount).toBe(75);
    await act(async () => {
      lastTable().state?.onSortingChange?.([{ id: "slug", desc: true }]);
      lastTable().state?.onGlobalFilterChange?.("izmir");
    });
    await flush();
    expect(lastDealersCall()).toMatchObject({ sort: "-slug", q: "izmir" });
  });

  it("sends state/source only together with the selected module", async () => {
    await render(
      createElement(DealerModules, { slug: "dist", orgUuid: "org-1" }),
    );
    expect(column("state")?.meta?.param).toBe("state");
    expect(column("source")?.meta?.param).toBe("source");
    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "state", value: ["disabled"] },
        { id: "source", value: ["standard", "admin"] },
      ]);
    });
    await flush();
    expect(lastDealersCall()).toEqual({
      limit: 20,
      offset: 0,
      sort: "name",
      state: "disabled",
      source: "standard,admin",
      module: "tasks",
    });
  });

  it("bulk-disables the selected dealers for the module", async () => {
    await render(
      createElement(DealerModules, { slug: "dist", orgUuid: "org-1" }),
    );
    await act(async () => {
      lastTable().state?.onRowSelectionChange?.({ "d-1": true, "d-2": true });
    });
    const disable = [...container.querySelectorAll("button")].find(
      (b) => b.textContent === "modules.dealers.disable",
    );
    await act(async () => {
      disable?.click();
    });
    await flush();
    expect(captured.bulk).toHaveBeenCalledWith(["d-1", "d-2"], "tasks", false);
  });
});

describe("PlatformModulesPage (client DataTable)", () => {
  it("is client-side with level facet and boolean filters", async () => {
    await render(createElement(PlatformModulesPage));
    const table = lastTable();
    expect(table.manual).toEqual({
      sorting: false,
      filtering: false,
      pagination: false,
    });
    expect(table.features?.persistKey).toBe("platform-modules-v1");
    expect(column("level")?.meta?.filterVariant).toBe("faceted");
    for (const id of ["enabled", "default_enabled", "paid"])
      expect(column(id)?.meta?.filterVariant, id).toBe("boolean");
  });

  it("inline switch PATCHes the module", async () => {
    await render(createElement(PlatformModulesPage));
    const cell = column("paid")?.cell as (ctx: unknown) => ReactNode;
    const host = document.createElement("div");
    document.body.appendChild(host);
    const cellRoot = createRoot(host);
    await act(async () => {
      cellRoot.render(
        cell({
          row: {
            original: {
              key: "tasks",
              level: "standard",
              enabled: true,
              default_enabled: false,
              paid: false,
            },
          },
        }) as never,
      );
    });
    await act(async () => {
      host.querySelector<HTMLElement>("[role='switch']")?.click();
    });
    await flush();
    expect(captured.platformPatch).toHaveBeenCalledWith("tasks", {
      paid: true,
    });
    act(() => cellRoot.unmount());
    host.remove();
  });
});
