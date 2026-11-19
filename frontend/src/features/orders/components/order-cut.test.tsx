// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type {
  Order,
  OrderCutPreview,
  OrderItem,
} from "@/features/orders/services/orders.service";

import {
  CutDialog,
  confirmKey,
  cutMeters,
  cutWarnings,
  DetachedPiecesNotice,
  isCutLine,
} from "./order-cut";
import { shortLines } from "./order-partial";
import { lineCovered, linesBelowAssigned } from "../lib/form";

const mutate = vi.fn();
let previewData: OrderCutPreview | undefined;

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, v?: Record<string, string>) =>
      v ? `${key}:${Object.values(v).join("|")}` : key,
  }),
}));
vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({ data: previewData, isLoading: false, isError: false }),
  useMutation: () => ({ mutate, isPending: false }),
  useQueryClient: () => ({ setQueryData: vi.fn(), invalidateQueries: vi.fn() }),
}));

const preview = (over: Partial<OrderCutPreview>): OrderCutPreview => ({
  item_uuid: "i1",
  unit_uuid: "u1",
  barcode: "BOX-1",
  closed_box: false,
  source_meters: "12.00",
  cut_meters: "3.00",
  remaining_meters: "9.00",
  whole: false,
  remnant: false,
  excess_meters: "0.00",
  excess_billing: "free",
  min_cut_meters: "2.00",
  missing_meters: "3.00",
  partial: false,
  credits: 5,
  line_credits: 5,
  plotter_cut: false,
  location_code: "A-01",
  ...over,
});

const item = {
  uuid: "i1",
  product: { uuid: "p1", name: "Black PPF", sku: "BPPF", unit_type: "piece" },
  sale_measure: "meter",
  meters: "3.00",
  quantity: null,
  line_credits: 5,
  assigned: "0",
  units: [],
} as unknown as OrderItem;
const order = { uuid: "o1", items: [item] } as unknown as Order;

describe("TEC-1241 cut helpers", () => {
  it("knows a cut line: a meter line with credits", () => {
    expect(isCutLine({ sale_measure: "meter", line_credits: 5 })).toBe(true);
    expect(isCutLine({ sale_measure: "meter", line_credits: null })).toBe(
      false,
    );
    expect(isCutLine({ sale_measure: "box", line_credits: null })).toBe(false);
  });

  it("normalises typed meters", () => {
    expect(cutMeters(" 3,5 ")).toBe("3.5");
    expect(cutMeters("0")).toBe("");
    expect(cutMeters("1.234")).toBe("");
    expect(cutMeters("abc")).toBe("");
  });

  it("warns about a closed box, a remnant and a partial piece", () => {
    expect(cutWarnings(preview({})).length).toBe(0);
    const keys = cutWarnings(
      preview({ closed_box: true, remnant: true, partial: true }),
    ).map((w) => w.key);
    expect(keys).toEqual([
      "orders.cut.box_open",
      "orders.cut.remnant",
      "orders.cut.partial_short",
    ]);
    expect(cutWarnings(preview({ closed_box: true, whole: true }))[0].key).toBe(
      "orders.cut.box_whole",
    );
  });

  it("TEC-1250 (K7): the whole roll with free extra meters is an info, not a warning", () => {
    // 3 m from a 3.5 m roll: the whole roll goes, 0.5 m free.
    const roll = cutWarnings(
      preview({
        source_meters: "3.50",
        cut_meters: "3.50",
        remaining_meters: "0.00",
        whole: true,
        excess_meters: "0.50",
        min_cut_meters: "1.00",
      }),
    );
    expect(roll).toEqual([
      {
        key: "orders.cut.whole_excess",
        values: { excess: "0.50", min: "1.00", whole: "3.50" },
        tone: "info",
      },
    ]);
    // 14.6 m from a closed 15 m box: the box opens and goes whole.
    const box = cutWarnings(
      preview({
        closed_box: true,
        whole: true,
        source_meters: "15.00",
        cut_meters: "15.00",
        remaining_meters: "0.00",
        excess_meters: "0.40",
      }),
    ).map((w) => `${w.key}:${w.tone}`);
    expect(box).toEqual([
      "orders.cut.box_whole:warning",
      "orders.cut.whole_excess:info",
    ]);
    // Exactly the minimum cut left: cut as before, nothing to say.
    expect(
      cutWarnings(
        preview({ remaining_meters: "1.00", min_cut_meters: "1.00" }),
      ),
    ).toEqual([]);
    expect(confirmKey(preview({ whole: true }))).toBe(
      "orders.cut.confirm_whole",
    );
    expect(confirmKey(preview({ closed_box: true, whole: true }))).toBe(
      "orders.cut.confirm_box_whole",
    );
    expect(confirmKey(preview({}))).toBe("orders.cut.confirm");
  });

  it("TEC-1250 (19.11): the charge setting says the extra meters go on the invoice", () => {
    const charged = cutWarnings(
      preview({
        source_meters: "3.50",
        cut_meters: "3.50",
        remaining_meters: "0.00",
        whole: true,
        excess_meters: "0.50",
        excess_billing: "charge",
        min_cut_meters: "1.00",
      }),
    );
    expect(charged).toEqual([
      {
        key: "orders.cut.whole_excess_charge",
        values: { excess: "0.50", min: "1.00", whole: "3.50" },
        tone: "info",
      },
    ]);
    // A charge line was raised to its pieces: the roll covers it all.
    expect(
      lineCovered({
        assigned: "3.50",
        excess_meters: "0.50",
        excess_billing: "charge",
      }),
    ).toBe(3.5);
    expect(
      lineCovered({
        assigned: "3.50",
        excess_meters: "0.50",
        excess_billing: "free",
      }),
    ).toBe(3);
  });

  it("TEC-1250 (K7): a line covered by a whole roll is covered, not over", () => {
    const covered = { ...item, assigned: "3.50", excess_meters: "0.50" };
    expect(lineCovered(covered)).toBe(3);
    const o = { items: [covered] } as unknown as Order;
    expect(shortLines(o)).toEqual([]);
    // Keeping the line at 3 m is not below what is assigned.
    expect(
      linesBelowAssigned(o, [
        {
          product_uuid: "p1",
          unit_type: "piece",
          sale_measure: "meter",
          amount: "3",
        } as unknown as Parameters<typeof linesBelowAssigned>[1][number],
      ]).size,
    ).toBe(0);
  });

  it("flags a partial shipment rest shorter than the minimum cut", () => {
    const o = {
      items: [
        { ...item, meters: "6.00", assigned: "4.50", min_cut_meters: "2.00" },
      ],
    } as unknown as Order;
    expect(shortLines(o)[0].belowMinCut).toBe("2.00");
    const ok = {
      items: [
        { ...item, meters: "6.00", assigned: "3", min_cut_meters: "2.00" },
      ],
    } as unknown as Order;
    expect(shortLines(ok)[0].belowMinCut).toBeUndefined();
  });
});

