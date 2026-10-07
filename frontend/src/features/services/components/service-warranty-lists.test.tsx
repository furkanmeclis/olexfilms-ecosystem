// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, Fragment, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";

type AnyRow = Record<string, unknown> & { uuid: string };
type RowAction = { id: string; onSelect: () => void };

const captured = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<AnyRow>>[],
  exports: [] as Record<string, unknown>[],
  combos: [] as Record<string, unknown>[],
  voids: [] as Record<string, unknown>[],
  rowActions: [] as RowAction[][],
  grants: new Set<string>(),
  orgType: "center" as string | null,
  push: vi.fn(),
  services: { listServices: vi.fn() },
  warranties: { list: vi.fn() },
  customers: { listOrganizations: vi.fn() },
  catalog: { listProducts: vi.fn() },
  servicePdf: vi.fn(),
  certificate: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: captured.push, back: vi.fn() }),
}));
vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: ReactNode }) =>
    createElement("a", { href }, children),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: {
      number: (v: number) => String(v),
      date: (v: string) => v,
      dateTime: (v: string) => v,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => captured.grants.has(p) }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () =>
    captured.orgType ? { slug: "acme", type: captured.orgType } : null,
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock(
  "@/features/services/services/service-wizard.service",
  async (orig) => ({
    ...(await orig<object>()),
    serviceWizardService: captured.services,
  }),
);
vi.mock("@/features/warranty/services/warranty.service", async (orig) => ({
  ...(await orig<object>()),
  warrantyService: captured.warranties,
}));
vi.mock("@/features/customers/services/customers.service", async (orig) => ({
  ...(await orig<object>()),
  customersService: captured.customers,
}));
vi.mock("@/features/catalog/services/catalog.service", () => ({
  catalogService: captured.catalog,
}));
vi.mock("@/features/services/components/service-pdf-button", () => ({
  downloadServicePdf: captured.servicePdf,
}));
vi.mock("@/features/warranty/components/warranty-certificate-button", () => ({
  downloadWarrantyCertificate: captured.certificate,
}));
vi.mock("@/features/warranty/components/warranty-detail-page", () => ({
  VoidDialog: (props: Record<string, unknown>) => {
    captured.voids.push(props);
    return null;
  },
}));
vi.mock("@/features/warranty/components/warranty-progress", () => ({
  WarrantyProgressBar: () => null,
}));
vi.mock("@/components/ui/async-combobox", () => ({
  AsyncCombobox: (props: Record<string, unknown>) => {
    captured.combos.push(props);
    return null;
  },
}));
vi.mock("@/features/io/components/export-menu", () => ({
  ExportMenu: (props: Record<string, unknown>) => {
    captured.exports.push(props);
    return null;
  },
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({
    children,
    actions,
    permission,
    forbiddenFallback,
  }: {
    children: ReactNode;
    actions?: ReactNode;
    permission?: string;
    forbiddenFallback?: ReactNode;
  }) =>
    permission && !captured.grants.has(permission)
      ? createElement(Fragment, null, forbiddenFallback)
      : createElement(Fragment, null, actions, children),
  EntityCreateButton: ({ label }: { label: string }) =>
    createElement("button", { "data-testid": "create" }, label),
  EntityToolbar: () => null,
  EntityRowActions: ({ actions }: { actions: RowAction[] }) => {
    captured.rowActions.push(actions);
    return null;
  },
  // Renders the toolbar and the actions cell of every row.
  EntityTable: (props: Partial<DataTableProps<AnyRow>>) => {
    captured.tables.push(props);
    const actions = (props.columns ?? []).find((c) => c.id === "actions");
    return createElement(
      "div",
      null,
      (props as { toolbarExtra?: ReactNode }).toolbarExtra,
      (props.data ?? []).map((row) =>
        createElement(
          Fragment,
          { key: row.uuid },
          typeof actions?.cell === "function"
            ? (actions.cell as (ctx: unknown) => ReactNode)({
                row: { original: row },
              })
            : null,
        ),
      ),
    );
  },
}));

import { Permission } from "@/config/permissions";
import { WarrantiesListPage } from "@/features/warranty/components/warranties-list-page";

import {
  ServiceItemsTable,
  ServiceWarrantiesTable,
} from "./service-detail-tables";
import { ServicesListPage } from "./services-list-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

