// @vitest-environment jsdom
import type { ColumnDef } from "@tanstack/react-table";
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  listMembers: vi.fn(),
  addMember: vi.fn(),
  removeMember: vi.fn(),
  memberCandidates: vi.fn(),
  myWarehouses: vi.fn(),
  updateSettings: vi.fn(),
}));
const captured = vi.hoisted(() => ({
  tables: [] as Record<string, unknown>[],
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: { number: String, dateTime: (v: string) => `dt(${v})` },
  }),
}));
vi.mock("@/providers/dialog-provider", () => ({
  useDialogs: () => ({ confirmDelete: vi.fn(async () => true) }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityTable: (props: Record<string, unknown>) => {
    captured.tables.push(props);
    return null;
  },
}));
vi.mock("@/components/ui/async-combobox", () => ({
  AsyncCombobox: (p: { onValueChange: (v: string) => void }) =>
    createElement("input", {
      "data-testid": "member-user",
      onChange: (e: { target: { value: string } }) =>
        p.onValueChange(e.target.value),
    }),
}));
vi.mock("@/features/warehouse/services/warehouse.service", async (orig) => ({
  ...(await orig<object>()),
  warehouseService: api,
}));

import type { Warehouse } from "../services/warehouse.service";
import { ActiveWarehouseSelect } from "./active-warehouse";
import {
  click,
  fill,
  mount,
  render,
  submit,
  unmount,
  type Mounted,
} from "./test-helpers";
import { CUTOFF_PATTERN, EodCutoffForm } from "./warehouse-settings-page";
import { MEMBERS_PERSIST_KEY, WarehouseMembersCard } from "./warehouse-members";

const w1 = {
  uuid: "w-1",
  code: "W1",
  name: "Main",
  address: null,
  active: true,
  require_location_placement: false,
  sort_order: 0,
  created_at: "",
  updated_at: "",
} satisfies Warehouse;

let m: Mounted;
beforeEach(() => {
  m = mount();
  captured.tables = [];
  localStorage.clear();
  for (const fn of Object.values(api)) fn.mockReset();
  api.listMembers.mockResolvedValue({
    items: [
      {
        uuid: "m-1",
        user: {
          uuid: "u-1",
          name: "Ada",
          surname: "Lovelace",
          email: "ada@x.test",
        },
        is_default: true,
        created_at: "2026-10-16T10:00:00Z",
      },
    ],
    total: 1,
    limit: 20,
    offset: 0,
  });
});
afterEach(() => unmount(m));

const lastTable = () =>
  captured.tables[captured.tables.length - 1] as {
    columns: ColumnDef<unknown, unknown>[];
    data: unknown[];
    features: { persistKey: string };
  };

describe("WarehouseMembersCard (TEC-769)", () => {
  it("lists the members by name with a remove action for managers", async () => {
    await render(
      m,
      createElement(WarehouseMembersCard, { warehouse: w1, canManage: true }),
    );
    expect(api.listMembers).toHaveBeenCalledWith(
      "w-1",
      expect.objectContaining({ sort: "name", limit: 20, offset: 0 }),
    );
    const t = lastTable();
    expect(t.features.persistKey).toBe(MEMBERS_PERSIST_KEY);
    expect(t.data).toHaveLength(1);
    expect(
      t.columns.map((c) => c.id ?? (c as { accessorKey?: string }).accessorKey),
    ).toContain("actions");
    expect(
      m.container.querySelector('[data-testid="member-add"]'),
    ).not.toBeNull();
  });

  it("hides the add and remove actions without members.write", async () => {
    await render(
      m,
      createElement(WarehouseMembersCard, { warehouse: w1, canManage: false }),
    );
    const ids = lastTable().columns.map(
      (c) => c.id ?? (c as { accessorKey?: string }).accessorKey,
    );
    expect(ids).not.toContain("actions");
    expect(m.container.querySelector('[data-testid="member-add"]')).toBeNull();
  });

  it("adds a member as default from the dialog", async () => {
    api.addMember.mockResolvedValue({});
    await render(
      m,
      createElement(WarehouseMembersCard, { warehouse: w1, canManage: true }),
    );
    await click(m.container.querySelector('[data-testid="member-add"]'));
    const save = document.querySelector(
      '[data-testid="member-save"]',
    ) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    await fill(document.querySelector('[data-testid="member-user"]'), "u-2");
    await click(document.querySelector('[data-testid="member-default"]'));
    await click(document.querySelector('[data-testid="member-save"]'));
    expect(api.addMember).toHaveBeenCalledWith("w-1", {
      user_uuid: "u-2",
      is_default: true,
    });
  });
});

describe("EodCutoffForm (TEC-769)", () => {
  const settings = { eod_cutoff: null, timezone: "Europe/Istanbul" };

  it("accepts HH:MM between 00:00 and 23:59 only", () => {
    for (const ok of ["00:00", "07:30", "18:00", "23:59"]) {
      expect(CUTOFF_PATTERN.test(ok)).toBe(true);
    }
    for (const bad of ["24:00", "7:30", "18:60", "18", "18:00:00", "ab:cd"]) {
      expect(CUTOFF_PATTERN.test(bad)).toBe(false);
    }
  });

  it("saves HH:MM and clears to null", async () => {
    api.updateSettings.mockResolvedValue({
      eod_cutoff: "18:30",
      timezone: "Europe/Istanbul",
    });
    await render(m, createElement(EodCutoffForm, { settings, canWrite: true }));
    await fill(
      m.container.querySelector('[data-testid="eod-cutoff-input"]'),
      "18:30",
    );
    expect(m.container.textContent).toContain(
      "warehouse.settings.eod_hint_cutoff",
    );
    await submit(m.container.querySelector("form"));
    expect(api.updateSettings).toHaveBeenCalledWith({ eod_cutoff: "18:30" });
    await click(m.container.querySelector('[data-testid="eod-cutoff-clear"]'));
    await submit(m.container.querySelector("form"));
    expect(api.updateSettings).toHaveBeenLastCalledWith({ eod_cutoff: null });
  });

  it("is read only without warehouse.write", async () => {
    await render(
      m,
      createElement(EodCutoffForm, {
        settings: { ...settings, eod_cutoff: "18:00" },
        canWrite: false,
      }),
    );
    const input = m.container.querySelector(
      '[data-testid="eod-cutoff-input"]',
    ) as HTMLInputElement;
    expect(input.disabled).toBe(true);
    expect(input.value).toBe("18:00");
    expect(
      m.container.querySelector('[data-testid="eod-cutoff-save"]'),
    ).toBeNull();
  });
});

describe("ActiveWarehouseSelect (TEC-769)", () => {
  const item = (uuid: string, member: boolean, isDefault = false) => ({
    ...w1,
    uuid,
    code: uuid.toUpperCase(),
    member,
    is_default: isDefault,
  });

  it("shows the member default of a restricted user", async () => {
    api.myWarehouses.mockResolvedValue({
      restricted: true,
      default_warehouse_uuid: "b",
      items: [item("a", false), item("b", true, true)],
    });
    await render(m, createElement(ActiveWarehouseSelect, { slug: "acme" }));
    const trigger = m.container.querySelector(
      '[data-testid="active-warehouse-select"]',
    );
    expect(trigger?.textContent).toContain("B");
    expect(
      m.container.querySelector('[data-testid="active-warehouse-restricted"]'),
    ).not.toBeNull();
  });

  it("renders nothing when no warehouse is writable", async () => {
    api.myWarehouses.mockResolvedValue({
      restricted: true,
      default_warehouse_uuid: null,
      items: [item("a", false)],
    });
    await render(m, createElement(ActiveWarehouseSelect, { slug: "acme" }));
    expect(
      m.container.querySelector('[data-testid="active-warehouse"]'),
    ).toBeNull();
  });
});
