// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type { PlateFormat, Territory } from "@/features/geo/types";

type AnyRow = PlateFormat | Territory;

const captured = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<AnyRow>>[],
  geo: {
    platformPlateFormats: vi.fn(),
    reorderPlateFormats: vi.fn(),
    updatePlateFormat: vi.fn(),
    territories: vi.fn(),
    deleteTerritory: vi.fn(),
  },
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
vi.mock("@/components/forms", () => ({
  AppForm: () => null,
  AppCombobox: () => null,
  AppInput: () => null,
  AppSwitch: () => null,
  FormSection: () => null,
}));
vi.mock("@/features/geo/components/address-fields", () => ({
  AddressFields: () => null,
  addressIds: () => ({}),
}));
vi.mock("@/features/geo/hooks/use-geo", async (orig) => ({
  ...(await orig<object>()),
  useCountries: () => ({ data: [] }),
}));
vi.mock("@/features/geo/services/geo.service", () => ({
  geoService: captured.geo,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (props: Partial<DataTableProps<AnyRow>>) => {
    captured.tables.push(props);
    return null;
  },
}));

import { PlateFormatsPage } from "./plate-formats-page";
import { TerritoriesPage } from "./territories-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

function plate(iso2: string, sort: number): PlateFormat {
  return {
    country_iso2: iso2,
    country_name_en: iso2,
    country_name_tr: iso2,
    regex: "^[A-Z]+$",
    input_mask: "",
    example: "AB",
    country_label: iso2,
    strip_color: "#003399",
    background_color: "#FFFFFF",
    text_color: "#000000",
    is_active: true,
    sort_order: sort,
  };
}

function territory(uuid: string, patch: Partial<Territory> = {}): Territory {
  return {
    uuid,
    level: "country",
    organization_uuid: "o-1",
    organization_name: "North Dist",
    country_id: 1,
    country_iso2: "TR",
    country_name_en: "Turkey",
    country_name_tr: "Türkiye",
    created_at: "2026-10-01T00:00:00Z",
    ...patch,
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  for (const fn of Object.values(captured.geo)) fn.mockReset();
  captured.geo.platformPlateFormats.mockResolvedValue({
    items: [plate("TR", 10), plate("DE", 20), plate("BG", 30)],
  });
  captured.geo.reorderPlateFormats.mockImplementation(
    async (countries: string[]) => ({
      items: countries.map((c, i) => plate(c, (i + 1) * 10)),
    }),
  );
  captured.geo.updatePlateFormat.mockResolvedValue(plate("DE", 20));
  captured.geo.territories.mockResolvedValue({
    items: [
      territory("t-1"),
      territory("t-2", {
        level: "province",
        organization_name: "South Dist",
        country_iso2: "DE",
        country_name_en: "Germany",
        country_name_tr: "Almanya",
        province_name: "Bavaria",
      }),
    ],
  });
  captured.geo.deleteTerritory.mockResolvedValue({ deleted: true });
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
const column = (id: string) =>
  (lastTable().columns as ColumnDef<AnyRow, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("PlateFormatsPage", () => {
  it("is a client-side table with reorder and inline edit", async () => {
    await render(createElement(PlateFormatsPage));
    const table = lastTable();
    expect(table.manual).toEqual({
      sorting: false,
      filtering: false,
      pagination: false,
    });
    expect(table.features).toMatchObject({
      persistKey: "platform-plate-formats-v1",
      rowReorder: true,
      inlineEdit: true,
    });
    expect(column("is_active")?.meta).toMatchObject({
      filterVariant: "boolean",
      editVariant: "boolean",
    });
  });

  it("saves a drag-and-drop order with PUT /plate-formats/order", async () => {
    await render(createElement(PlateFormatsPage));
    const rows = lastTable().data as PlateFormat[];
    await act(async () => {
      lastTable().onRowReorder?.([rows[2], rows[0], rows[1]]);
    });
    await flush();
    expect(captured.geo.reorderPlateFormats).toHaveBeenCalledWith([
      "BG",
      "TR",
      "DE",
    ]);
    expect(
      (lastTable().data as PlateFormat[]).map((r) => r.country_iso2),
    ).toEqual(["BG", "TR", "DE"]);
  });

  it("inline-edits is_active with a PATCH", async () => {
    await render(createElement(PlateFormatsPage));
    const row = (lastTable().data as PlateFormat[])[1];
    await act(async () => {
      lastTable().onCellEdit?.({
        rowId: "DE",
        columnId: "is_active",
        value: false,
        row,
      });
    });
    await flush();
    expect(captured.geo.updatePlateFormat).toHaveBeenCalledWith("DE", {
      is_active: false,
    });
  });
});

describe("TerritoriesPage", () => {
  it("filters by distributor, level and country on the client", async () => {
    await render(createElement(TerritoriesPage));
    const table = lastTable();
    expect(table.manual).toMatchObject({ filtering: false, sorting: false });
    expect(table.features?.persistKey).toBe("platform-territories-v1");
    expect(column("level")?.meta?.filterVariant).toBe("faceted");
    expect(
      column("distributor")?.meta?.filterOptions?.map((o) => o.value),
    ).toEqual(["North Dist", "South Dist"]);
    expect(
      column("country_iso2")?.meta?.filterOptions?.map((o) => o.value),
    ).toEqual(["DE", "TR"]);
  });

  it("deletes a territory from the row action", async () => {
    await render(createElement(TerritoriesPage));
    const cell = column("actions")?.cell as (ctx: unknown) => ReactNode;
    const host = document.createElement("div");
    document.body.appendChild(host);
    const cellRoot = createRoot(host);
    await act(async () => {
      cellRoot.render(cell({ row: { original: territory("t-1") } }) as never);
    });
    await act(async () => {
      host.querySelector<HTMLElement>("button")?.click();
    });
    await flush();
    expect(captured.geo.deleteTerritory).toHaveBeenCalledWith("t-1");
    act(() => cellRoot.unmount());
    host.remove();
  });
});
