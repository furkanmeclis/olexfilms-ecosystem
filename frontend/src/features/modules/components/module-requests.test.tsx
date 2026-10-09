// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { act, createElement, type ReactElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ModuleRequestRow } from "@/features/modules/types";

type TableProps = {
  columns: ColumnDef<ModuleRequestRow, unknown>[];
  data: ModuleRequestRow[];
  rowCount?: number;
  features?: { persistKey?: string; rowSelection?: boolean };
  state?: {
    onRowSelectionChange?: (s: RowSelectionState) => void;
  };
  toolbarExtra?: ReactNode;
};

const captured = vi.hoisted(() => ({
  tables: [] as TableProps[],
  requests: vi.fn(),
  decideRequest: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: { dateTime: (v: string) => v },
  }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (props: TableProps) => {
    captured.tables.push(props);
    return props.toolbarExtra ?? null;
  },
}));
vi.mock("@/features/modules/services/modules.service", () => ({
  modulesService: {
    requests: captured.requests,
    decideRequest: captured.decideRequest,
  },
}));

import {
  MODULE_REQUESTS_PERSIST_KEY,
  ModuleRequestsTable,
} from "./module-requests";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

function requestRow(uuid: string, status: ModuleRequestRow["status"]) {
  return {
    uuid,
    organization_uuid: `org-${uuid}`,
    organization_name: `Bayi ${uuid}`,
    organization_type: "dealer",
    module_key: "fleet",
    note: "lütfen",
    status,
    requested_by_uuid: null,
    requested_by_name: "Ali",
    decided_by_uuid: null,
    decided_by_name: "",
    decision_note: "",
    created_at: "2026-10-01T10:00:00Z",
    decided_at: null,
  } satisfies ModuleRequestRow;
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  captured.requests.mockReset().mockResolvedValue({
    items: [requestRow("a", "pending"), requestRow("b", "pending")],
    total: 2,
    limit: 20,
    offset: 0,
  });
  captured.decideRequest.mockReset().mockResolvedValue({});
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

async function renderTable(scope: "tenant" | "platform", canDecide = true) {
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client: new QueryClient() },
        createElement(ModuleRequestsTable, {
          scope,
          canDecide,
          moduleKeys: ["fleet", "efficiency"],
        }),
      ),
    );
  });
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

const last = () => captured.tables.at(-1)!;
const column = (id: string) =>
  last().columns.find(
    (c) => (c.id ?? (c as { accessorKey?: string }).accessorKey) === id,
  )!;

function renderCell(id: string, row: ModuleRequestRow) {
  const host = document.createElement("div");
  document.body.appendChild(host);
  const r = createRoot(host);
  act(() =>
    r.render(
      (column(id).cell as (ctx: unknown) => ReactElement)({
        row: { original: row },
      }),
    ),
  );
  return { host, unmount: () => act(() => r.unmount()) };
}

describe("Module request queue (TEC-509)", () => {
  it("lists the tenant queue with filters, persistKey and pending by default", async () => {
    await renderTable("tenant");
    expect(captured.requests).toHaveBeenCalledWith(
      "tenant",
      expect.objectContaining({
        status: "pending",
        sort: "-created_at",
        limit: 20,
        offset: 0,
      }),
    );
    expect(last().features).toEqual({
      persistKey: MODULE_REQUESTS_PERSIST_KEY.tenant,
      rowSelection: true,
    });
    expect(last().rowCount).toBe(2);
    const meta = (id: string) =>
      (column(id).meta ?? {}) as { filterVariant?: string; param?: string };
    expect(meta("module_key")).toMatchObject({
      filterVariant: "faceted",
      param: "module_key",
    });
    expect(meta("status")).toMatchObject({ filterVariant: "faceted" });
    expect(meta("created_at")).toMatchObject({
      filterVariant: "date-range",
      param: "created",
    });
    expect(column("organization_name")).toBeDefined();
    expect(column("note")).toBeDefined();
  });

  it("uses the platform queue for the center", async () => {
    await renderTable("platform", false);
    expect(captured.requests).toHaveBeenCalledWith(
      "platform",
      expect.anything(),
    );
    expect(last().features?.persistKey).toBe(
      MODULE_REQUESTS_PERSIST_KEY.platform,
    );
    expect(last().features?.rowSelection).toBe(false);
    const { host, unmount } = renderCell("actions", requestRow("a", "pending"));
    expect(host.querySelector("button")).toBeNull();
    unmount();
  });

  it("row actions only for pending requests", async () => {
    await renderTable("tenant");
    const done = renderCell("actions", requestRow("a", "approved"));
    expect(done.host.querySelector("button")).toBeNull();
    done.unmount();
    const open = renderCell("actions", requestRow("a", "pending"));
    expect(open.host.querySelector("button")).not.toBeNull();
    open.unmount();
  });

  it("bulk approves the selected pending requests with the note", async () => {
    await renderTable("tenant");
    await act(async () =>
      last().state?.onRowSelectionChange?.({ a: true, b: true }),
    );
    const bulk = document.querySelector(
      '[data-testid="module-requests-bulk-approve"]',
    ) as HTMLButtonElement;
    expect(bulk).not.toBeNull();
    await act(async () => bulk.click());
    const note = document.querySelector(
      '[data-testid="module-request-note"]',
    ) as HTMLTextAreaElement;
    await act(async () => {
      Object.getOwnPropertyDescriptor(
        HTMLTextAreaElement.prototype,
        "value",
      )?.set?.call(note, "uygun");
      note.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () =>
      (
        document.querySelector(
          '[data-testid="module-request-submit"]',
        ) as HTMLButtonElement
      ).click(),
    );
    expect(captured.decideRequest).toHaveBeenCalledTimes(2);
    expect(captured.decideRequest).toHaveBeenCalledWith(
      "tenant",
      "a",
      "approve",
      "uygun",
    );
    expect(captured.decideRequest).toHaveBeenCalledWith(
      "tenant",
      "b",
      "approve",
      "uygun",
    );
  });
});