const page = (items: unknown[] = [], total = items.length) => ({
  items,
  total,
  limit: 20,
  offset: 0,
});

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  captured.exports = [];
  captured.combos = [];
  captured.voids = [];
  captured.rowActions = [];
  captured.grants = new Set();
  captured.orgType = "center";
  captured.push.mockReset();
  captured.servicePdf.mockReset();
  captured.certificate.mockReset();
  captured.services.listServices.mockReset().mockResolvedValue(page([], 45));
  captured.warranties.list.mockReset().mockResolvedValue(page([], 12));
  captured.customers.listOrganizations.mockReset().mockResolvedValue([
    { uuid: "o1", name: "Olex Bayi", type: "dealer" },
    { uuid: "o2", name: "Olex Dist", type: "distributor" },
  ]);
  captured.catalog.listProducts.mockReset().mockResolvedValue(page([]));
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

const lastTable = () => captured.tables[captured.tables.length - 1]!;
const lastCall = (fn: ReturnType<typeof vi.fn>) =>
  fn.mock.calls[fn.mock.calls.length - 1];
const column = (id: string) =>
  (lastTable().columns as ColumnDef<AnyRow, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );
const lastActions = () =>
  captured.rowActions.slice(-1)[0]?.map((a) => a.id) ?? [];

async function setFilters(filters: { id: string; value: unknown }[]) {
  await act(async () => {
    lastTable().state?.onColumnFiltersChange?.(filters);
  });
  await flush();
}

async function setSort(id: string, desc: boolean) {
  await act(async () => {
    lastTable().state?.onSortingChange?.([{ id, desc }]);
  });
  await flush();
}

async function search(q: string) {
  await act(async () => {
    lastTable().state?.onGlobalFilterChange?.(q);
  });
  await flush();
}

function service(over: Record<string, unknown> = {}) {
  return {
    uuid: "s1",
    service_no: "DSABCD1234",
    status: "completed",
    status_label: "Completed",
    organization: { uuid: "o1", name: "Olex Bayi", type: "dealer" },
    customer: { uuid: "c1", name: "Ayşe", surname: "Yılmaz" },
    car_brand: { uuid: "b1", name: "BMW" },
    car_model: { uuid: "m1", name: "320i" },
    model_year: 2022,
    plate: "34ABC123",
    package: null,
    items_editable: false,
    completed_at: "2026-10-01T10:00:00Z",
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T10:00:00Z",
    ...over,
  };
}

function warranty(over: Record<string, unknown> = {}) {
  return {
    uuid: "w1",
    public_code: "PUBCODE1",
    status: "active",
    start_at: "2026-01-01T00:00:00Z",
    end_at: "2028-01-01T00:00:00Z",
    created_at: "2026-01-01T00:00:00Z",
    product: { uuid: "p1", sku: "PPF-1", name: "Film PPF" },
    service: { uuid: "s1", service_no: "DS12345678" },
    organization: { uuid: "o1", name: "Olex Bayi", type: "dealer" },
    vehicle: {
      uuid: "v1",
      brand_name: "BMW",
      model_name: "X5",
      model_year: 2024,
      plate: "34 ABC 123",
    },
    holder: { uuid: "h1", name: "Ayşe", surname: "Kaya", anonymized: false },
    can_void: false,
    ...over,
  };
}

