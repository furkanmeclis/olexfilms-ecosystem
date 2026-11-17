// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, Fragment, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type Row = { uuid: string } & Record<string, unknown>;

const state = vi.hoisted(() => ({
  list: vi.fn(),
  tables: [] as Record<string, unknown>[],
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) =>
      key === "activity.actions.user.updated" ? "User updated" : key,
    locale: "en",
    format: { dateTime: (v: string) => `dt(${v})` },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/features/io/services/activity.service", () => ({
  activityService: { list: state.list },
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  // Renders every cell of every row.
  EntityTable: (props: { columns: ColumnDef<Row>[]; data: Row[] }) => {
    state.tables.push(props);
    return createElement(
      "div",
      null,
      props.data.map((row) =>
        createElement(
          "div",
          { key: row.uuid, "data-row": row.uuid },
          props.columns.map((col, i) =>
            createElement(
              Fragment,
              { key: i },
              typeof col.cell === "function"
                ? (col.cell as (ctx: unknown) => ReactNode)({
                    row: { original: row },
                  })
                : null,
              " | ",
            ),
          ),
        ),
      ),
    );
  },
}));

import { UserActivitySection } from "./user-activity-section";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  state.tables = [];
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
});

describe("UserActivitySection (TEC-1218)", () => {
  it("lists the user's activity filtered by actor on the server", async () => {
    state.list.mockResolvedValue({
      items: [
        {
          uuid: "a-1",
          action: "user.updated",
          resource: "users",
          resource_uuid: "7c0ffee0-1111-2222-3333-444455556666",
          payload: {},
          created_at: "2026-11-10T09:00:00Z",
        },
        {
          uuid: "a-2",
          action: "order.created",
          resource: "orders",
          payload: { via: "ai" },
          created_at: "2026-11-09T09:00:00Z",
        },
      ],
      total: 2,
      limit: 10,
      offset: 0,
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    await act(async () => {
      root.render(
        createElement(
          QueryClientProvider,
          { client },
          createElement(UserActivitySection, { uuid: "user-1" }),
        ),
      );
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });

    expect(state.list).toHaveBeenCalledWith(
      expect.objectContaining({ actor: "user-1", sort: "-created_at" }),
    );
    const first = container.querySelector("[data-row='a-1']");
    expect(first?.textContent).toContain("dt(2026-11-10T09:00:00Z)");
    expect(first?.textContent).toContain("User updated");
    expect(first?.textContent).toContain("#7c0ffee0");
    const second = container.querySelector("[data-row='a-2']");
    // Unknown action key falls back to the raw action; AI badge shown.
    expect(second?.textContent).toContain("order.created");
    expect(second?.textContent).toContain("activity.via_ai");
    expect(container.textContent).toContain("users.detail.activity");
  });
});
