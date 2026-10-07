// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type { OAuthGrant } from "@/features/mcp/services/mcp.service";

const grant: OAuthGrant = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000a1",
  client_id: "client-1",
  client_name: "Claude",
  resource: "/mcp/dealer",
  realm: "dealer",
  scopes: ["mcp"],
  organization_uuid: "org-1",
  organization_name: "Bayi Bir",
  organization_type: "dealer",
  created_at: "2026-10-01T09:00:00Z",
  last_used_at: null,
};

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<OAuthGrant>>,
  platform: [] as unknown[][],
  portal: [] as unknown[][],
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: { dateTime: (v: string) => v, relative: (v: string) => v },
  }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/lib/api/platform-request", () => ({
  platformRequest: async (...args: unknown[]) => {
    captured.platform.push(args);
    return args[0] === "GET"
      ? { items: [grant], total: 1, limit: 10, offset: 0 }
      : undefined;
  },
}));
vi.mock("@/features/portal/lib/portal-client", () => ({
  portalRequest: async (...args: unknown[]) => {
    captured.portal.push(args);
    return (args[1] as { method?: string } | undefined)?.method === "DELETE"
      ? undefined
      : { items: [grant], total: 1, limit: 10, offset: 0 };
  },
}));
vi.mock("@/components/dialogs/confirm-dialog", () => ({
  ConfirmDialog: (props: {
    open: boolean;
    onConfirm: () => void;
    onCancel: () => void;
  }) =>
    props.open
      ? createElement(
          "div",
          { "data-testid": "confirm-dialog" },
          createElement(
            "button",
            { "data-testid": "confirm-yes", onClick: props.onConfirm },
            "yes",
          ),
          createElement(
            "button",
            { "data-testid": "confirm-no", onClick: props.onCancel },
            "no",
          ),
        )
      : null,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityRowActions: ({
    actions,
  }: {
    actions: { id: string; label: string; onSelect: () => void }[];
  }) =>
    createElement(
      "div",
      null,
      actions.map((a) =>
        createElement(
          "button",
          { key: a.id, "data-testid": `row-${a.id}`, onClick: a.onSelect },
          a.label,
        ),
      ),
    ),
  EntityTable: (props: Partial<DataTableProps<OAuthGrant>>) => {
    captured.table = props;
    const actions = (
      props.columns as ColumnDef<OAuthGrant, unknown>[] | undefined
    )?.find((c) => c.id === "actions");
    const row = props.data?.[0];
    if (!actions?.cell || !row) return null;
    const cell = actions.cell as (ctx: unknown) => ReactNode;
    return createElement("div", null, cell({ row: { original: row } }));
  },
}));

import { ConnectedAppsTable } from "./connected-apps-table";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  captured.platform = [];
  captured.portal = [];
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

async function render(realm: "panel" | "portal") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(ConnectedAppsTable, { realm }),
      ),
    );
  });
  await flush();
}

const lastGet = () =>
  captured.platform.filter((c) => c[0] === "GET").at(-1) as [
    string,
    string,
    { query: Record<string, unknown> },
  ];
const byTestId = (id: string) =>
  container.querySelector<HTMLElement>(`[data-testid="${id}"]`);

describe("ConnectedAppsTable", () => {
  it("lists the grants with the default sort and maps sort, realm filter and q", async () => {
    await render("panel");
    expect(lastGet()[1]).toBe("/v1/oauth/grants");
    expect(lastGet()[2].query).toEqual({
      limit: 10,
      offset: 0,
      sort: "-created_at",
    });
    expect(captured.table?.rowCount).toBe(1);
    expect(captured.table?.features?.persistKey).toBe(
      "profile-connected-apps-v1",
    );

    await act(async () => {
      captured.table?.state?.onSortingChange?.([
        { id: "last_used_at", desc: true },
      ]);
    });
    await flush();
    expect(lastGet()[2].query.sort).toBe("-last_used_at");

    await act(async () => {
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "realm", value: ["dealer", "user"] },
      ]);
      captured.table?.state?.onGlobalFilterChange?.("claude");
    });
    await flush();
    expect(lastGet()[2].query).toMatchObject({
      realm: "dealer,user",
      q: "claude",
      offset: 0,
    });
  });

  it("sorts only by the whitelisted fields", async () => {
    await render("panel");
    const cols = captured.table?.columns as ColumnDef<OAuthGrant, unknown>[];
    const sortable = cols
      .filter((c) => c.enableSorting)
      .map((c) => c.id ?? ("accessorKey" in c ? c.accessorKey : ""));
    expect(sortable.sort()).toEqual([
      "client_name",
      "created_at",
      "last_used_at",
    ]);
  });

  it("deletes the grant only after the confirmation dialog", async () => {
    await render("panel");
    await act(async () => byTestId("row-disconnect")?.click());
    expect(byTestId("confirm-dialog")).not.toBeNull();
    expect(captured.platform.some((c) => c[0] === "DELETE")).toBe(false);

    await act(async () => byTestId("confirm-no")?.click());
    expect(byTestId("confirm-dialog")).toBeNull();
    expect(captured.platform.some((c) => c[0] === "DELETE")).toBe(false);

    await act(async () => byTestId("row-disconnect")?.click());
    await act(async () => byTestId("confirm-yes")?.click());
    await flush();
    expect(captured.platform).toContainEqual([
      "DELETE",
      `/v1/oauth/grants/${grant.uuid}`,
    ]);
    expect(byTestId("confirm-dialog")).toBeNull();
  });

  it("uses the customer portal endpoints on the portal", async () => {
    await render("portal");
    expect(captured.platform).toEqual([]);
    expect(captured.portal[0]?.[0]).toBe(
      "portal/oauth/grants?limit=10&offset=0&sort=-created_at",
    );
    await act(async () => byTestId("row-disconnect")?.click());
    await act(async () => byTestId("confirm-yes")?.click());
    await flush();
    expect(captured.portal).toContainEqual([
      `portal/oauth/grants/${grant.uuid}`,
      { method: "DELETE" },
    ]);
  });
});
