// @vitest-environment jsdom
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
  }),
}));

import { buildLocationTree } from "@/features/warehouse/lib/tree";
import type { WarehouseLocation } from "@/features/warehouse/services/warehouse.service";

import { LocationTree } from "./location-tree";
import { click, mount, render, unmount, type Mounted } from "./test-helpers";

function loc(
  uuid: string,
  type: WarehouseLocation["type"],
  code: string,
  parent: string | null,
  sort = 0,
  active = true,
): WarehouseLocation {
  return {
    uuid,
    warehouse_uuid: "w1",
    room_uuid: "r1",
    parent_uuid: parent,
    type,
    code,
    full_code: `WH1-R1-${code}`,
    name: code,
    active,
    sort_order: sort,
    created_at: "2026-10-01T09:00:00Z",
    updated_at: "2026-10-01T09:00:00Z",
  };
}

const items = [
  loc("b1", "bin", "01", "s1"),
  loc("s1", "shelf", "S1", "a1"),
  loc("a2", "aisle", "B", null, 2),
  loc("a1", "aisle", "A", null, 1),
  loc("b2", "bin", "02", "s1", 0, false),
];

let m: Mounted;
beforeEach(() => {
  m = mount();
});
afterEach(() => {
  unmount(m);
  vi.clearAllMocks();
});

const rows = () =>
  Array.from(
    m.container.querySelectorAll<HTMLElement>('[data-testid="location-node"]'),
  );

describe("LocationTree (TEC-231)", () => {
  it("renders the roots in sort order with children collapsed", async () => {
    const onToggle = vi.fn();
    await render(
      m,
      createElement(LocationTree, {
        label: "tree",
        nodes: buildLocationTree(items),
        expanded: new Set<string>(),
        onToggle,
      }),
    );
    expect(
      m.container.querySelector('[role="tree"]')?.getAttribute("aria-label"),
    ).toBe("tree");
    expect(rows().map((r) => r.dataset.uuid)).toEqual(["a1", "a2"]);
    const a1 = rows()[0];
    expect(a1.getAttribute("aria-level")).toBe("1");
    expect(a1.getAttribute("aria-expanded")).toBe("false");
    // A leaf has no expanded state and no toggle.
    expect(rows()[1].hasAttribute("aria-expanded")).toBe(false);
    expect(
      rows()[1].querySelector('[data-testid="location-toggle"]'),
    ).toBeNull();

    await click(a1.querySelector('[data-testid="location-toggle"]'));
    expect(onToggle).toHaveBeenCalledWith("a1");
  });

  it("shows the nested levels of expanded nodes with type and state", async () => {
    await render(
      m,
      createElement(LocationTree, {
        label: "tree",
        nodes: buildLocationTree(items),
        expanded: new Set(["a1", "s1"]),
        onToggle: vi.fn(),
      }),
    );
    expect(rows().map((r) => r.dataset.uuid)).toEqual([
      "a1",
      "s1",
      "b1",
      "b2",
      "a2",
    ]);
    const byUuid = (u: string) => rows().find((r) => r.dataset.uuid === u)!;
    expect(byUuid("s1").getAttribute("aria-level")).toBe("2");
    expect(byUuid("b1").getAttribute("aria-level")).toBe("3");
    expect(byUuid("a1").getAttribute("aria-expanded")).toBe("true");
    expect(byUuid("b1").textContent).toContain("warehouse.type.bin");
    expect(byUuid("b2").textContent).toContain("warehouse.tree.inactive");
    expect(byUuid("b1").dataset.code).toBe("WH1-R1-01");
  });

  it("selects a node and renders its actions", async () => {
    const onSelect = vi.fn();
    const renderActions = vi.fn((n: { uuid: string }) =>
      createElement("button", { "data-testid": `act-${n.uuid}` }, "x"),
    );
    await render(
      m,
      createElement(LocationTree, {
        label: "tree",
        nodes: buildLocationTree(items),
        expanded: new Set<string>(),
        onToggle: vi.fn(),
        selected: "a2",
        onSelect,
        renderActions,
      }),
    );
    expect(m.container.querySelector('[data-testid="act-a1"]')).not.toBeNull();
    expect(m.container.querySelector('[data-testid="act-a2"]')).not.toBeNull();
    expect(rows()[1].getAttribute("aria-selected")).toBe("true");
    expect(rows()[0].getAttribute("aria-selected")).toBe("false");

    const label = Array.from(rows()[0].querySelectorAll("button")).find((b) =>
      b.textContent?.includes("warehouse.type.aisle"),
    );
    await click(label ?? null);
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ uuid: "a1" }),
    );
  });
});
