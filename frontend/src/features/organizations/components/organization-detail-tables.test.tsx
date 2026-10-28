// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";

type AnyRow = { uuid: string };

const captured = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<AnyRow>>[],
  children: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: { dateTime: (v: string) => v, date: (v: string) => v },
  }),
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityTable: (props: Partial<DataTableProps<AnyRow>>) => {
    captured.tables.push(props);
    return null;
  },
}));
vi.mock(
  "@/features/organizations/services/organizations.service",
  async (orig) => ({
    ...(await orig<object>()),
    organizationsService: { children: captured.children },
  }),
);

import {
  OrganizationChildrenTable,
  OrganizationMembersTable,
} from "./organization-detail-tables";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  captured.children.mockReset();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function render(node: ReturnType<typeof createElement>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

const last = () => captured.tables[captured.tables.length - 1]!;
const columnIds = (table: Partial<DataTableProps<AnyRow>>) =>
  (table.columns as ColumnDef<AnyRow, unknown>[]).map(
    (c) => c.id ?? ("accessorKey" in c ? String(c.accessorKey) : ""),
  );

describe("organization detail nested tables", () => {
  it("members render as a client-side DataTable with role/status facets", async () => {
    const members = [
      {
        uuid: "u1",
        email: "a@x.test",
        name: "A",
        surname: "B",
        status: "active",
        role: "owner",
      },
    ];
    await render(createElement(OrganizationMembersTable, { members }));
    const table = last();
    expect(table.manual).toEqual({
      sorting: false,
      filtering: false,
      pagination: false,
    });
    expect(table.data).toBe(members);
    expect(table.features?.persistKey).toBe("platform-organization-members-v2");
    expect(columnIds(table)).toEqual([
      "name",
      "email",
      "role",
      "role_slugs",
      "status",
      "created_at",
    ]);
    const role = (table.columns as ColumnDef<AnyRow, unknown>[])[2];
    expect(role?.meta?.filterVariant).toBe("faceted");
  });

  it("TEC-1036: role slugs filter by any match; handlers add a row actions column", async () => {
    const members = [
      {
        uuid: "u1",
        email: "a@x.test",
        name: "A",
        surname: "B",
        status: "active",
        role: "staff",
        role_slugs: ["center_accounting", "center_warehouse"],
      },
    ];
    const handlers = { onEditRoles: vi.fn(), onRemove: vi.fn() };
    await render(
      createElement(OrganizationMembersTable, { members, handlers }),
    );
    const table = last();
    expect(columnIds(table)).toEqual([
      "name",
      "email",
      "role",
      "role_slugs",
      "status",
      "created_at",
      "actions",
    ]);
    const slugs = (table.columns as ColumnDef<AnyRow, unknown>[])[3]!;
    expect(slugs.meta?.filterVariant).toBe("faceted");
    const row = { getValue: () => members[0]!.role_slugs };
    const filterFn = slugs.filterFn as (
      row: unknown,
      id: string,
      value: unknown,
    ) => boolean;
    expect(filterFn(row, "role_slugs", ["center_warehouse"])).toBe(true);
    expect(filterFn(row, "role_slugs", ["center_social"])).toBe(false);
    expect(filterFn(row, "role_slugs", [])).toBe(true);
  });

  it("sub-organizations load from /children into a client-side table", async () => {
    const child = { uuid: "c1", name: "Dealer", slug: "dealer" };
    captured.children.mockResolvedValue({ items: [child] });
    await render(createElement(OrganizationChildrenTable, { uuid: "p1" }));
    expect(captured.children).toHaveBeenCalledWith("p1");
    const table = last();
    expect(table.manual?.pagination).toBe(false);
    expect(table.data).toEqual([child]);
    expect(table.features?.persistKey).toBe(
      "platform-organization-children-v1",
    );
  });
});