describe("ServicesListPage (TEC-378)", () => {
  it("income / profit / margin columns only for accounting readers, hidden by default (TEC-347)", async () => {
    captured.grants = new Set([Permission.ServicesRead]);
    await render(createElement(ServicesListPage, { slug: "acme" }));
    for (const id of ["income_amount", "gross_profit", "margin_pct"]) {
      expect(column(id)).toBeUndefined();
    }
    act(() => root.unmount());
    root = createRoot(container);
    captured.grants = new Set([
      Permission.ServicesRead,
      Permission.AccountingRead,
    ]);
    await render(createElement(ServicesListPage, { slug: "acme" }));
    for (const id of ["income_amount", "gross_profit", "margin_pct"]) {
      const col = column(id);
      expect(col).toBeDefined();
      expect(
        (col?.meta as { defaultHidden?: boolean } | undefined)?.defaultHidden,
      ).toBe(true);
      expect(col?.enableSorting).toBe(false);
    }
  });

  it("is forbidden without services.read", async () => {
    await render(createElement(ServicesListPage, { slug: "acme" }));
    expect(container.textContent).toContain("common.error_forbidden");
    expect(captured.services.listServices).not.toHaveBeenCalled();
  });

  it("maps sort, the status / organization facets, date ranges and q", async () => {
    captured.grants = new Set([
      Permission.ServicesRead,
      Permission.OrganizationsRead,
    ]);
    await render(createElement(ServicesListPage, { slug: "acme" }));
    expect(lastCall(captured.services.listServices)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "-created_at",
    });
    expect(lastTable().rowCount).toBe(45);
    expect(lastTable().renderGridItem).toBeTypeOf("function");
    expect(lastTable().features?.persistKey).toBe("tenant-services-v1");
    // Only backend sort fields are sortable.
    for (const id of [
      "service_no",
      "status",
      "plate",
      "organization",
      "created_at",
      "completed_at",
      "updated_at",
    ]) {
      expect(column(id)?.enableSorting).toBe(true);
    }
    expect(column("customer")?.enableSorting).toBe(false);
    expect(column("organization")?.enableColumnFilter).toBe(true);

    await setFilters([
      { id: "status", value: ["draft", "ready"] },
      { id: "organization", value: ["o1", "o2"] },
      { id: "created_at", value: ["2026-10-01", "2026-10-05"] },
      { id: "completed_at", value: [undefined, "2026-10-31"] },
    ]);
    await setSort("plate", false);
    await search("34ABC");

    expect(lastCall(captured.services.listServices)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "plate",
      q: "34ABC",
      status: "draft,ready",
      organization_uuid: "o1,o2",
      created_from: "2026-10-01",
      created_to: "2026-10-05",
      completed_to: "2026-10-31",
    });

    await setSort("organization", true);
    expect(lastCall(captured.services.listServices)?.[0]).toMatchObject({
      sort: "-organization",
    });
  });

  it("exports with the list filters, search and sort", async () => {
    captured.grants = new Set([Permission.ServicesRead]);
    await render(createElement(ServicesListPage, { slug: "acme" }));
    await setFilters([{ id: "status", value: ["completed"] }]);
    await setSort("completed_at", true);
    await search("ayşe");
    const menu = captured.exports.at(-1)!;
    expect(menu.exportPath).toBe("/v1/services/export");
    expect(menu.formats).toEqual(["xlsx", "csv", "pdf"]);
    expect(menu.jobsHref).toBe("/t/acme/exports");
    expect(menu.query).toEqual({
      status: "completed",
      q: "ayşe",
      sort: "-completed_at",
    });
  });

  it("turns the organization filter off for a dealer", async () => {
    captured.orgType = "dealer";
    captured.grants = new Set([
      Permission.ServicesRead,
      Permission.OrganizationsRead,
    ]);
    await render(createElement(ServicesListPage, { slug: "acme" }));
    expect(column("organization")?.enableColumnFilter).toBe(false);
    expect(captured.customers.listOrganizations).not.toHaveBeenCalled();
  });

  it("offers open, continue in wizard and PDF per row", async () => {
    captured.grants = new Set([
      Permission.ServicesRead,
      Permission.ServicesWrite,
      Permission.CustomersRead,
      Permission.VehiclesRead,
    ]);
    captured.services.listServices.mockResolvedValue(
      page([service({ uuid: "d1", status: "draft", items_editable: true })]),
    );
    await render(createElement(ServicesListPage, { slug: "acme" }));
    expect(container.querySelector("[data-testid=create]")).not.toBeNull();
    expect(lastActions()).toEqual(["view", "wizard", "pdf"]);
    const actions = captured.rowActions.at(-1)!;
    await act(async () => actions.find((a) => a.id === "wizard")!.onSelect());
    expect(captured.push).toHaveBeenCalledWith("/t/acme/services/d1/wizard");
    await act(async () => actions.find((a) => a.id === "pdf")!.onSelect());
    expect(captured.servicePdf).toHaveBeenCalledWith(
      expect.objectContaining({ serviceUuid: "d1", serviceNo: "DSABCD1234" }),
    );

    lastTable().onRowClick?.(service({ uuid: "d1" }) as AnyRow);
    expect(captured.push).toHaveBeenLastCalledWith("/t/acme/services/d1");
  });

  it("hides the wizard action for a completed service or without write", async () => {
    captured.grants = new Set([Permission.ServicesRead]);
    captured.services.listServices.mockResolvedValue(
      page([service({ status: "draft", items_editable: true })]),
    );
    await render(createElement(ServicesListPage, { slug: "acme" }));
    expect(container.querySelector("[data-testid=create]")).toBeNull();
    expect(lastActions()).toEqual(["view", "pdf"]);
  });
});

