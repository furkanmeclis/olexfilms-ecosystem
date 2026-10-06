// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type {
  ListPlatformNotificationsParams,
  Notification,
} from "@/features/notifications/services/notifications.service";

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<Notification>>,
  listParams: [] as ListPlatformNotificationsParams[],
  exportQuery: null as null | Record<string, unknown>,
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: { dateTime: (v: string) => v, date: (v: string) => v },
  }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: unknown }) => children,
  EntityToolbar: () => null,
  EntityTable: (
    props: Partial<DataTableProps<Notification>> & { toolbarExtra?: unknown },
  ) => {
    captured.table = props;
    return props.toolbarExtra ?? null;
  },
}));
vi.mock("@/features/io", () => ({
  ResourceIOToolbar: ({ query }: { query: Record<string, unknown> }) => {
    captured.exportQuery = query;
    return null;
  },
}));
vi.mock(
  "@/features/notifications/components/notification-audience-filter",
  () => ({ NotificationAudienceFilter: () => null }),
);
vi.mock(
  "@/features/notifications/components/notification-detail-drawer",
  () => ({ NotificationDetailDrawer: () => null }),
);
vi.mock("@/features/notifications/hooks/use-notification-realtime", () => ({
  useNotificationRealtimeInvalidate: () => {},
}));
vi.mock("@/features/notifications/hooks/use-notifications-query", () => ({
  usePlatformNotificationsList: (params: ListPlatformNotificationsParams) => {
    captured.listParams.push(params);
    return {
      data: { items: [], total: 42, limit: 20, offset: 0 },
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    };
  },
  usePlatformNotificationsMeta: () => ({ data: undefined }),
}));

import { NotificationsPage } from "./notifications-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  captured.listParams = [];
  captured.exportQuery = null;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function render() {
  const client = new QueryClient();
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(NotificationsPage),
      ),
    );
  });
}

const lastParams = () => captured.listParams[captured.listParams.length - 1];
const column = (id: string) =>
  (captured.table?.columns as ColumnDef<Notification, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("NotificationsPage list params", () => {
  it("opens on the in-app channel facet and passes the total", async () => {
    await render();
    expect(lastParams()).toEqual({
      limit: 20,
      offset: 0,
      sort: "-created_at",
      channel: "inapp",
      scope: "me",
      user_uuid: undefined,
    });
    expect(captured.table?.rowCount).toBe(42);
    expect(captured.table?.features?.persistKey).toBe(
      "platform-notifications-v4",
    );
  });

  it("maps status/channel/priority facets to CSV and created to a range", async () => {
    await render();
    await act(async () => {
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "status", value: ["failed", "sent"] },
        { id: "channel", value: ["email", "sms"] },
        { id: "priority", value: ["high", "critical"] },
        { id: "created_at", value: ["2026-10-01", "2026-10-05"] },
      ]);
    });
    expect(lastParams()).toMatchObject({
      status: "failed,sent",
      channel: "email,sms",
      priority: "high,critical",
      created_from: "2026-10-01",
      created_to: "2026-10-05",
      offset: 0,
    });
    // Export gets the same filters.
    expect(captured.exportQuery).toMatchObject({
      status: "failed,sent",
      channel: "email,sms",
      priority: "high,critical",
      created_from: "2026-10-01",
      scope: "me",
    });
  });

  it("sorts only whitelisted columns, one field at a time", async () => {
    await render();
    for (const id of ["status", "priority", "channel", "created_at", "sent_at"])
      expect(column(id)?.enableSorting, id).toBe(true);
    expect(column("title")?.enableSorting).toBe(false);
    await act(async () => {
      captured.table?.state?.onSortingChange?.([
        { id: "priority", desc: true },
      ]);
    });
    expect(lastParams().sort).toBe("-priority");
  });

  it("sends the toolbar search as q", async () => {
    await render();
    await act(async () => {
      captured.table?.state?.onGlobalFilterChange?.("invoice");
    });
    expect(lastParams().q).toBe("invoice");
  });
});
