// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type { ServiceCatalogItem } from "@/features/service-catalog/services/service-catalog.service";

const captured = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<ServiceCatalogItem>>[],
  items: [] as unknown[],
  patch: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    format: { currency: (v: number | null, c: string) => `${v} ${c}` },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/features/modules/services/modules.service", () => ({
  modulesService: { platformList: () => Promise.resolve({ items: [] }) },
}));
vi.mock(
  "@/features/service-catalog/services/service-catalog.service",
  async (orig) => {
    const actual =
      await orig<
        typeof import("@/features/service-catalog/services/service-catalog.service")
      >();
    return {
      ...actual,
      serviceCatalogService: {
        ...actual.serviceCatalogService,
        listPlatform: () => Promise.resolve({ items: captured.items }),
        listVisible: () => Promise.resolve({ items: captured.items }),
        listContractTemplates: () => Promise.resolve({ items: [] }),
        patch: captured.patch,
      },
    };
  },
);
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: ReactNode }) => children,
  EntityToolbar: () => null,
  EntityTable: (props: Partial<DataTableProps<ServiceCatalogItem>>) => {
    captured.tables.push(props);
    return null;
  },
}));

import { ServiceCatalogPage } from "./service-catalog-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const item = {
  uuid: "svc-1",
  name: "Starter",
  description: "",
  category: "training",
  default_price: "100.00",
  currency: "EUR",
  recurrence: "monthly",
  cancellation_fee: "0",
  is_active: true,
  modules: [],
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  captured.items = [item];
  captured.patch.mockReset();
  captured.patch.mockResolvedValue({ ...item, is_active: false });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function render(mode: "platform" | "tenant") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(ServiceCatalogPage, { mode, slug: "acme" }),
      ),
    );
  });
  for (let i = 0; i < 4; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

// The page table is the first EntityTable (the overrides dialog is closed).
const pageTable = () =>
  captured.tables
    .filter((t) => String(t.features?.persistKey).includes("service-catalog"))
    .at(-1)!;
const column = (id: string) =>
  (pageTable().columns as ColumnDef<ServiceCatalogItem, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("ServiceCatalogPage DataTable", () => {
  it("platform: client-side table with category / recurrence / active filters", async () => {
    await render("platform");
    const table = pageTable();
    expect(table.manual).toEqual({
      sorting: false,
      filtering: false,
      pagination: false,
    });
    expect(table.data).toHaveLength(1);
    expect(table.features?.persistKey).toBe("platform-service-catalog-v1");
    expect(column("category")?.meta?.filterVariant).toBe("faceted");
    expect(
      column("recurrence")?.meta?.filterOptions?.map((o) => o.value),
    ).toEqual(["one_time", "monthly", "yearly"]);
    expect(column("is_active")?.meta?.filterVariant).toBe("boolean");
    expect(column("price")?.enableSorting).toBe(true);
    expect(column("actions")).toBeDefined();
  });

  it("platform: inline edit of is_active patches the item", async () => {
    await render("platform");
    await act(async () => {
      pageTable().onCellEdit?.({
        rowId: "svc-1",
        columnId: "is_active",
        value: false,
        row: item as ServiceCatalogItem,
      });
    });
    expect(captured.patch).toHaveBeenCalledWith("svc-1", { is_active: false });
  });

  it("tenant: read-only table without actions or active filter", async () => {
    await render("tenant");
    expect(pageTable().features?.persistKey).toBe("tenant-service-catalog-v1");
    expect(column("actions")).toBeUndefined();
    expect(column("is_active")?.meta?.filterVariant).toBeUndefined();
    expect(pageTable().onCellEdit).toBeUndefined();
  });
});
