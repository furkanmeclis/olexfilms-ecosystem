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
  bulkMenus: [] as Record<string, unknown>[],
  exports: [] as Record<string, unknown>[],
  customerExports: [] as Record<string, unknown>[],
  rowActions: new Map<string, RowAction[]>(),
  grants: new Set<string>(),
  orgType: "center" as string | null,
  push: vi.fn(),
  customers: {
    list: vi.fn(),
    listOrganizations: vi.fn(),
  },
  leads: { list: vi.fn() },
  vehicles: { list: vi.fn() },
  catalog: { listBrands: vi.fn(), listModels: vi.fn() },
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
vi.mock("@/features/customers/services/customers.service", async (orig) => ({
  ...(await orig<object>()),
  customersService: captured.customers,
}));
vi.mock("@/features/leads/services/leads.service", async (orig) => ({
  ...(await orig<object>()),
  leadsService: captured.leads,
}));
vi.mock("@/features/vehicles/services/vehicles.service", async (orig) => ({
  ...(await orig<object>()),
  vehiclesService: captured.vehicles,
}));
vi.mock("@/features/vehicle-catalog/services/vehicle-catalog.service", () => ({
  vehicleCatalogService: captured.catalog,
}));
vi.mock("@/features/vehicle-catalog/components/vehicle-brand-logo", () => ({
  VehicleBrandLogo: () => null,
}));
vi.mock("@/features/bulk-engine", async (orig) => ({
  ...(await orig<object>()),
  BulkActionMenu: (props: Record<string, unknown>) => {
    captured.bulkMenus.push(props);
    return null;
  },
  SelectionBanner: () => null,
}));
vi.mock("@/features/io/components/export-menu", () => ({
  ExportMenu: (props: Record<string, unknown>) => {
    captured.exports.push(props);
    return null;
  },
}));
vi.mock("@/features/customers/components/customer-list-export", () => ({
  CustomerListExportButton: (props: Record<string, unknown>) => {
    captured.customerExports.push(props);
    return null;
  },
}));
vi.mock("@/features/customers/components/customer-actions", () => ({
  AnonymizeDialog: () => createElement("div", { "data-dialog": "anonymize" }),
  DataExportDialog: () => createElement("div", { "data-dialog": "export" }),
  UpgradeDialog: () => createElement("div", { "data-dialog": "upgrade" }),
}));
vi.mock("@/features/customers/components/vehicle-form-dialog", () => ({
  VehicleFormDialog: ({ vehicle }: { vehicle: { uuid: string } | null }) =>
    createElement("div", {
      "data-dialog": "vehicle",
      "data-uuid": vehicle?.uuid,
    }),
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({
    children,
    actions,
  }: {
    children: ReactNode;
    actions?: ReactNode;
  }) => createElement(Fragment, null, actions, children),
  EntityCreateButton: () => null,
  EntityToolbar: () => null,
  EntityRowActions: ({ actions }: { actions: RowAction[] }) => {
    captured.rowActions.set(String(captured.rowActions.size), actions);
    return null;
  },
  // Renders the actions cell of every row so row actions can be asserted.
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
import { LeadsListPage } from "@/features/leads/components/leads-list-page";
import { VehiclesListPage } from "@/features/vehicles/components/vehicles-list-page";

import { CustomerVehiclesTable } from "./customer-vehicles-table";
import { CustomersListPage } from "./customers-list-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

const page = (items: unknown[] = [], total = 0) => ({
  items,
  total,
  limit: 20,
  offset: 0,
});

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  captured.bulkMenus = [];
  captured.exports = [];
  captured.customerExports = [];
  captured.rowActions = new Map();
  captured.grants = new Set();
  captured.orgType = "center";
  captured.push.mockReset();
  for (const fn of [
    ...Object.values(captured.customers),
    ...Object.values(captured.leads),
    ...Object.values(captured.vehicles),
    ...Object.values(captured.catalog),
  ]) {
    fn.mockReset();
  }
  captured.customers.list.mockResolvedValue(page([], 45));
  captured.customers.listOrganizations.mockResolvedValue([
    { uuid: "o1", name: "Olex Bayi", type: "dealer" },
    { uuid: "o2", name: "Olex Dist", type: "distributor" },
  ]);
  captured.leads.list.mockResolvedValue(page([], 7));
  captured.vehicles.list.mockResolvedValue(page([], 3));
  captured.catalog.listBrands.mockResolvedValue(
    page([{ uuid: "b1", name: "BMW" }]),
  );
  captured.catalog.listModels.mockResolvedValue(
    page([{ uuid: "m1", name: "320i" }]),
  );
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
const actionIds = () =>
  [...captured.rowActions.values()].map((list) => list.map((a) => a.id));

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

function customer(over: Record<string, unknown> = {}) {
  return {
    uuid: "c1",
    name: "Ayşe",
    surname: "Yılmaz",
    email: "ayse@example.com",
    phone: "+905551234567",
    status: "active",
    anonymized: false,
    type: "individual",
    company_name: null,
    locale: "tr",
    created_at: "2026-09-01T09:00:00Z",
    linked_at: "2026-09-01T09:00:00Z",
    first_service_at: null,
    ...over,
  };
}

function vehicle(over: Record<string, unknown> = {}) {
  return {
    uuid: "v1",
    customer_uuid: "c1",
    organization_uuid: "o1",
    plate: "34 ABC 123",
    plate_normalized: "34ABC123",
    plate_country: "TR",
    vin: null,
    model_year: 2022,
    car_brand: { uuid: "b1", name: "BMW" },
    car_model: { uuid: "m1", name: "320i" },
    created_at: "2026-09-01T09:00:00Z",
    updated_at: "2026-09-01T09:00:00Z",
    warnings: [],
    ...over,
  };
}

describe("CustomersListPage (TEC-372)", () => {
  beforeEach(() => {
    captured.grants = new Set([
      Permission.CustomersRead,
      Permission.OrganizationsRead,
    ]);
  });

  it("maps sort, facets, the linked range, organization and q to the list params", async () => {
    await render(createElement(CustomersListPage, { slug: "acme" }));
    expect(lastCall(captured.customers.list)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "-linked_at",
    });
    expect(lastTable().rowCount).toBe(45);
    expect(lastTable().renderGridItem).toBeTypeOf("function");

    await setFilters([
      { id: "status", value: ["active", "pending"] },
      { id: "type", value: ["corporate"] },
      { id: "linked_at", value: ["2026-01-01", "2026-02-01"] },
      { id: "organization", value: ["o1", "o2"] },
    ]);
    await setSort("email", false);
    await act(async () => {
      lastTable().state?.onGlobalFilterChange?.("ayşe");
    });
    await flush();

    expect(lastCall(captured.customers.list)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "email",
      q: "ayşe",
      status: "active,pending",
      type: "corporate",
      linked_from: "2026-01-01",
      linked_to: "2026-02-01",
      organization_uuid: "o1,o2",
    });
    // The export runs on the same filters, search and sort.
    expect(
      captured.customerExports[captured.customerExports.length - 1]?.filters,
    ).toEqual({
      status: "active,pending",
      type: "corporate",
      linked_from: "2026-01-01",
      linked_to: "2026-02-01",
      organization_uuid: "o1,o2",
      q: "ayşe",
      sort: "email",
    });

    for (const id of [
      "name",
      "email",
      "status",
      "linked_at",
      "first_service_at",
    ]) {
      expect(column(id)?.enableSorting).toBe(true);
    }
    expect(column("phone")?.enableSorting).toBe(false);
  });

  it("offers the organization filter to center and distributor only", async () => {
    await render(createElement(CustomersListPage, { slug: "acme" }));
    expect(captured.customers.listOrganizations).toHaveBeenCalled();
    expect(column("organization")?.enableColumnFilter).toBe(true);
    expect(column("organization")?.meta?.filterOptions).toEqual([
      { value: "o1", label: "Olex Bayi" },
      { value: "o2", label: "Olex Dist" },
    ]);

    act(() => root.unmount());
    root = createRoot(container);
    captured.customers.listOrganizations.mockClear();
    captured.orgType = "dealer";
    await render(createElement(CustomersListPage, { slug: "acme" }));
    expect(captured.customers.listOrganizations).not.toHaveBeenCalled();
    expect(column("organization")?.enableColumnFilter).toBe(false);
  });

  it("shows the detail actions per row by permission and opens their dialogs", async () => {
    captured.grants = new Set([
      Permission.CustomersRead,
      Permission.CustomersWrite,
      Permission.CustomersAnonymize,
      Permission.OrganizationsRead,
    ]);
    captured.customers.list.mockResolvedValue(
      page(
        [
          customer(),
          customer({ uuid: "c2", anonymized: true, status: "anonymized" }),
        ],
        2,
      ),
    );
    await render(createElement(CustomersListPage, { slug: "acme" }));
    const [first, second] = [...captured.rowActions.values()].slice(-2);
    expect(first?.map((a) => a.id)).toEqual([
      "view",
      "edit",
      "upgrade",
      "export",
      "anonymize",
    ]);
    expect(second?.map((a) => a.id)).toEqual(["view"]);

    await act(async () => {
      first?.find((a) => a.id === "anonymize")?.onSelect();
    });
    expect(container.querySelector("[data-dialog=anonymize]")).not.toBeNull();
    await act(async () => {
      first?.find((a) => a.id === "edit")?.onSelect();
    });
    expect(captured.push).toHaveBeenCalledWith("/t/acme/customers/c1/edit");
  });

  it("dealer rows have no privacy or upgrade actions", async () => {
    captured.orgType = "dealer";
    captured.grants = new Set([
      Permission.CustomersRead,
      Permission.CustomersWrite,
      Permission.CustomersAnonymize,
      Permission.OrganizationsRead,
    ]);
    captured.customers.list.mockResolvedValue(page([customer()], 1));
    await render(createElement(CustomersListPage, { slug: "acme" }));
    expect(actionIds().at(-1)).toEqual(["view", "edit"]);
  });
});

describe("CustomerVehiclesTable (TEC-372)", () => {
  it("is a nested client-side table with sort and row actions", async () => {
    const onEdit = vi.fn();
    await render(
      createElement(CustomerVehiclesTable, {
        slug: "acme",
        vehicles: [vehicle()] as never,
        canWrite: true,
        canTransfer: true,
        onEdit,
      }),
    );
    expect(lastTable().manual).toEqual({
      sorting: false,
      filtering: false,
      pagination: false,
    });
    for (const id of ["plate", "brand", "model", "model_year", "vin"]) {
      expect(column(id)?.enableSorting).toBe(true);
    }
    const actions = actionIds().at(-1);
    expect(actions).toEqual(["open", "edit-vehicle", "transfer"]);
    const list = [...captured.rowActions.values()].at(-1)!;
    await act(async () => {
      list.find((a) => a.id === "edit-vehicle")?.onSelect();
    });
    expect(onEdit).toHaveBeenCalledWith(
      expect.objectContaining({ uuid: "v1" }),
    );
    await act(async () => {
      list.find((a) => a.id === "open")?.onSelect();
    });
    expect(captured.push).toHaveBeenCalledWith("/t/acme/vehicles/v1");
  });

  it("read-only users only open the vehicle", async () => {
    await render(
      createElement(CustomerVehiclesTable, {
        slug: "acme",
        vehicles: [vehicle()] as never,
        canWrite: false,
        canTransfer: false,
        onEdit: vi.fn(),
      }),
    );
    expect(actionIds().at(-1)).toEqual(["open"]);
  });
});

describe("LeadsListPage (TEC-372)", () => {
  beforeEach(() => {
    captured.grants = new Set([Permission.LeadsRead, Permission.LeadsWrite]);
  });

  it("maps sort, facets, the assignee (incl. none), created range and the follow-up tab", async () => {
    await render(createElement(LeadsListPage, { slug: "acme" }));
    expect(lastCall(captured.leads.list)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "-created_at",
    });
    expect(lastTable().rowCount).toBe(7);
    expect(column("assignee")?.meta?.filterOptions?.[0]).toEqual({
      value: "none",
      label: "leads.form.unassigned",
    });

    await setFilters([
      { id: "status", value: ["new", "quoted"] },
      { id: "target_type", value: ["dealer_candidate"] },
      { id: "source", value: ["whatsapp", "website"] },
      { id: "temperature", value: ["hot"] },
      { id: "assignee", value: ["none", "7"] },
      { id: "created_at", value: ["2026-09-01", "2026-09-30"] },
    ]);
    await setSort("temperature", true);
    const tab = container.querySelector(
      "[data-testid=lead-tab-overdue]",
    ) as HTMLButtonElement;
    await act(async () => tab.click());
    await flush();

    const expected = {
      status: "new,quoted",
      target_type: "dealer_candidate",
      source: "whatsapp,website",
      temperature: "hot",
      assignee_user_id: "none,7",
      created_from: "2026-09-01",
      created_to: "2026-09-30",
    };
    expect(lastCall(captured.leads.list)?.[0]).toEqual({
      ...expected,
      limit: 20,
      offset: 0,
      sort: "-temperature",
      follow_up: "overdue",
    });
    // A chosen assignee stays among the options.
    expect(
      column("assignee")?.meta?.filterOptions?.map((o) => o.value),
    ).toEqual(["none", "7"]);

    const exportProps = captured.exports[captured.exports.length - 1];
    expect(exportProps?.exportPath).toBe("/v1/leads/export");
    expect(exportProps?.formats).toEqual(["xlsx", "csv", "pdf"]);
    expect(exportProps?.query).toEqual({
      ...expected,
      follow_up: "overdue",
      sort: "-temperature",
    });

    for (const id of [
      "name",
      "status",
      "temperature",
      "follow_up_date",
      "created_at",
    ]) {
      expect(column(id)?.enableSorting).toBe(true);
    }
  });

  it("offers bulk assign / set_status on the leads resource with an enum status", async () => {
    await render(createElement(LeadsListPage, { slug: "acme" }));
    expect(lastTable().features?.rowSelection).toBe(true);
    const menu = captured.bulkMenus[captured.bulkMenus.length - 1];
    expect(menu?.resource).toBe("leads");
    const actions = menu?.actions as {
      id: string;
      params?: { key: string; kind: string; options?: string[] }[];
    }[];
    expect(actions.map((a) => a.id)).toEqual(["assign", "set_status"]);
    expect(actions[0]?.params?.[0]).toMatchObject({
      key: "assignee_user_id",
      kind: "number",
    });
    expect(actions[1]?.params).toEqual([
      expect.objectContaining({
        key: "status",
        kind: "enum",
        options: ["new", "contacted", "quoted", "won", "lost"],
      }),
      expect.objectContaining({ key: "lost_reason", kind: "text" }),
    ]);
  });

  it("hides selection and bulk for readers", async () => {
    captured.grants = new Set([Permission.LeadsRead]);
    await render(createElement(LeadsListPage, { slug: "acme" }));
    expect(lastTable().features?.rowSelection).toBe(false);
    expect(captured.bulkMenus).toHaveLength(0);
  });
});

