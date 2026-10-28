// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "tr",
    format: {
      number: (v: number | null) => (v === null ? "—" : String(v)),
      currency: (v: number | null, c: string) =>
        v === null ? "—" : `${v.toFixed(2)} ${c}`,
    },
  }),
}));

import { StatementOrderDetail } from "./statement-order-detail";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root | null = null;
let host: HTMLDivElement | null = null;

afterEach(() => {
  act(() => root?.unmount());
  host?.remove();
  root = null;
  host = null;
});

const order = {
  uuid: "o-1",
  order_no: "ORD-00000042",
  status: "received",
  currency: "TRY",
  items: [
    {
      product_uuid: "p-1",
      product_sku: "PPF-1",
      product_name: "Film",
      unit_type: "piece",
      quantity: 2,
      meters: null,
      unit_price: "50.0000",
      line_total: "100.00",
      units: [
        { uuid: "u-1", barcode: "B-1", quantity: null, meters: null },
        { uuid: "u-2", barcode: "B-2", quantity: null, meters: null },
      ],
    },
    {
      product_uuid: "p-2",
      product_sku: "ROLL",
      product_name: "Rulo",
      unit_type: "roll_meter",
      quantity: null,
      meters: "12.50",
      unit_price: "8.0000",
      line_total: "100.00",
      units: [],
    },
  ],
};

describe("StatementOrderDetail (TEC-1035)", () => {
  it("expands the order lines with their barcodes", async () => {
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
    await act(async () => {
      root!.render(createElement(StatementOrderDetail, { order }));
    });
    const toggle = host.querySelector(
      '[data-testid="statement-order-toggle"]',
    ) as HTMLButtonElement;
    expect(toggle.textContent).toContain('"no":"ORD-00000042"');
    expect(toggle.textContent).toContain('"items":2');
    expect(toggle.textContent).toContain('"barcodes":2');
    expect(host.querySelector('[data-testid="statement-order-items"]')).toBe(
      null,
    );

    await act(async () => toggle.click());
    const items = host.querySelectorAll('[data-testid="statement-order-item"]');
    expect(items).toHaveLength(2);
    expect(items[0].textContent).toContain("Film");
    expect(items[0].textContent).toContain("PPF-1");
    expect(items[0].textContent).toContain("50.00 TRY");
    expect(items[0].textContent).toContain("100.00 TRY");
    const chips = [
      ...host.querySelectorAll('[data-testid="statement-order-barcode"]'),
    ].map((c) => c.textContent);
    expect(chips).toEqual(["B-1", "B-2"]);
    expect(items[1].textContent).toContain("accounting.statement.order_meters");
    expect(items[1].textContent).toContain("accounting.statement.no_barcodes");
  });
});
