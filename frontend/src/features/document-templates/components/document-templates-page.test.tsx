// @vitest-environment jsdom
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type {
  DocumentTemplate,
  ListDocumentTemplatesParams,
} from "@/features/document-templates/services/document-templates.service";

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<DocumentTemplate>>,
  params: [] as ListDocumentTemplatesParams[],
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
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
vi.mock("@/components/common/permission-guard", () => ({
  PermissionGuard: () => null,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({ children }: { children: unknown }) => children,
  EntityToolbar: () => null,
  EntityTable: (props: Partial<DataTableProps<DocumentTemplate>>) => {
    captured.table = props;
    return null;
  },
}));
vi.mock("@/features/document-templates/hooks/use-document-templates", () => ({
  useDocumentTemplates: (params: ListDocumentTemplatesParams) => {
    captured.params.push(params);
    return {
      data: { items: [], total: 240, limit: 50, offset: 0 },
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    };
  },
}));

import {
  DocumentTemplatesPage,
  documentStatusParams,
} from "./document-templates-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  captured.params = [];
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function render() {
  await act(async () => {
    root.render(createElement(DocumentTemplatesPage));
  });
}

const last = () => captured.params[captured.params.length - 1];
const column = (id: string) =>
  (captured.table?.columns as ColumnDef<DocumentTemplate, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("DocumentTemplatesPage", () => {
  it("pages with offset (no 100-row cut) and keeps the backend default sort", async () => {
    await render();
    expect(last()).toEqual({ limit: 50, offset: 0 });
    expect(captured.table?.rowCount).toBe(240);
    await act(async () => {
      captured.table?.state?.onPaginationChange?.({
        pageIndex: 3,
        pageSize: 50,
      });
    });
    expect(last()).toEqual({ limit: 50, offset: 150 });
  });

  it("maps kind/language/status facets, brand and q", async () => {
    await render();
    await act(async () => {
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "kind", value: ["service", "warranty"] },
        { id: "language", value: ["de", "zh-CN"] },
        { id: "status", value: ["draft"] },
        { id: "brand", value: "acme" },
      ]);
      captured.table?.state?.onGlobalFilterChange?.("garantie");
    });
    expect(last()).toEqual({
      limit: 50,
      offset: 0,
      kind: "service,warranty",
      language: "de,zh-CN",
      status: "draft",
      brand: "acme",
      q: "garantie",
    });
  });

  it("asks for non-current versions when superseded is selected", () => {
    expect(documentStatusParams(["superseded", "active"])).toEqual({
      status: "superseded,active",
      current: "false",
    });
    expect(documentStatusParams([])).toEqual({
      status: undefined,
      current: undefined,
    });
  });

  it("sorts by the whitelisted fields", async () => {
    await render();
    for (const id of ["kind", "language", "name", "version", "updated_at"])
      expect(column(id)?.enableSorting, id).toBe(true);
    expect(column("status")?.enableSorting).toBe(false);
    await act(async () => {
      captured.table?.state?.onSortingChange?.([{ id: "version", desc: true }]);
    });
    expect(last().sort).toBe("-version");
  });
});
