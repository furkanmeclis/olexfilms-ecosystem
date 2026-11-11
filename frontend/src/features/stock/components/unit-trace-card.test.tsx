// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    format: { dateTime: (v: string) => `dt(${v})` },
  }),
}));

import { UnitTraceCard, type UnitTrace } from "./unit-trace-card";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

const org = (name: string) => ({ uuid: name, name, type: "dealer" });

describe("UnitTraceCard (TEC-1006)", () => {
  it("shows the order that carried the unit and the service with its customer", () => {
    const trace: UnitTrace = {
      source: "order",
      origin_warehouse: {
        uuid: "w1",
        code: "FLORYA",
        name: "Merkez Depo",
      },
      orders: [
        {
          uuid: "o1",
          order_no: "SP-1",
          status: "received",
          external_reference: null,
          buyer: org("Bayi Bir"),
          seller: org("Olex Merkez"),
          created_at: "2026-01-01T00:00:00Z",
          submitted_at: null,
          shipped_at: null,
          received_at: "2026-01-02T00:00:00Z",
        },
      ],
      services: [
        {
          uuid: "s1",
          service_no: "DS123",
          status: "completed",
          completed_at: "2026-02-01T00:00:00Z",
          organization: org("Bayi Bir"),
          customer: { uuid: "c1", name: "Ayşe Yılmaz" },
        },
      ],
    };
    act(() =>
      root.render(createElement(UnitTraceCard, { trace, slug: "acme" })),
    );
    expect(container.textContent).toContain("stock.trace.source.order");
    expect(container.textContent).toContain("Merkez Depo");
    expect(
      container.querySelector("[data-testid=trace-orders]")?.textContent,
    ).toContain("Olex Merkez → Bayi Bir");
    expect(
      container.querySelector("[data-testid=trace-services]")?.textContent,
    ).toContain("Ayşe Yılmaz");
    expect(
      container.querySelector('a[href="/t/acme/orders/o1"]'),
    ).not.toBeNull();
  });

  it("says the source is unknown when no order carried it", () => {
    const trace: UnitTrace = {
      source: "unknown",
      origin_warehouse: null,
      orders: [],
      services: [],
    };
    act(() => root.render(createElement(UnitTraceCard, { trace })));
    expect(container.textContent).toContain("stock.trace.no_order_unknown");
    expect(container.textContent).toContain("stock.trace.source.unknown");
  });
});
