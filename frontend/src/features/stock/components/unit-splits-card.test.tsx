// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({ listFamily: vi.fn() }));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, v?: Record<string, unknown>) =>
      v ? `${key} ${JSON.stringify(v)}` : key,
  }),
}));
vi.mock("@/features/stock/services/stock.service", () => ({
  stockService: api,
  stockKeys: {
    family: (o: string, b: string) => ["stock", "family", o, b],
  },
}));

import {
  StockFamilyCard,
  UnitSplitsCard,
  type UnitSplits,
} from "./unit-splits-card";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  api.listFamily.mockReset();
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

const dealer = { uuid: "d1", name: "Bayi Bir", type: "dealer" as const };

describe("UnitSplitsCard (TEC-1078)", () => {
  it("lists the parts with meters, state, holder and order", () => {
    const splits: UnitSplits = {
      source: null,
      parts: [
        {
          uuid: "p1",
          barcode: "2026AUR230-D001-3M",
          depth: 1,
          source_barcode: "2026AUR230-D001",
          meters: "3.00",
          remaining_meters: "3.00",
          status: "available",
          family_status: "sold",
          owner_type: "organization",
          holder: dealer,
          order: { uuid: "o1", order_no: "ORD-00000001" },
          split_at: "2026-11-01T00:00:00Z",
        },
        {
          uuid: "p2",
          barcode: "2026AUR230-D001-2.5M",
          depth: 1,
          source_barcode: "2026AUR230-D001",
          meters: "2.50",
          remaining_meters: "0.00",
          status: "used",
          family_status: "used",
          owner_type: "trash",
          holder: dealer,
          order: null,
          split_at: "2026-11-02T00:00:00Z",
        },
      ],
    };
    act(() =>
      root.render(createElement(UnitSplitsCard, { splits, slug: "acme" })),
    );
    const parts = container.querySelectorAll("[data-testid=split-part]");
    expect(parts).toHaveLength(2);
    expect(parts[0].textContent).toContain("2026AUR230-D001-3M");
    expect(parts[0].textContent).toContain("stock.family_status.sold");
    expect(parts[0].textContent).toContain("Bayi Bir");
    expect(parts[0].textContent).toContain("ORD-00000001");
    expect(parts[1].textContent).toContain("stock.family_status.used");
    expect(parts[0].querySelector("a")?.getAttribute("href")).toBe(
      "/t/acme/stock/units/2026AUR230-D001-3M",
    );
  });

  it("names the source roll and hides when there is no family", () => {
    act(() =>
      root.render(
        createElement(UnitSplitsCard, {
          splits: {
            source: { uuid: "r1", barcode: "ROLL-1", meters: "3.00" },
            parts: [],
          },
          slug: "acme",
        }),
      ),
    );
    expect(
      container.querySelector("[data-testid=split-source]")?.textContent,
    ).toContain("ROLL-1");
    act(() =>
      root.render(
        createElement(UnitSplitsCard, {
          splits: { source: null, parts: [] },
          slug: "acme",
        }),
      ),
    );
    expect(container.querySelector("[data-testid=unit-splits]")).toBeNull();
  });
});

describe("StockFamilyCard (TEC-1078)", () => {
  const row = (barcode: string, depth: number, status: string) => ({
    uuid: barcode,
    barcode,
    unit_kind: "serial",
    status: "available",
    quantity: status === "on_hand" ? 1 : 0,
    initial_meters: "3.00",
    remaining_meters: "3.00",
    product: {
      uuid: "pr1",
      sku: "AUR",
      name: "PPF",
      unit_type: "roll_meter",
      uses_fixed_barcode: false,
    },
    category: { uuid: "c1", name: "PPF" },
    location: null,
    purchase_price: null,
    updated_at: "2026-11-01T00:00:00Z",
    holder: dealer,
    family_status: status,
    family_depth: depth,
    split_meters: depth > 0 ? "3.00" : undefined,
  });

  async function renderCard(barcode: string) {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    await act(async () => {
      root.render(
        createElement(
          QueryClientProvider,
          { client },
          createElement(StockFamilyCard, {
            orgUuid: "org-1",
            barcode,
            slug: "acme",
          }),
        ),
      );
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  }

  it("shows the roll with its parts for a searched main code", async () => {
    api.listFamily.mockResolvedValue({
      items: [
        row("2026AUR230-D001", 0, "on_hand"),
        row("2026AUR230-D001-2M", 1, "sold"),
        row("2026AUR230-D001-5M", 1, "used"),
      ],
      total: 3,
      limit: 100,
      offset: 0,
    });
    await renderCard(" 2026AUR230-D001 ");
    expect(api.listFamily).toHaveBeenCalledWith("org-1", "2026AUR230-D001");
    const parts = container.querySelectorAll("[data-testid=split-part]");
    expect(Array.from(parts, (p) => p.querySelector("a")?.textContent)).toEqual(
      ["2026AUR230-D001", "2026AUR230-D001-2M", "2026AUR230-D001-5M"],
    );
  });

  it("stays hidden for a code without parts", async () => {
    api.listFamily.mockResolvedValue({
      items: [row("OLX-1", 0, "on_hand")],
      total: 1,
      limit: 100,
      offset: 0,
    });
    await renderCard("OLX-1");
    expect(container.querySelector("[data-testid=stock-family]")).toBeNull();
  });
});
