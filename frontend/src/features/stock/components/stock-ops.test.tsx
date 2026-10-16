// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, Fragment, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  unitHistory: vi.fn(),
  listReclassifications: vi.fn(),
  getReclassification: vi.fn(),
  requestReclassification: vi.fn(),
  decideReclassification: vi.fn(),
  split: vi.fn(),
}));
const catalog = vi.hoisted(() => ({ listProducts: vi.fn() }));
const nav = vi.hoisted(() => ({ push: vi.fn() }));
const dialogs = vi.hoisted(() => ({ confirm: vi.fn(), prompt: vi.fn() }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
const stepUp = vi.hoisted(() => ({ ensure: vi.fn() }));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  user: "user-b",
  org: "org-center",
}));
const captured = vi.hoisted(() => ({
  rowActions: [] as string[][],
  combos: [] as Record<string, unknown>[],
}));

vi.mock("next/navigation", () => ({
  useRouter: () => nav,
  useParams: () => ({ slug: "acme" }),
}));
vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: { href: string; children: unknown } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({
    uuid: state.org,
    slug: "acme",
    type: "center",
  }),
}));
vi.mock("@/providers/auth-provider", () => ({
  useAuth: () => ({ user: { uuid: state.user } }),
}));
vi.mock("@/features/step-up-engine", () => ({ useStepUp: () => stepUp }));
vi.mock("@/providers/dialog-provider", () => ({ useDialogs: () => dialogs }));
vi.mock("@/providers/toast-provider", () => ({ appToast: toast }));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/features/stock/services/stock-ops.service", async (orig) => ({
  ...(await orig<object>()),
  stockOpsService: api,
}));
vi.mock("@/features/catalog/services/catalog.service", () => ({
  catalogService: catalog,
}));
vi.mock("@/components/ui/async-combobox", () => ({
  AsyncCombobox: (props: Record<string, unknown>) => {
    captured.combos.push(props);
    return null;
  },
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityRowActions: ({ actions }: { actions: { id: string }[] }) => {
    captured.rowActions.push(actions.map((a) => a.id));
    return null;
  },
  EntityTable: (props: {
    columns: ColumnDef<unknown, unknown>[];
    data: unknown[];
  }) =>
    createElement(
      "div",
      null,
      ...props.data.map((row, i) =>
        createElement(
          "div",
          { key: i },
          ...props.columns
            .filter((c) => typeof c.cell === "function")
            .map((c, j) =>
              createElement(
                Fragment,
                { key: j },
                (c.cell as (ctx: unknown) => ReactNode)({
                  row: { original: row },
                }),
              ),
            ),
        ),
      ),
    ),
}));

import { Permission } from "@/config/permissions";
import type { StockReclassification } from "@/features/stock/services/stock-ops.service";

import { ReclassificationDetailPage } from "./reclassification-detail-page";
import { ReclassificationsPage } from "./reclassifications-page";
import { ReclassifyDialog } from "./reclassify-dialog";
import { SplitDialog } from "./split-dialog";
import { UnitOperations } from "./unit-operations";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  captured.rowActions = [];
  captured.combos = [];
  stepUp.ensure.mockResolvedValue(true);
  dialogs.confirm.mockResolvedValue(true);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.clearAllMocks();
  state.grants = new Set();
  state.user = "user-b";
  state.org = "org-center";
});

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render(node: ReturnType<typeof createElement>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await flush();
}

const $ = (id: string) =>
  document.querySelector<HTMLElement>(`[data-testid="${id}"]`);

async function click(el: HTMLElement | null) {
  expect(el).not.toBeNull();
  await act(async () => {
    el!.click();
  });
  await flush();
}

