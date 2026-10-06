// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import { bulkUndoPath, isTenantBulkResource } from "@/features/bulk-engine";
import { BULK_PATHS } from "@/features/bulk-engine/types";
import type {
  VehicleBrand,
  VehicleModel,
} from "@/features/vehicle-catalog/services/vehicle-catalog.service";

type AnyRow = VehicleBrand | VehicleModel;

const captured = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<AnyRow>>[],
  bulkMenus: [] as Record<string, unknown>[],
  service: {
    listBrands: vi.fn(),
    getBrand: vi.fn(),
    listModels: vi.fn(),
    modelFacets: vi.fn(),
    updateBrand: vi.fn(),
    updateModel: vi.fn(),
  },
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), back: vi.fn() }),
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
vi.mock("@/providers/dialog-provider", () => ({
  useDialogs: () => ({ confirmDelete: vi.fn(async () => true) }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock(
  "@/features/vehicle-catalog/services/vehicle-catalog.service",
  async (orig) => ({
    ...(await orig<object>()),
    vehicleCatalogService: captured.service,
  }),
);
vi.mock("@/features/bulk-engine", async (orig) => ({
  ...(await orig<object>()),
  BulkActionMenu: (props: Record<string, unknown>) => {
    captured.bulkMenus.push(props);
    return null;
  },
}));
vi.mock("@/features/vehicle-catalog/components/vehicle-brand-dialog", () => ({
  VehicleBrandDialog: () => null,
}));
vi.mock("@/features/vehicle-catalog/components/vehicle-model-dialog", () => ({
  VehicleModelDialog: () => null,
}));
vi.mock("@/features/vehicle-catalog/components/vehicle-image-field", () => ({
  VehicleImageField: () => null,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: ReactNode }) => children,
  EntityCreateButton: () => null,
  EntityToolbar: () => null,
  EntityTable: (props: Partial<DataTableProps<AnyRow>>) => {
    captured.tables.push(props);
    return createElement(
      "div",
      null,
      (props as { toolbarExtra?: ReactNode }).toolbarExtra,
    );
  },
}));

import { VehicleBrandDetailPage } from "./vehicle-brand-detail-page";
import { VehicleBrandsPage } from "./vehicle-brands-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const brand: VehicleBrand = {
  uuid: "b1",
  external_id: null,
  name: "Audi",
  show_name: false,
  logo_height: null,
  has_logo: true,
  logo_url: "/brand-logos/b1",
  has_hero: false,
  hero_url: "/vehicle-heroes/default",
  model_count: 2,
  active: true,
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
} as VehicleBrand;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  captured.bulkMenus = [];
  for (const fn of Object.values(captured.service)) fn.mockReset();
  captured.service.listBrands.mockResolvedValue({
    items: [brand],
    total: 31,
    limit: 20,
    offset: 0,
  });
  captured.service.getBrand.mockResolvedValue(brand);
  captured.service.listModels.mockResolvedValue({
    items: [],
    total: 0,
    limit: 20,
    offset: 0,
  });
  captured.service.modelFacets.mockResolvedValue({
    body_type: [
      { value: "SUV", count: 3 },
      { value: "Sedan", count: 1 },
    ],
    powertrain: [{ value: "EV", count: 2 }],
  });
  captured.service.updateBrand.mockResolvedValue(brand);
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function flush() {
  for (let i = 0; i < 5; i++) {
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
const lastCall = (fn: ReturnType<typeof vi.fn>) =>
  fn.mock.calls[fn.mock.calls.length - 1];
const column = (id: string) =>
  (lastTable().columns as ColumnDef<AnyRow, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("VehicleBrandsPage", () => {
  it("maps sort and the active / has_logo filters to the list params", async () => {
    await render(createElement(VehicleBrandsPage));
    expect(lastCall(captured.service.listBrands)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "name",
    });
    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "active", value: false },
        { id: "logo", value: true },
      ]);
      lastTable().state?.onSortingChange?.([{ id: "model_count", desc: true }]);
    });
    await flush();
    expect(lastCall(captured.service.listBrands)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "-model_count",
      active: "false",
      has_logo: "true",
    });
    expect(lastTable().rowCount).toBe(31);
    expect(lastTable().renderGridItem).toBeTypeOf("function");
  });

  it("offers bulk activate / deactivate on vehicle_catalog.brands", async () => {
    await render(createElement(VehicleBrandsPage));
    const menu = captured.bulkMenus[captured.bulkMenus.length - 1];
    expect(menu?.resource).toBe("vehicle_catalog.brands");
    expect(
      (menu?.actions as { id: string }[]).map((action) => action.id),
    ).toEqual(["activate", "deactivate"]);
  });

  it("platform vehicle bulk routes and undo path", () => {
    expect(BULK_PATHS["vehicle_catalog.brands"]).toBe(
      "/v1/platform/vehicle-catalog/brands/bulk",
    );
    expect(BULK_PATHS["vehicle_catalog.models"]).toBe(
      "/v1/platform/vehicle-catalog/models/bulk",
    );
    expect(isTenantBulkResource("vehicle_catalog.models")).toBe(false);
    expect(isTenantBulkResource("catalog.categories")).toBe(true);
    expect(bulkUndoPath("vehicle_catalog.brands", "op-1")).toBe(
      "/v1/platform/bulk-operations/op-1/undo",
    );
  });
});

describe("VehicleBrandDetailPage models table", () => {
  it("uses facet options and maps body_type / powertrain / year / sort", async () => {
    await render(createElement(VehicleBrandDetailPage, { uuid: "b1" }));
    expect(captured.service.modelFacets).toHaveBeenCalledWith({
      brand_uuid: "b1",
    });
    expect(
      column("body_type")?.meta?.filterOptions?.map((o) => o.value),
    ).toEqual(["SUV", "Sedan"]);
    expect(lastCall(captured.service.listModels)?.[0]).toEqual({
      brand_uuid: "b1",
      limit: 20,
      offset: 0,
      sort: "name",
    });

    await act(async () => {
      lastTable().state?.onColumnFiltersChange?.([
        { id: "body_type", value: ["SUV", "Sedan"] },
        { id: "powertrain", value: ["EV"] },
        { id: "years", value: [2015, 2020] },
      ]);
      lastTable().state?.onSortingChange?.([{ id: "years", desc: false }]);
    });
    await flush();
    expect(lastCall(captured.service.listModels)?.[0]).toEqual({
      brand_uuid: "b1",
      limit: 20,
      offset: 0,
      sort: "year_start",
      body_type: "SUV,Sedan",
      powertrain: "EV",
      year_min: "2015",
      year_max: "2020",
    });
    const menu = captured.bulkMenus[captured.bulkMenus.length - 1];
    expect(menu?.resource).toBe("vehicle_catalog.models");
    expect(lastTable().renderGridItem).toBeTypeOf("function");
  });
});
