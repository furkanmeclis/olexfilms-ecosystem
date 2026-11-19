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
  cutMeters,
  cutWarnings,
  DetachedPiecesNotice,
  isCutLine,
} from "./order-cut";
import { shortLines } from "./order-partial";

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