describe("VehiclesListPage (TEC-372)", () => {
  beforeEach(() => {
    captured.grants = new Set([
      Permission.VehiclesRead,
      Permission.VehiclesWrite,
      Permission.CustomersRead,
      Permission.OrganizationsRead,
    ]);
  });

  it("maps sort, q, brand / model and organization to GET /v1/vehicles", async () => {
    await render(createElement(VehiclesListPage, { slug: "acme" }));
    expect(lastCall(captured.vehicles.list)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "-created_at",
    });
    expect(lastTable().rowCount).toBe(3);
    expect(column("brand")?.meta?.filterOptions).toEqual([
      { value: "b1", label: "BMW" },
    ]);
    // Models are offered once one brand is chosen.
    expect(column("model")?.enableColumnFilter).toBe(false);

    await setFilters([{ id: "brand", value: ["b1"] }]);
    expect(lastCall(captured.catalog.listModels)?.[0]).toMatchObject({
      brand_uuid: "b1",
    });
    expect(column("model")?.enableColumnFilter).toBe(true);

    await setFilters([
      { id: "brand", value: ["b1"] },
      { id: "model", value: ["m1"] },
      { id: "organization", value: ["o2"] },
    ]);
    await setSort("brand", false);
    await act(async () => {
      lastTable().state?.onGlobalFilterChange?.("BMW 3");
    });
    await flush();
    expect(lastCall(captured.vehicles.list)?.[0]).toEqual({
      limit: 20,
      offset: 0,
      sort: "brand",
      q: "BMW 3",
      car_brand_uuid: "b1",
      car_model_uuid: "m1",
      organization_uuid: "o2",
    });
    for (const id of ["plate", "brand", "model", "model_year", "created_at"]) {
      expect(column(id)?.enableSorting).toBe(true);
    }
  });

  it("opens the vehicle on row click and offers customer / edit actions", async () => {
    captured.vehicles.list.mockResolvedValue(page([vehicle()], 1));
    await render(createElement(VehiclesListPage, { slug: "acme" }));
    lastTable().onRowClick?.(vehicle() as never);
    expect(captured.push).toHaveBeenCalledWith("/t/acme/vehicles/v1");
    const list = [...captured.rowActions.values()].at(-1)!;
    expect(list.map((a) => a.id)).toEqual(["open", "customer", "edit"]);
    await act(async () => {
      list.find((a) => a.id === "customer")?.onSelect();
    });
    expect(captured.push).toHaveBeenCalledWith("/t/acme/customers/c1");
    await act(async () => {
      list.find((a) => a.id === "edit")?.onSelect();
    });
    expect(
      container
        .querySelector("[data-dialog=vehicle]")
        ?.getAttribute("data-uuid"),
    ).toBe("v1");
  });
});
