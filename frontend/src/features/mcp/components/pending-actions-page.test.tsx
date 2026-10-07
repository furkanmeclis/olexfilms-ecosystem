// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type { AIActionCard } from "@/features/mcp/services/mcp.service";

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<AIActionCard>>,
  requests: [] as unknown[][],
  items: [] as AIActionCard[],
  search: "",
  replaced: [] as string[],
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: vi.fn(),
    replace: (url: string) => captured.replaced.push(url),
  }),
  usePathname: () => "/t/bayi/assistant/approvals",
  useSearchParams: () => new URLSearchParams(captured.search),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: { dateTime: (v: string) => v, relative: (v: string) => v },
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
    if (args[0] === "POST") {
      return { action_uuid: "x", status: "confirmed", source: "mcp" };
    }
    return {
      items: captured.items,
      total: captured.items.length,
      limit: 20,
      offset: 0,
    };
  },
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: unknown }) => children,
  EntityToolbar: () => null,
  EntityTable: (props: Partial<DataTableProps<AIActionCard>>) => {
    captured.table = props;
    return null;
  },
}));

import { PendingActionsPage } from "./pending-actions-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

function card(uuid: string, expiresInMs: number): AIActionCard {
  return {
    action_uuid: uuid,
    tool_use_id: "toolu_1",
    tool_name: "create_lead",
    source: "mcp",
    status: "pending",
    preview: {
      action: "create_lead",
      summary: "Lead for Ayşe",
      fields: [{ key: "contact_name", value: "Ayşe" }],
    },
    expires_at: new Date(Date.now() + expiresInMs).toISOString(),
    created_at: new Date().toISOString(),
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  captured.requests = [];
  captured.items = [];
  captured.search = "";
  captured.replaced = [];
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

async function flush() {
  for (let i = 0; i < 4; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(PendingActionsPage, { slug: "bayi" }),
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
const dialogButton = (id: string) =>
  document.body.querySelector<HTMLButtonElement>(`[data-testid="${id}"]`);

describe("PendingActionsPage", () => {
  it("lists with the default sort and maps the source filter, sort and q", async () => {
    await render();
    expect(lastGet()[1]).toBe("/v1/ai/pending-actions");
    expect(lastGet()[2].query).toEqual({
      limit: 20,
      offset: 0,
      sort: "-created_at",
    });
    expect(captured.table?.features?.persistKey).toBe(
      "tenant-ai-pending-actions-v1",
    );
    const cols = captured.table?.columns as ColumnDef<AIActionCard, unknown>[];
    const sortable = cols
      .filter((c) => c.enableSorting)
      .map((c) => c.id ?? ("accessorKey" in c ? c.accessorKey : ""));
    expect(sortable.sort()).toEqual(["created_at", "expires_at", "tool_name"]);

    await act(async () => {
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "source", value: ["mcp", "whatsapp"] },
      ]);
      captured.table?.state?.onSortingChange?.([
        { id: "expires_at", desc: false },
      ]);
      captured.table?.state?.onGlobalFilterChange?.("lead");
    });
    await flush();
    expect(lastGet()[2].query).toMatchObject({
      source: "mcp,whatsapp",
      sort: "expires_at",
      q: "lead",
      offset: 0,
    });
  });

  it("opens the card from ?action= and confirms through the pending-actions API", async () => {
    captured.items = [card("a-1", 10 * 60_000)];
    captured.search = "action=a-1";
    await render();
    const confirm = dialogButton("pending-action-confirm");
    expect(confirm).not.toBeNull();
    expect(confirm?.disabled).toBe(false);
    expect(
      document.body.querySelector('[data-testid="pending-action-countdown"]'),
    ).not.toBeNull();
    await act(async () => confirm?.click());
    await flush();
    expect(captured.requests).toContainEqual([
      "POST",
      "/v1/ai/pending-actions/a-1/confirm",
    ]);
    expect(captured.replaced.at(-1)).toBe("/t/bayi/assistant/approvals");
  });

  it("disables Approve once the action expired but still allows Reject", async () => {
    captured.items = [card("a-2", -1000)];
    captured.search = "action=a-2";
    await render();
    const confirm = dialogButton("pending-action-confirm");
    expect(confirm?.disabled).toBe(true);
    expect(
      document.body.querySelector('[data-testid="pending-action-expired"]'),
    ).not.toBeNull();
    await act(async () => confirm?.click());
    expect(captured.requests.some((c) => c[0] === "POST")).toBe(false);

    const reject = dialogButton("pending-action-cancel");
    expect(reject?.disabled).toBe(false);
    await act(async () => reject?.click());
    await flush();
    expect(captured.requests).toContainEqual([
      "POST",
      "/v1/ai/pending-actions/a-2/cancel",
    ]);
  });
});
