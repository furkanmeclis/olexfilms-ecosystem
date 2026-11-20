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
  dealersPage: vi.fn(),
  dealerOrganizations: vi.fn(),
  linkDealer: vi.fn(),
  setDealerCode: vi.fn(),
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
vi.mock("@/lib/toast", () => ({ appToast: toast }));
vi.mock("@/providers/dialog-provider", () => ({
  useDialogs: () => ({ confirm: vi.fn() }),
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

import type { GlorianDealer } from "@/features/integrations/glorian/services/glorian.service";
import { ApiError } from "@/lib/api/errors";

import {
  GlorianDealerLinkDialog,
  GlorianDealers,
  needsAttention,
} from "./glorian-dealers";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
// The radio group (Radix) measures itself; jsdom has no ResizeObserver.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

const ORG = "0b9c4c1e-0000-4000-8000-0000000000a1";
const ORG2 = "0b9c4c1e-0000-4000-8000-0000000000a2";

const coded: GlorianDealer = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000d1",
  remote_id: "17",
  name: "Parazit Dizayn",
  dealer_code: "PD-01",
  phone: "+905550000001",
  active: true,
  match_type: "none",
  organization: null,
  synced_at: "2026-11-01T09:00:00Z",
  linked_at: null,
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  captured.tables = [];
  for (const fn of [...Object.values(api), ...Object.values(toast)]) {
    fn.mockReset();
  }
  api.dealersPage.mockResolvedValue({
    items: [coded],
    total: 1,
    limit: 20,
    offset: 0,
  });
  api.dealerOrganizations.mockResolvedValue([]);
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
    $("dealer-link-save")!.click();
  });
  await flush();
}

describe("GlorianDealers list", () => {
  it("opens on the dealers that need a decision", async () => {
    await render(createElement(GlorianDealers, { canManage: true }));
    expect(api.dealersPage).toHaveBeenLastCalledWith({
      match_type: "ambiguous,none",
      sort: "name",
      limit: 20,
      offset: 0,
    });
    expect(lastTable().data).toEqual([coded]);
    expect(lastTable().features?.persistKey).toBe(
      "platform-glorian-dealers-v1",
    );
    expect(columnIds()).toEqual([
      "name",
      "dealer_code",
      "phone",
      "match_type",
      "organization",
      "active",
      "synced_at",
      "actions",
    ]);
  });

  it("has no row actions without manage permission", async () => {
    await render(createElement(GlorianDealers, { canManage: false }));
    expect(columnIds()).not.toContain("actions");
  });

  it("flags only active unmatched or ambiguous dealers", () => {
    expect(needsAttention({ match_type: "none", active: true })).toBe(true);
    expect(needsAttention({ match_type: "ambiguous", active: true })).toBe(
      true,
    );
    expect(needsAttention({ match_type: "none", active: false })).toBe(false);
    expect(needsAttention({ match_type: "code", active: true })).toBe(false);
  });
});

describe("GlorianDealerLinkDialog", () => {
  it("writes the dealer code onto the organization by default", async () => {
    api.setDealerCode.mockResolvedValue({});
    const onOpenChange = vi.fn();
    await render(
      createElement(GlorianDealerLinkDialog, { dealer: coded, onOpenChange }),
    );
    await type("dealer-link-organization", ORG);
    await submit();
    expect(api.setDealerCode).toHaveBeenCalledWith(ORG, "PD-01");
    expect(api.linkDealer).not.toHaveBeenCalled();
    expect(toast.success).toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("clears a manual link so the written code applies", async () => {
    api.setDealerCode.mockResolvedValue({});
    api.linkDealer.mockResolvedValue({});
    const manual: GlorianDealer = {
      ...coded,
      match_type: "manual",
      organization: {
        uuid: ORG2,
        name: "Eski",
        city: "",
        phone: "",
        glorian_dealer_code: null,
      },
    };
    await render(
      createElement(GlorianDealerLinkDialog, {
        dealer: manual,
        onOpenChange: () => {},
      }),
    );
    expect($("dealer-link-organization")!.value).toBe(ORG2);
    await type("dealer-link-organization", ORG);
    await submit();
    expect(api.setDealerCode).toHaveBeenCalledWith(ORG, "PD-01");
    expect(api.linkDealer).toHaveBeenCalledWith(manual.uuid, null);
  });

  it("links a dealer without a code by hand", async () => {
    api.linkDealer.mockResolvedValue({});
    await render(
      createElement(GlorianDealerLinkDialog, {
        dealer: { ...coded, dealer_code: null },
        onOpenChange: () => {},
      }),
    );
    await type("dealer-link-organization", ORG);
    await submit();
    expect(api.linkDealer).toHaveBeenCalledWith(coded.uuid, ORG);
    expect(api.setDealerCode).not.toHaveBeenCalled();
  });

  it("requires an organization and shows the taken code", async () => {
    await render(
      createElement(GlorianDealerLinkDialog, {
        dealer: coded,
        onOpenChange: () => {},
      }),
    );
    await submit();
    expect(api.setDealerCode).not.toHaveBeenCalled();
    expect($("organization_uuid-error")?.textContent).toBe(
      "integrations.glorian.validation.required",
    );

    api.setDealerCode.mockRejectedValue(
      new ApiError({
        status: 409,
        code: "GLORIAN_DEALER_CODE_TAKEN",
        message: "taken",
      }),
    );
    await type("dealer-link-organization", ORG);
    await submit();
    expect(toast.error).toHaveBeenCalledWith(
      "integrations.glorian.errors.dealer_code_taken",
    );
  });
});