describe("WarrantiesListPage (TEC-378)", () => {
  it("is forbidden without warranties.read", async () => {
    await render(createElement(WarrantiesListPage, { slug: "acme" }));
    expect(container.textContent).toContain("warranty.list.forbidden");
    expect(captured.warranties.list).not.toHaveBeenCalled();
  });

  it("maps the expiry sort, facets, days left, date ranges, product and q", async () => {
    captured.grants = new Set([
      Permission.WarrantiesRead,
      Permission.CatalogRead,
      Permission.OrganizationsRead,
    ]);
    await render(createElement(WarrantiesListPage, { slug: "acme" }));
    expect(lastCall(captured.warranties.list)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "expiry",
    });
    expect(lastTable().rowCount).toBe(12);
    expect(lastTable().features?.persistKey).toBe("tenant-warranties-v1");
    expect(lastTable().renderGridItem).toBeTypeOf("function");
    for (const id of [
      "public_code",
      "service_no",
      "status",
      "product",
      "organization",
      "expiry",
      "start_at",
      "end_at",
      "created_at",
    ]) {
      expect(column(id)?.enableSorting).toBe(true);
    }
    expect(column("holder")?.enableSorting).toBe(false);
    expect(column("vehicle")?.enableSorting).toBe(false);

    await setFilters([
      { id: "status", value: ["active", "expired"] },
      { id: "organization", value: ["o2"] },
      { id: "expiry", value: [0, 30] },
      { id: "start_at", value: ["2026-01-01", undefined] },
      { id: "end_at", value: ["2026-11-01", "2026-12-31"] },
    ]);
    // The catalog picker in the toolbar sets the product filter.
    const combo = captured.combos.at(-1)!;
    expect(combo.id).toBe("warranty-product");
    await act(async () => (combo.onValueChange as (v: string) => void)("p1"));
    await flush();
    await setSort("product", false);
    await search("34ABC");

    expect(lastCall(captured.warranties.list)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "product",
      q: "34ABC",
      status: "active,expired",
      organization_uuid: "o2",
      days_left_min: "0",
      days_left_max: "30",
      start_from: "2026-01-01",
      end_from: "2026-11-01",
      end_to: "2026-12-31",
      product_uuid: "p1",
    });
    expect(captured.combos.at(-1)!.value).toBe("p1");

    // Clearing the picker drops product_uuid; -expiry reverses the default.
    await act(async () =>
      (captured.combos.at(-1)!.onValueChange as (v: string) => void)(""),
    );
    await flush();
    await setSort("expiry", true);
    const last = lastCall(captured.warranties.list)?.[0];
    expect(last.product_uuid).toBeUndefined();
    expect(last.sort).toBe("-expiry");
  });

  it("hides the product picker without catalog.read", async () => {
    captured.grants = new Set([Permission.WarrantiesRead]);
    await render(createElement(WarrantiesListPage, { slug: "acme" }));
    expect(captured.combos).toHaveLength(0);
  });

  it("exports with the list filters, search and sort", async () => {
    captured.grants = new Set([Permission.WarrantiesRead]);
    await render(createElement(WarrantiesListPage, { slug: "acme" }));
    await setFilters([{ id: "status", value: ["void"] }]);
    await search("PUB");
    const menu = captured.exports.at(-1)!;
    expect(menu.exportPath).toBe("/v1/warranties/export");
    expect(menu.formats).toEqual(["xlsx", "csv", "pdf"]);
    expect(menu.query).toEqual({ status: "void", q: "PUB", sort: "expiry" });
  });

  it("offers open, certificate and void per row by status and grants", async () => {
    captured.grants = new Set([
      Permission.WarrantiesRead,
      Permission.WarrantiesVoid,
    ]);
    captured.warranties.list.mockResolvedValue(
      page([
        warranty({ can_void: true }),
        warranty({ uuid: "w2", status: "void", can_void: false }),
      ]),
    );
    await render(createElement(WarrantiesListPage, { slug: "acme" }));
    const [first, second] = captured.rowActions.slice(-2);
    expect(first!.map((a) => a.id)).toEqual(["view", "certificate", "void"]);
    expect(second!.map((a) => a.id)).toEqual(["view"]);

    await act(async () =>
      first!.find((a) => a.id === "certificate")!.onSelect(),
    );
    expect(captured.certificate).toHaveBeenCalledTimes(1);

    await act(async () => first!.find((a) => a.id === "void")!.onSelect());
    await flush();
    expect(captured.voids.at(-1)?.warranty).toMatchObject({ uuid: "w1" });
  });

  it("needs warranties.void for the void action", async () => {
    captured.grants = new Set([Permission.WarrantiesRead]);
    captured.warranties.list.mockResolvedValue(
      page([warranty({ can_void: true })]),
    );
    await render(createElement(WarrantiesListPage, { slug: "acme" }));
    expect(lastActions()).toEqual(["view", "certificate"]);
  });
});

