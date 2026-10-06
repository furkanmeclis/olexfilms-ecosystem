// @vitest-environment jsdom
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import type {
  ListStorageParams,
  StorageObject,
} from "@/features/storage/types";

const captured = vi.hoisted(() => ({
  table: null as null | Partial<DataTableProps<StorageObject>>,
  listParams: [] as ListStorageParams[],
  items: [] as Partial<StorageObject>[],
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: { dateTime: (v: string) => v, date: (v: string) => v },
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
vi.mock("@/components/entity", async () => ({
  useServerListState: (
    await import("@/components/entity/use-server-list-state")
  ).useServerListState,
  EntityPage: ({ children }: { children: unknown }) => children,
  EntityTable: (props: Partial<DataTableProps<StorageObject>>) => {
    captured.table = props;
    return null;
  },
}));
const { nothing } = vi.hoisted(() => ({ nothing: () => null }));
vi.mock("@/features/storage/components/storage-sidebar", () => ({
  StorageSidebar: nothing,
}));
vi.mock("@/features/storage/components/storage-breadcrumb", () => ({
  StorageBreadcrumb: nothing,
}));
vi.mock("@/features/storage/components/storage-confirm-dialog", () => ({
  StorageConfirmDialog: nothing,
}));
vi.mock("@/features/storage/components/storage-details", () => ({
  StorageDetailsPanel: nothing,
}));
vi.mock("@/features/storage/components/storage-input-dialog", () => ({
  StorageInputDialog: nothing,
}));
vi.mock("@/features/storage/components/storage-share-dialog", () => ({
  StorageShareDialog: nothing,
}));
vi.mock("@/features/storage/components/storage-link-dialogs", () => ({
  StorageQrDialog: nothing,
}));
vi.mock("@/features/storage/components/storage-preview", () => ({
  StoragePreviewDialog: nothing,
}));
vi.mock("@/features/storage/components/storage-upload-manager", () => ({
  StorageUploadManager: nothing,
}));
vi.mock("@/features/storage/components/storage-upload-zone", () => ({
  StorageUploadZone: nothing,
}));
vi.mock("@/features/storage/components/storage-toolbar", () => ({
  StorageToolbar: nothing,
}));
vi.mock("@/features/storage/components/storage-empty-state", () => ({
  StorageEmptyState: nothing,
}));
vi.mock("@/features/storage/hooks/use-storage-upload", () => ({
  useStorageUpload: () => ({
    start: vi.fn(),
    jobs: [],
    cancel: vi.fn(),
    retry: vi.fn(),
    dismiss: vi.fn(),
  }),
}));
vi.mock("@/features/storage/hooks/use-storage", () => {
  const mutation = () => ({ mutate: vi.fn() });
  return {
    useStorageFiles: (params: ListStorageParams) => {
      captured.listParams.push(params);
      return {
        data: { items: captured.items, total: 240 },
        isLoading: false,
        isError: false,
        refetch: vi.fn(),
      };
    },
    useStorageUsage: () => ({ data: undefined }),
    useCreateFolder: mutation,
    useDeleteFiles: mutation,
    useRestoreFiles: mutation,
    usePurgeFiles: mutation,
    useRenameFile: mutation,
    useMoveFile: mutation,
    useCopyFile: mutation,
    useStarFile: mutation,
  };
});

import { StorageExplorer } from "./storage-explorer";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

const folder = {
  key: "docs/",
  name: "docs",
  kind: "folder",
  file_kind: "folder",
} as Partial<StorageObject>;
const file = {
  key: "a.pdf",
  name: "a.pdf",
  kind: "file",
  file_kind: "pdf",
  mime_type: "application/pdf",
} as Partial<StorageObject>;

beforeEach(() => {
  window.localStorage.clear();
  captured.table = null;
  captured.listParams = [];
  captured.items = [folder, file];
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
    root.render(createElement(StorageExplorer));
  });
}

const lastParams = () => captured.listParams[captured.listParams.length - 1]!;
const column = (id: string) =>
  (captured.table?.columns as ColumnDef<StorageObject, unknown>[]).find(
    (c) =>
      c.id === id ||
      ("accessorKey" in c && (c.accessorKey as string | undefined) === id),
  );

describe("StorageExplorer file table", () => {
  it("pages past the first 100 objects with the list total", async () => {
    await render();
    expect(lastParams()).toMatchObject({
      limit: 50,
      offset: 0,
      sort: "name",
      prefix: "",
      view: "all",
      recursive: false,
    });
    expect(captured.table?.rowCount).toBe(240);

    await act(async () => {
      captured.table?.state?.onPaginationChange?.({
        pageIndex: 2,
        pageSize: 100,
      });
    });
    expect(lastParams()).toMatchObject({ limit: 100, offset: 200 });
  });

  it("maps header sort and kind/access/modified filters to list params", async () => {
    await render();
    for (const id of ["name", "type", "size", "updated_at"]) {
      expect(column(id)?.enableSorting, id).toBe(true);
    }
    await act(async () => {
      captured.table?.state?.onSortingChange?.([{ id: "type", desc: true }]);
      captured.table?.state?.onColumnFiltersChange?.([
        { id: "type", value: "image" },
        { id: "access", value: "public" },
        { id: "updated_at", value: ["2026-01-01", "2026-02-01"] },
      ]);
      captured.table?.state?.onGlobalFilterChange?.("logo");
    });
    expect(lastParams()).toMatchObject({
      sort: "-type",
      kind: "image",
      access: "public",
      modified_from: "2026-01-01",
      modified_to: "2026-02-01",
      q: "logo",
      // Search looks into sub-folders.
      recursive: true,
    });
  });

  it("opening a folder navigates there and resets paging/selection", async () => {
    await render();
    await act(async () => {
      captured.table?.state?.onPaginationChange?.({
        pageIndex: 1,
        pageSize: 50,
      });
      captured.table?.state?.onRowSelectionChange?.({ "a.pdf": true });
    });
    expect(captured.table?.state?.rowSelection).toEqual({ "a.pdf": true });

    await act(async () => {
      captured.table?.onRowClick?.(folder as StorageObject);
    });
    expect(lastParams()).toMatchObject({ prefix: "docs/", offset: 0 });
    expect(captured.table?.state?.rowSelection).toEqual({});
    expect(captured.table?.renderGridItem).toBeTypeOf("function");
  });
});
