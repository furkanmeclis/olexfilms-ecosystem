// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type { OAuthClientSummary } from "@/features/mcp/services/mcp.service";

const client: OAuthClientSummary = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000c1",
  client_id: "client-1",
  client_name: "Claude",
  redirect_uris: ["https://claude.ai/api/mcp/auth_callback"],
  created_ip: "203.0.113.7",
  status: "active",
  active_grants: 3,
  created_at: "2026-10-01T09:00:00Z",
  last_used_at: null,
  revoked_at: null,
};

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<OAuthClientSummary>>,
  requests: [] as unknown[][],
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: {
      dateTime: (v: string) => v,
      relative: (v: string) => v,
      number: (v: number) => String(v),
    },
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
vi.mock("@/lib/api/platform-request", () => ({
  platformRequest: async (...args: unknown[]) => {
    captured.requests.push(args);
    return args[0] === "GET"
      ? { items: [client], total: 1, limit: 20, offset: 0 }
      : undefined;
  },
}));
vi.mock("@/components/dialogs/confirm-dialog", () => ({
  ConfirmDialog: (props: { open: boolean; onConfirm: () => void }) =>
    props.open
      ? createElement(
          "button",
          { "data-testid": "confirm-yes", onClick: props.onConfirm },
          "yes",
        )
      : null,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: unknown }) => children,
  EntityToolbar: () => null,
  EntityRowActions: ({
    actions,
  }: {
    actions: { id: string; disabled?: boolean; onSelect: () => void }[];
  }) =>
    createElement(
      "div",
      null,
      actions.map((a) =>
        createElement(
          "button",
          {
            key: a.id,
            "data-testid": `row-${a.id}`,
            disabled: a.disabled,
            onClick: a.onSelect,
          },
          a.id,
        ),
      ),
    ),
  EntityTable: (props: Partial<DataTableProps<OAuthClientSummary>>) => {
    captured.table = props;
    const actions = (
      props.columns as ColumnDef<OAuthClientSummary, unknown>[] | undefined
    )?.find((c) => c.id === "actions");
    const row = props.data?.[0];
    if (!actions?.cell || !row) return null;
    const cell = actions.cell as (ctx: unknown) => ReactNode;
    return createElement("div", null, cell({ row: { original: row } }));
  },
}));

import { McpClientsPage } from "./mcp-clients-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  captured.requests = [];
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

async function render() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client: qc },
        createElement(McpClientsPage),
      ),
    );
  });
  await flush();
}

const lastGet = () =>
  captured.requests.filter((c) => c[0] === "GET").at(-1) as [
    string,
    string,
    { query: Record<string, unknown> },
  ];

describe("McpClientsPage", () => {
  it("maps the status filter, sort and q to the platform client list", async () => {
    await render();
    expect(lastGet()[1]).toBe("/v1/platform/oauth/clients");
    expect(lastGet()[2].query).toEqual({
      limit: 20,
      offset: 0,
      sort: "-created_at",
    });
    await act(async () => {
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "status", value: ["active"] },
      ]);
      captured.table?.state?.onSortingChange?.([
        { id: "client_name", desc: false },
      ]);
      captured.table?.state?.onGlobalFilterChange?.("claude");
    });
    await flush();
    expect(lastGet()[2].query).toMatchObject({
      status: "active",
      sort: "client_name",
      q: "claude",
    });
    expect(captured.table?.features?.persistKey).toBe(
      "platform-mcp-clients-v1",
    );
  });

  it("revokes a client after the confirmation", async () => {
    await render();
    const btn = container.querySelector<HTMLButtonElement>(
      '[data-testid="row-revoke"]',
    );
    await act(async () => btn?.click());
    expect(captured.requests.some((c) => c[0] === "DELETE")).toBe(false);
    await act(async () =>
      container
        .querySelector<HTMLButtonElement>('[data-testid="confirm-yes"]')
        ?.click(),
    );
    await flush();
    expect(captured.requests).toContainEqual([
      "DELETE",
      `/v1/platform/oauth/clients/${client.uuid}`,
    ]);
  });
});