describe("Service detail nested tables (TEC-378)", () => {
  const item = (over: Record<string, unknown> = {}) => ({
    uuid: "i1",
    product: { uuid: "p1", sku: "PPF-1", name: "Olex PPF", unit_type: "m" },
    barcode: "OLX-1",
    unit_kind: "serial",
    kind: "partial",
    quantity: null,
    meters: "4.50",
    applied_parts: ["body_kaput"],
    notes: null,
    created_at: "2026-10-01T09:00:00Z",
    correction: null,
    ...over,
  });

  it("items: client-side sort, product / part facets and amount order", async () => {
    await render(
      createElement(ServiceItemsTable, {
        items: [
          item(),
          item({
            uuid: "i2",
            product: {
              uuid: "p2",
              sku: "C-1",
              name: "Ceramic",
              unit_type: "pc",
            },
            kind: "full",
            meters: null,
            quantity: 3,
            applied_parts: ["custom_part", "body_kaput"],
          }),
        ] as never,
      }),
    );
    const table = lastTable();
    expect(table.manual).toEqual({
      sorting: false,
      filtering: false,
      pagination: false,
    });
    expect(table.features?.persistKey).toBe("tenant-service-items-v1");
    expect(table.initialState?.sorting).toEqual([
      { id: "created_at", desc: false },
    ]);
    for (const id of ["product", "barcode", "amount", "created_at"]) {
      expect(column(id)?.enableSorting).toBe(true);
    }
    expect(column("product")?.meta?.filterOptions?.map((o) => o.value)).toEqual(
      ["Ceramic", "Olex PPF"],
    );
    // Sorted by label: unknown parts keep their key, known ones translate.
    expect(column("parts")?.meta?.filterOptions).toEqual([
      { value: "custom_part", label: "custom_part" },
      { value: "body_kaput", label: "services.parts.names.body_kaput" },
    ]);
    // Amount sorts by meters of a cut, else pieces.
    const amount = column("amount") as unknown as {
      accessorFn: (r: unknown) => number;
    };
    expect(amount.accessorFn(item())).toBe(4.5);
    expect(amount.accessorFn(item({ meters: null, quantity: 3 }))).toBe(3);
    // The parts facet matches any applied part.
    const parts = column("parts") as unknown as {
      filterFn: (row: unknown, id: string, value: unknown) => boolean;
    };
    const row = { getValue: () => ["custom_part", "body_kaput"] };
    expect(parts.filterFn(row, "parts", ["body_kaput"])).toBe(true);
    expect(parts.filterFn(row, "parts", ["roof"])).toBe(false);
  });

  it("warranties: status facet, end date order and row link with warranties.read", async () => {
    const w = {
      uuid: "w1",
      public_code: "W-1",
      service_item_uuid: "i1",
      product_name: "Olex PPF",
      item_kind: "partial",
      status: "active",
      start_at: "2026-10-01T00:00:00Z",
      end_at: "2036-10-01T00:00:00Z",
      expired_at: null,
      voided_at: null,
    };
    await render(
      createElement(ServiceWarrantiesTable, {
        slug: "acme",
        warranties: [w, { ...w, uuid: "w2", status: "void" }] as never,
        canOpen: true,
        emptyTitle: "none",
      }),
    );
    const table = lastTable();
    expect(table.features?.persistKey).toBe("tenant-service-warranties-v1");
    expect(table.initialState?.sorting).toEqual([
      { id: "end_at", desc: false },
    ]);
    expect(column("status")?.meta?.filterVariant).toBe("faceted");
    expect(column("status")?.enableColumnFilter).toBe(true);
    expect(lastActions()).toEqual(["view"]);
    table.onRowClick?.(w as never);
    expect(captured.push).toHaveBeenCalledWith("/t/acme/warranties/w1");

    await render(
      createElement(ServiceWarrantiesTable, {
        slug: "acme",
        warranties: [w] as never,
        canOpen: false,
        emptyTitle: "none",
      }),
    );
    expect(lastTable().onRowClick).toBeUndefined();
    expect(column("actions")).toBeUndefined();
  });
});
