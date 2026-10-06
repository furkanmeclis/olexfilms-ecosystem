// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type {
  ListUsersParams,
  PublicUser,
} from "@/features/users/services/users.service";

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<PublicUser>>,
  listParams: [] as ListUsersParams[],
  meta: undefined as undefined | { default_sort?: string },
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
vi.mock("@/providers/auth-provider", () => ({
  useAuth: () => ({ user: { uuid: "me", isSuperAdmin: true } }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: unknown }) => children,
  EntityCreateButton: () => null,
  EntityToolbar: () => null,
  EntityTable: (props: Partial<DataTableProps<PublicUser>>) => {
    captured.table = props;
    return null;
  },
}));
vi.mock("@/features/bulk-engine", () => ({
  BulkActionMenu: () => null,
  SelectionBanner: () => null,
  resolveBulkActionsWithIcons: () => [],
  useBulkSelection: () => ({
    rowSelection: {},
    onRowSelectionChange: () => {},
    selectedCount: 0,
    showSelectAllBanner: false,
    scope: { mode: "ids", ids: [] },
    selectAllMatching: () => {},
    clearSelection: () => {},
  }),
}));
vi.mock("@/features/io", () => ({ ResourceIOToolbar: () => null }));
vi.mock("@/features/users/components/user-set-password-dialog", () => ({
  UserSetPasswordDialog: () => null,
}));
vi.mock("@/features/users/hooks/use-user-mutations", () => ({
  useEnableUser: () => ({ mutate: vi.fn() }),
  useDisableUser: () => ({ mutate: vi.fn() }),
  useImpersonateUser: () => ({ mutate: vi.fn() }),
}));
vi.mock("@/features/users/hooks/use-users-query", () => ({
  useUsersList: (params: ListUsersParams) => {
    captured.listParams.push(params);
    return {
      data: { items: [], total: 57, limit: 20, offset: 0 },
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    };
  },
  useUsersMeta: () => ({ data: captured.meta }),
}));
vi.mock("@/features/roles/services/roles.service", () => ({
  rolesService: {
    list: vi.fn(async () => ({
      items: [
        { uuid: "r1", slug: "admin", name: "Admin", is_system: true },
        { uuid: "r2", slug: "support", name: "Support", is_system: false },
      ],
      total: 2,
      limit: 100,
      offset: 0,
    })),
  },
}));

import { UsersPage } from "./users-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  captured.listParams = [];
  captured.meta = undefined;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(QueryClientProvider, { client }, createElement(UsersPage)),
    );
  });
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

const lastParams = () => captured.listParams[captured.listParams.length - 1];
const column = (id: string) =>
  (captured.table?.columns as ColumnDef<PublicUser, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("UsersPage list params", () => {
  it("sends status/roles as CSV and created_at as a date range", async () => {
    await render();
    expect(lastParams()).toEqual({ limit: 20, offset: 0, sort: "-created_at" });

    await act(async () => {
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "status", value: ["active", "disabled"] },
        { id: "roles", value: ["admin", "support"] },
        { id: "created_at", value: ["2026-01-01", "2026-02-01"] },
      ]);
    });

    expect(lastParams()).toMatchObject({
      status: "active,disabled",
      role: "admin,support",
      created_from: "2026-01-01",
      created_to: "2026-02-01",
      offset: 0,
    });
  });

  it("role filter options come from the roles catalog (slug values)", async () => {
    await render();
    const roles = column("roles");
    expect(roles?.meta?.param).toBe("role");
    expect(roles?.meta?.filterVariant).toBe("faceted");
    expect(roles?.meta?.filterOptions?.map((o) => o.value)).toEqual([
      "admin",
      "support",
    ]);
  });

  it("enables sorting on name/email/status/created_at and sends single sort", async () => {
    await render();
    for (const id of ["name", "email", "status", "created_at"]) {
      expect(column(id)?.enableSorting, id).toBe(true);
    }
    await act(async () => {
      captured.table?.state?.onSortingChange?.([{ id: "email", desc: false }]);
    });
    expect(lastParams().sort).toBe("email");
  });

  it("passes the list total as rowCount and uses meta default_sort", async () => {
    captured.meta = { default_sort: "email" };
    await render();
    expect(captured.table?.rowCount).toBe(57);
    expect(captured.table?.features?.persistKey).toBe("platform-users-v1");
    expect(lastParams().sort).toBe("email");
  });
});