describe("TEC-1241 CutDialog", () => {
  let host: HTMLDivElement;
  let root: Root;
  beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
    root = createRoot(host);
    mutate.mockReset();
  });
  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    document.body.innerHTML = "";
  });

  const render = (partial = false) =>
    act(() =>
      root.render(
        createElement(CutDialog, {
          order,
          item,
          barcode: "BOX-1",
          partial,
          onOpenChange: () => {},
        }),
      ),
    );
  const confirm = () =>
    document.querySelector<HTMLButtonElement>('[data-testid="cut-confirm"]')!;

  it("names the box opening and confirms with Enter", () => {
    previewData = preview({
      closed_box: true,
      source_meters: "15.00",
      remaining_meters: "12.00",
    });
    render();
    const warn = document.querySelector('[data-testid="cut-warning"]')!;
    expect(warn.textContent).toContain("orders.cut.box_open:3.00|12.00|15.00");
    expect(confirm().textContent).toBe("orders.cut.confirm_box");
    expect(confirm().disabled).toBe(false);
    act(() => {
      confirm().form!.dispatchEvent(
        new Event("submit", { bubbles: true, cancelable: true }),
      );
    });
    expect(mutate).toHaveBeenCalledTimes(1);
  });

  it("TEC-1250: says the whole roll goes free of the extra and confirms in one step", () => {
    previewData = preview({
      source_meters: "3.50",
      cut_meters: "3.50",
      remaining_meters: "0.00",
      whole: true,
      excess_meters: "0.50",
      min_cut_meters: "1.00",
    });
    render();
    const info = document.querySelector('[data-testid="cut-info"]')!;
    expect(info.textContent).toContain(
      "orders.cut.whole_excess:0.50|1.00|3.50",
    );
    expect(document.querySelector('[data-testid="cut-warning"]')).toBeNull();
    expect(confirm().textContent).toBe("orders.cut.confirm_whole");
    expect(confirm().disabled).toBe(false);
    act(() => {
      confirm().form!.dispatchEvent(
        new Event("submit", { bubbles: true, cancelable: true }),
      );
    });
    expect(mutate).toHaveBeenCalledTimes(1);
  });

  it("asks for a note before a partial piece", () => {
    previewData = preview({
      partial: true,
      cut_meters: "2.00",
      missing_meters: "3.00",
    });
    render();
    expect(confirm().disabled).toBe(true);
    expect(
      document.querySelector('[data-testid="cut-switch-partial"]'),
    ).not.toBeNull();
  });

  it("shows the partial fields when started as partial", () => {
    previewData = preview({ partial: true });
    render(true);
    expect(
      document.querySelector('[data-testid="cut-partial"]'),
    ).not.toBeNull();
    expect(confirm().disabled).toBe(true);
  });

  it("lists the cut pieces left in stock", () => {
    act(() =>
      root.render(
        createElement(DetachedPiecesNotice, {
          order: {
            detached_cut_pieces: [
              {
                unit_uuid: "x",
                barcode: "R-3M",
                cut_meters: "3.00",
                remaining_meters: "3.00",
              },
            ],
          } as unknown as Order,
        }),
      ),
    );
    expect(host.textContent).toContain("R-3M");
  });
});
