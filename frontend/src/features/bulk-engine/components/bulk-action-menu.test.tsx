// @vitest-environment jsdom
import { ListChecks } from "lucide-react";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  mutateAsync: vi.fn(),
  confirm: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key, locale: "en" }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/providers/dialog-provider", () => ({
  useDialogs: () => ({ confirm: mocks.confirm, confirmDelete: mocks.confirm }),
}));
vi.mock("@/features/bulk-engine/hooks/use-bulk-mutation", () => ({
  useBulkMutation: () => ({ mutateAsync: mocks.mutateAsync, isPending: false }),
}));

import type { BulkActionDef } from "@/features/bulk-engine/types";

import { BulkActionMenu } from "./bulk-action-menu";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const SET_STATUS: BulkActionDef = {
  id: "set_status",
  label_key: "bulk.actions.leads.set_status",
  permission: "leads.write",
  params: [
    {
      key: "status",
      kind: "enum",
      required: true,
      label_key: "bulk.params.lead_status",
      options: ["new", "lost"],
    },
    { key: "lost_reason", kind: "text", label_key: "bulk.params.lost_reason" },
  ],
  icon: ListChecks,
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  mocks.mutateAsync.mockReset();
  mocks.confirm.mockReset();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

describe("BulkActionMenu params (TEC-372)", () => {
  it("renders a select for enum params and a textarea for text params", async () => {
    await act(async () => {
      root.render(
        createElement(BulkActionMenu, {
          resource: "leads",
          actions: [SET_STATUS],
          scope: { mode: "page", ids: ["l1"] },
          selectedCount: 1,
        }),
      );
    });
    const button = container.querySelector("button");
    await act(async () => button?.click());

    const status = document.querySelector("[data-param=status]");
    expect(
      status?.querySelector("[role=combobox]")?.getAttribute("aria-label"),
    ).toBe("bulk.params.lead_status");
    const reason = document.querySelector(
      "[data-param=lost_reason] textarea",
    ) as HTMLTextAreaElement | null;
    expect(reason).not.toBeNull();

    // The required enum is still empty: nothing runs.
    const confirm = [...document.querySelectorAll("button")].find(
      (b) => b.textContent === "common.confirm",
    );
    await act(async () => confirm?.click());
    expect(mocks.mutateAsync).not.toHaveBeenCalled();
  });
});
