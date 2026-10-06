// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type { NotificationDelivery } from "@/features/notification-center/services/notification-center.service";

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<NotificationDelivery>>,
  deliveries: vi.fn(),
  events: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: { dateTime: (v: string) => v },
  }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (props: Partial<DataTableProps<NotificationDelivery>>) => {
    captured.table = props;
    return null;
  },
}));
vi.mock(
  "@/features/notification-center/services/notification-center.service",
  () => ({
    notificationCenterService: {
      deliveries: captured.deliveries,
      events: captured.events,
    },
  }),
);

import { DeliveriesTable } from "./deliveries-table";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  captured.deliveries.mockReset();
  captured.deliveries.mockResolvedValue({
    items: [],
    total: 130,
    limit: 25,
    offset: 0,
  });
  captured.events.mockResolvedValue([
    { code: "orders.created" },
    { code: "tasks.assigned" },
  ]);
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
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(DeliveriesTable),
      ),
    );
  });
  await flush();
}

const lastCall = () =>
  captured.deliveries.mock.calls[captured.deliveries.mock.calls.length - 1][0];
const column = (id: string) =>
  (captured.table?.columns as ColumnDef<NotificationDelivery, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("DeliveriesTable", () => {
  it("loads page 1 sorted by -created_at and passes the total", async () => {
    await render();
    expect(lastCall()).toEqual({ limit: 25, offset: 0, sort: "-created_at" });
    expect(captured.table?.rowCount).toBe(130);
  });

  it("maps status/channel CSV, event select, created range and q", async () => {
    await render();
    expect(
      column("event_code")?.meta?.filterOptions?.map((o) => o.value),
    ).toEqual(["orders.created", "tasks.assigned"]);
    await act(async () => {
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "status", value: ["failed", "skipped_disabled"] },
        { id: "channel", value: ["email"] },
        { id: "event_code", value: "orders.created" },
        { id: "created_at", value: ["2026-10-01", undefined] },
      ]);
      captured.table?.state?.onGlobalFilterChange?.("ada@example.com");
    });
    await flush();
    expect(lastCall()).toEqual({
      limit: 25,
      offset: 0,
      sort: "-created_at",
      status: "failed,skipped_disabled",
      channel: "email",
      event_code: "orders.created",
      created_from: "2026-10-01",
      q: "ada@example.com",
    });
  });

  it("sorts by status / channel / event code only", async () => {
    await render();
    expect(column("recipient")?.enableSorting).toBe(false);
    await act(async () => {
      captured.table?.state?.onSortingChange?.([
        { id: "event_code", desc: false },
      ]);
    });
    await flush();
    expect(lastCall().sort).toBe("event_code");
    await act(async () => {
      captured.table?.state?.onPaginationChange?.({
        pageIndex: 2,
        pageSize: 25,
      });
    });
    await flush();
    expect(lastCall()).toMatchObject({ offset: 50, limit: 25 });
  });
});
