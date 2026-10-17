// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, Fragment, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";

const captured = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<{ uuid: string }>>[],
}));
const api = vi.hoisted(() => ({
  locationMapsPage: vi.fn(),
  createLocationMap: vi.fn(),
  updateLocationMap: vi.fn(),
  deleteLocationMap: vi.fn(),
  mapWarehouses: vi.fn(),
  mapLocations: vi.fn(),
}));
const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key }),
  useFormatter: () => ({ dateTime: (v: string) => v }),
}));
vi.mock("@/providers/toast-provider", () => ({ appToast: toast }));
vi.mock("@/providers/dialog-provider", () => ({
  useDialogs: () => ({ confirmDelete: vi.fn() }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: (
    props: Partial<DataTableProps<{ uuid: string }>> & {
      toolbarExtra?: ReactNode;
    },
  ) => {
    captured.tables.push(props);
    return createElement("div", null, props.toolbarExtra);
  },
}));
vi.mock("@/components/ui/dialog", () => {
  const Pass = ({ children }: { children?: ReactNode }) =>
    createElement(Fragment, null, children);
  return {
    Dialog: ({ open, children }: { open: boolean; children: ReactNode }) =>
      open ? createElement("div", { role: "dialog" }, children) : null,
    DialogContent: Pass,
    DialogDescription: Pass,
    DialogFooter: Pass,
    DialogHeader: Pass,
    DialogTitle: Pass,
  };
});
// The popover combobox is replaced by a plain input bound to the form.
vi.mock("@/components/forms", async (orig) => {
  const { useFormContext } = await import("react-hook-form");
  return {
    ...(await orig<object>()),
    AppCombobox: ({
      name,
      onValueChange,
      "data-testid": testId,
    }: {
      name: string;
      onValueChange?: (value: string) => void;
      "data-testid"?: string;
    }) => {
      const { register, formState } = useFormContext();
      const error = formState.errors[name]?.message as string | undefined;
      return createElement(
        Fragment,
        null,
        createElement("input", {
          "data-testid": testId,
          ...register(name, {
            onChange: (e: { target: { value: string } }) =>
              onValueChange?.(e.target.value),
          }),
        }),
        error
          ? createElement("p", { "data-testid": `${name}-error` }, error)
          : null,
      );
    },
  };
});
vi.mock(
  "@/features/integrations/glorian/services/glorian.service",
  async (orig) => ({
    ...(await orig<object>()),
    glorianService: api,
  }),
);

import type { GlorianLocationMap } from "@/features/integrations/glorian/services/glorian.service";
import { ApiError } from "@/lib/api/errors";

import {
  GlorianLocationMapDialog,
  GlorianLocationMaps,
} from "./glorian-location-maps";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const WH = "0b9c4c1e-0000-4000-8000-0000000000a1";
const WH2 = "0b9c4c1e-0000-4000-8000-0000000000a2";
const BIN = "0b9c4c1e-0000-4000-8000-0000000000b1";

const row: GlorianLocationMap = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000c1",
  remote_location_code: "IST-MAIN",
  warehouse: { uuid: WH, code: "IST", name: "Istanbul", active: true },
  location: { uuid: BIN, code: "A1", full_code: "IST-A-A1", name: "Raf A1" },
  created_at: "2026-10-01T09:00:00Z",
  updated_at: "2026-10-01T09:00:00Z",
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  for (const fn of [...Object.values(api), ...Object.values(toast)]) {
    fn.mockReset();
  }
  api.locationMapsPage.mockResolvedValue({
    items: [row],
    total: 1,
    limit: 20,
    offset: 0,
  });
  api.mapWarehouses.mockResolvedValue([row.warehouse]);
  api.mapLocations.mockResolvedValue([row.location]);
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

async function render(node: ReturnType<typeof createElement>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await flush();
}

const $ = (id: string) =>
  container.querySelector<HTMLInputElement>(`[data-testid="${id}"]`);
const lastTable = () => captured.tables[captured.tables.length - 1];
const columnIds = () =>
  (lastTable().columns ?? []).map(
    (c) => c.id ?? (c as { accessorKey?: string }).accessorKey,
  );

async function type(id: string, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )!.set!;
  await act(async () => {
    setter.call($(id), value);
    $(id)!.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function submit() {
  await act(async () => {
    $("location-map-save")!.click();
  });
  await flush();
}

describe("GlorianLocationMaps list", () => {
  it("renders rows with sort, q and row actions for managers", async () => {
    await render(createElement(GlorianLocationMaps, { canManage: true }));
    expect(api.locationMapsPage).toHaveBeenLastCalledWith({
      sort: "remote_location_code",
      limit: 20,
      offset: 0,
    });
    expect(lastTable().data).toEqual([row]);
    expect(lastTable().rowCount).toBe(1);
    expect(lastTable().features?.persistKey).toBe(
      "platform-glorian-location-maps-v1",
    );
    expect(columnIds()).toEqual([
      "remote_location_code",
      "warehouse",
      "location",
      "created_at",
      "actions",
    ]);
    expect($("location-map-create")).not.toBeNull();

    await act(async () => {
      lastTable().state?.onGlobalFilterChange?.("IST");
      lastTable().state?.onSortingChange?.([{ id: "warehouse", desc: true }]);
    });
    await flush();
    expect(api.locationMapsPage).toHaveBeenLastCalledWith({
      q: "IST",
      sort: "-warehouse",
      limit: 20,
      offset: 0,
    });
  });

  it("hides create / edit / delete without manage permission", async () => {
    await render(createElement(GlorianLocationMaps, { canManage: false }));
    expect(columnIds()).not.toContain("actions");
    expect($("location-map-create")).toBeNull();
  });

  it("opens the create dialog from the toolbar", async () => {
    await render(createElement(GlorianLocationMaps, { canManage: true }));
    await act(async () => {
      $("location-map-create")!.click();
    });
    await flush();
    expect(container.querySelector('[role="dialog"]')).not.toBeNull();
    expect($("location-map-remote")).not.toBeNull();
  });
});

describe("GlorianLocationMapDialog", () => {
  it("submits the create body and clears the bin on warehouse change", async () => {
    api.createLocationMap.mockResolvedValue(row);
    const onOpenChange = vi.fn();
    await render(
      createElement(GlorianLocationMapDialog, { open: true, onOpenChange }),
    );
    await type("location-map-remote", " IST-MAIN ");
    await type("location-map-warehouse", WH2);
    await type("location-map-location", BIN);
    await type("location-map-warehouse", WH);
    await flush();
    expect($("location-map-location")!.value).toBe("");
    await type("location-map-location", BIN);
    await submit();
    expect(api.createLocationMap).toHaveBeenCalledWith({
      remote_location_code: "IST-MAIN",
      warehouse_uuid: WH,
      location_uuid: BIN,
    });
    expect(toast.success).toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("updates without a bin and shows the taken code on the field", async () => {
    api.updateLocationMap.mockRejectedValue(
      new ApiError({
        status: 409,
        code: "GLORIAN_REMOTE_LOCATION_TAKEN",
        message: "taken",
      }),
    );
    await render(
      createElement(GlorianLocationMapDialog, {
        open: true,
        onOpenChange: () => {},
        map: row,
      }),
    );
    expect($("location-map-remote")!.value).toBe("IST-MAIN");
    await type("location-map-location", "");
    await submit();
    expect(api.updateLocationMap).toHaveBeenCalledWith(row.uuid, {
      remote_location_code: "IST-MAIN",
      warehouse_uuid: WH,
      location_uuid: null,
    });
    expect(toast.error).toHaveBeenCalledWith(
      "integrations.glorian.errors.remote_location_taken",
    );
    expect(
      container.textContent?.includes(
        "integrations.glorian.errors.remote_location_taken",
      ),
    ).toBe(true);
  });

  it("requires the remote code and the warehouse", async () => {
    await render(
      createElement(GlorianLocationMapDialog, {
        open: true,
        onOpenChange: () => {},
      }),
    );
    await submit();
    expect(api.createLocationMap).not.toHaveBeenCalled();
    expect($("warehouse_uuid-error")?.textContent).toBe(
      "integrations.glorian.validation.required",
    );
  });
});
