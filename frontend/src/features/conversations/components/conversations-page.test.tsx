// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type {
  Conversation,
  ConversationListParams,
} from "@/features/conversations/services/conversations.service";

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<Conversation>>,
  listParams: [] as ConversationListParams[],
  bulk: null as null | { resource: string; paramOptions?: unknown },
  meta: undefined as undefined | { default_sort?: string },
  search: "",
  panel: null as null | string,
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/platform/conversations",
  useSearchParams: () => new URLSearchParams(captured.search),
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
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/features/conversations/hooks/use-conversations-realtime", () => ({
  useConversationsRealtime: () => undefined,
}));
vi.mock("@/features/conversations/components/conversation-panel", () => ({
  ConversationPanel: ({ uuid }: { uuid: string }) => {
    captured.panel = uuid;
    return null;
  },
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: unknown }) => children,
  EntityToolbar: () => null,
  EntityTable: (
    props: Partial<DataTableProps<Conversation>> & { toolbarExtra?: unknown },
  ) => {
    captured.table = props;
    return props.toolbarExtra ?? null;
  },
}));
vi.mock("@/features/bulk-engine", () => ({
  BulkActionMenu: (props: { resource: string; paramOptions?: unknown }) => {
    captured.bulk = props;
    return null;
  },
  SelectionBanner: () => null,
  resolveBulkActionsWithIcons: () => [],
  useBulkSelection: () => ({
    rowSelection: {},
    onRowSelectionChange: () => {},
    selectedCount: 0,
    showSelectAllBanner: false,
    scope: { mode: "none" },
    selectAllMatching: () => {},
    clearSelection: () => {},
  }),
}));
vi.mock("@/features/conversations/hooks/use-conversations", () => ({
  useConversationsList: (params: ConversationListParams) => {
    captured.listParams.push(params);
    return {
      data: { items: [], total: 42, limit: 20, offset: 0 },
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    };
  },
  useConversationsMeta: () => ({ data: captured.meta }),
  usePlatformAdmins: () => ({
    data: [
      { value: "u-1", label: "Ada Admin" },
      { value: "u-2", label: "Bora Admin" },
    ],
  }),
  useMarkConversationRead: () => ({ mutate: vi.fn() }),
  useSetConversationStatus: () => ({ mutate: vi.fn() }),
}));

import { ConversationsPage } from "./conversations-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  captured.listParams = [];
  captured.bulk = null;
  captured.meta = undefined;
  captured.search = "";
  captured.panel = null;
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
      createElement(
        QueryClientProvider,
        { client },
        createElement(ConversationsPage),
      ),
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
  (captured.table?.columns as ColumnDef<Conversation, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("ConversationsPage list params", () => {
  it("defaults to -last_message_at and passes total / persistKey", async () => {
    await render();
    expect(lastParams()).toEqual({
      limit: 20,
      offset: 0,
      sort: "-last_message_at",
    });
    expect(captured.table?.rowCount).toBe(42);
    expect(captured.table?.features?.persistKey).toBe(
      "platform-conversations-v1",
    );
    expect(captured.table?.features?.rowSelection).toBe(true);
    expect(captured.bulk?.resource).toBe("conversations");
  });

  it("sends faceted filters as CSV, unread as boolean and a date range", async () => {
    await render();
    await act(async () => {
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "status", value: ["open", "pending"] },
        { id: "ai_mode", value: ["paused"] },
        { id: "identity_kind", value: ["customer", "visitor"] },
        { id: "assigned_user", value: ["u-1", "u-2"] },
        { id: "unread_count", value: true },
        { id: "last_message_at", value: ["2026-10-01", "2026-10-07"] },
      ]);
    });
    expect(lastParams()).toMatchObject({
      status: "open,pending",
      ai_mode: "paused",
      identity_kind: "customer,visitor",
      assigned_user_uuid: "u-1,u-2",
      unread: "true",
      last_message_from: "2026-10-01",
      last_message_to: "2026-10-07",
      offset: 0,
    });
  });

  it("sorts by one whitelisted field and searches with q", async () => {
    await render();
    for (const id of ["last_message_at", "unread_count", "created_at"]) {
      expect(column(id)?.enableSorting, id).toBe(true);
    }
    for (const id of ["contact", "status", "ai_mode", "assigned_user"]) {
      expect(column(id)?.enableSorting, id).toBe(false);
    }
    await act(async () => {
      captured.table?.state?.onSortingChange?.([
        { id: "unread_count", desc: true },
      ]);
    });
    expect(lastParams().sort).toBe("-unread_count");
    await act(async () => {
      captured.table?.state?.onGlobalFilterChange?.("ayşe");
    });
    expect(lastParams().q).toBe("ayşe");
  });

  it("uses the meta default sort and admin assignees as filter options", async () => {
    captured.meta = { default_sort: "-created_at" };
    await render();
    expect(lastParams().sort).toBe("-created_at");
    const assigned = column("assigned_user");
    expect(assigned?.meta?.param).toBe("assigned_user_uuid");
    expect(assigned?.meta?.filterOptions?.map((o) => o.value)).toEqual([
      "u-1",
      "u-2",
    ]);
    expect(captured.bulk?.paramOptions).toEqual({
      assignee_user_uuid: [
        { value: "u-1", label: "Ada Admin" },
        { value: "u-2", label: "Bora Admin" },
      ],
    });
  });

  it("opens the selected conversation beside the list", async () => {
    captured.search = "c=0b9c4c1e-0000-4000-8000-0000000000a1";
    await render();
    expect(captured.panel).toBe("0b9c4c1e-0000-4000-8000-0000000000a1");
  });
});