async function type(el: HTMLElement | null, value: string) {
  expect(el).not.toBeNull();
  const input = el as HTMLInputElement | HTMLTextAreaElement;
  const proto = Object.getPrototypeOf(input);
  const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
  await act(async () => {
    setter?.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

function request(
  patch: Partial<StockReclassification> = {},
): StockReclassification {
  const product = (uuid: string, name: string) => ({
    uuid,
    sku: name.toUpperCase(),
    name,
    unit_type: "piece" as const,
    uses_fixed_barcode: false,
  });
  return {
    uuid: "r-1",
    status: "pending",
    unit: { uuid: "u-1", barcode: "OLX-1" },
    organization: { uuid: "org-dealer", name: "Dealer", type: "dealer" },
    from_product: product("p-a", "Film A"),
    to_product: product("p-b", "Film B"),
    reason: "wrong label",
    requested_by: { uuid: "user-a", name: "Ayşe" },
    decided_by: null,
    decided_at: null,
    decision_note: null,
    movement_uuid: null,
    created_at: "2026-10-01T09:00:00Z",
    ...patch,
  };
}

const ROLL = {
  barcode: "OLX-ROLL-1",
  status: "available",
  remaining_meters: "37.50",
  product: {
    uuid: "p-r",
    name: "PPF",
    sku: "PPF",
    unit_type: "roll_meter",
    uses_fixed_barcode: false,
  },
};

describe("UnitOperations gates (TEC-763)", () => {
  it("offers nothing without stock.write", async () => {
    state.grants = new Set([Permission.StockRead]);
    await render(createElement(UnitOperations, { slug: "acme", unit: ROLL }));
    expect($("unit-reclassify")).toBeNull();
    expect($("unit-split")).toBeNull();
  });

  it("offers fix and split on a roll with stock.write", async () => {
    state.grants = new Set([Permission.StockRead, Permission.StockWrite]);
    await render(
      createElement(UnitOperations, {
        slug: "acme",
        unit: ROLL,
        showHistory: true,
      }),
    );
    expect($("unit-reclassify")).not.toBeNull();
    expect($("unit-split")).not.toBeNull();
    expect($("unit-history-link")?.getAttribute("href")).toBe(
      "/t/acme/stock/units/OLX-ROLL-1",
    );
  });

  it("does not split pieces or fix fixed barcodes", async () => {
    state.grants = new Set([Permission.StockWrite]);
    await render(
      createElement(UnitOperations, {
        slug: "acme",
        unit: {
          ...ROLL,
          product: {
            ...ROLL.product,
            unit_type: "piece",
            uses_fixed_barcode: true,
          },
        },
      }),
    );
    expect($("unit-operations")).toBeNull();
  });
});

describe("SplitDialog (TEC-763)", () => {
  const open = () =>
    render(
      createElement(SplitDialog, {
        slug: "acme",
        barcode: "OLX-ROLL-1",
        remaining: "37.50",
        open: true,
        onOpenChange: () => undefined,
      }),
    );

  it("refuses more meters than the roll has", async () => {
    await open();
    await type(document.getElementById("split-meters"), "40");
    await click($("split-submit"));
    expect($("split-error")?.textContent).toContain(
      "stock.split.errors.exceeds",
    );
    expect(api.split).not.toHaveBeenCalled();
  });

  it("previews the rest, splits with a key and offers the new label", async () => {
    api.split.mockResolvedValue({
      uuid: "s-1",
      meters: "12.50",
      source: {
        uuid: "u-1",
        barcode: "OLX-ROLL-1",
        status: "available",
        remaining_meters: "25.00",
      },
      new_unit: {
        uuid: "u-2",
        barcode: "OLX-ROLL-1-S1",
        status: "available",
        remaining_meters: "12.50",
      },
      replayed: false,
      created_at: "2026-10-01T09:00:00Z",
    });
    await open();
    await type(document.getElementById("split-meters"), "12.5");
    expect($("split-preview")?.textContent).toContain('"left":"25.00"');
    await click($("split-submit"));
    expect(api.split).toHaveBeenCalledWith(
      expect.objectContaining({
        barcode: "OLX-ROLL-1",
        meters: "12.50",
        idempotency_key: expect.stringMatching(/^split:/),
      }),
    );
    expect($("split-done")?.textContent).toContain("OLX-ROLL-1-S1");
    expect($("split-label")).not.toBeNull();
    expect($("split-open-new")?.getAttribute("href")).toBe(
      "/t/acme/stock/units/OLX-ROLL-1-S1",
    );
  });
});

describe("ReclassifyDialog (TEC-763)", () => {
  it("validates the target and reason before sending", async () => {
    await render(
      createElement(ReclassifyDialog, {
        slug: "acme",
        barcode: "OLX-1",
        product: ROLL.product,
        open: true,
        onOpenChange: () => undefined,
      }),
    );
    await click($("reclass-submit"));
    const alerts = [...document.querySelectorAll('[role="alert"]')].map(
      (a) => a.textContent,
    );
    expect(alerts).toEqual([
      "stock.reclass.errors.target_required",
      "stock.reclass.errors.reason_required",
    ]);
    expect(api.requestReclassification).not.toHaveBeenCalled();

    // A picked product and a reason send the request.
    const combo = captured.combos.at(-1) as {
      onValueChange: (v: string) => void;
    };
    await act(async () => combo.onValueChange("p-b"));
    await type(document.getElementById("reclass-reason"), "wrong label");
    api.requestReclassification.mockResolvedValue(request());
    await click($("reclass-submit"));
    expect(api.requestReclassification).toHaveBeenCalledWith({
      barcode: "OLX-1",
      to_product_uuid: "p-b",
      reason: "wrong label",
    });
    expect($("reclass-created")).not.toBeNull();
  });
});

describe("ReclassificationDetailPage (TEC-157 four eyes)", () => {
  it("lets another center user approve after a step-up", async () => {
    state.grants = new Set([Permission.StockRead, Permission.StockReclassify]);
    api.getReclassification.mockResolvedValue(request());
    api.decideReclassification.mockResolvedValue(
      request({ status: "approved" }),
    );
    await render(
      createElement(ReclassificationDetailPage, { slug: "acme", uuid: "r-1" }),
    );
    expect($("reclass-self-hint")).toBeNull();
    await click($("reclass-approve"));
    expect(dialogs.confirm).toHaveBeenCalledOnce();
    expect(stepUp.ensure).toHaveBeenCalledOnce();
    expect(api.decideReclassification).toHaveBeenCalledWith(
      "r-1",
      "approve",
      undefined,
    );
  });

  it("does not approve when the step-up is cancelled", async () => {
    state.grants = new Set([Permission.StockRead, Permission.StockReclassify]);
    stepUp.ensure.mockResolvedValue(false);
    api.getReclassification.mockResolvedValue(request());
    await render(
      createElement(ReclassificationDetailPage, { slug: "acme", uuid: "r-1" }),
    );
    await click($("reclass-approve"));
    expect(api.decideReclassification).not.toHaveBeenCalled();
  });

  it("shows the requester no approve button but the reason", async () => {
    state.grants = new Set([Permission.StockRead, Permission.StockReclassify]);
    state.user = "user-a";
    api.getReclassification.mockResolvedValue(request());
    await render(
      createElement(ReclassificationDetailPage, { slug: "acme", uuid: "r-1" }),
    );
    expect($("reclass-approve")).toBeNull();
    expect($("reclass-reject")).not.toBeNull();
    expect($("reclass-self-hint")?.textContent).toBe("stock.reclass.self_hint");
  });

  it("rejects with a reason only", async () => {
    state.grants = new Set([Permission.StockRead, Permission.StockReclassify]);
    api.getReclassification.mockResolvedValue(request());
    api.decideReclassification.mockResolvedValue(
      request({ status: "rejected" }),
    );
    await render(
      createElement(ReclassificationDetailPage, { slug: "acme", uuid: "r-1" }),
    );
    dialogs.prompt.mockResolvedValueOnce("  ");
    await click($("reclass-reject"));
    expect(api.decideReclassification).not.toHaveBeenCalled();
    expect(toast.error).toHaveBeenCalledWith(
      "stock.reclass.errors.note_required",
    );
    dialogs.prompt.mockResolvedValueOnce("label is right");
    await click($("reclass-reject"));
    expect(api.decideReclassification).toHaveBeenCalledWith(
      "r-1",
      "reject",
      "label is right",
    );
  });

  it("gives the requesting organization the cancel only", async () => {
    state.grants = new Set([Permission.StockRead, Permission.StockWrite]);
    state.user = "user-a";
    state.org = "org-dealer";
    api.getReclassification.mockResolvedValue(request());
    await render(
      createElement(ReclassificationDetailPage, { slug: "acme", uuid: "r-1" }),
    );
    expect($("reclass-approve")).toBeNull();
    expect($("reclass-reject")).toBeNull();
    expect($("reclass-cancel")).not.toBeNull();
  });

  it("is closed without stock.read", async () => {
    await render(
      createElement(ReclassificationDetailPage, { slug: "acme", uuid: "r-1" }),
    );
    expect(api.getReclassification).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("common.error_forbidden");
  });
});

describe("ReclassificationsPage row actions (TEC-763)", () => {
  it("drops the approval on the viewer's own request", async () => {
    state.grants = new Set([Permission.StockRead, Permission.StockReclassify]);
    state.user = "user-a";
    api.listReclassifications.mockResolvedValue({
      items: [
        request(),
        request({ uuid: "r-2", requested_by: { uuid: "user-c", name: "C" } }),
      ],
      total: 2,
      limit: 20,
      offset: 0,
    });
    await render(createElement(ReclassificationsPage, { slug: "acme" }));
    expect(api.listReclassifications).toHaveBeenCalledWith(
      expect.objectContaining({ sort: "-created_at" }),
    );
    const [own, other] = captured.rowActions.slice(-2);
    expect(own).toEqual(["view", "history", "reject"]);
    expect(other).toEqual(["view", "history", "approve", "reject"]);
  });
});
