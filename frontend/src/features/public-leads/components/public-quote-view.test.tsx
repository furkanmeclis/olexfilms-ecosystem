// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import { registerMessages, translate } from "@/lib/i18n/messages";
import ar from "@/locales/ar";
import tr from "@/locales/tr";

import type { PublicQuote, PublicQuoteResult } from "../lib/public-quote";
import { PublicQuoteView } from "./public-quote-view";

vi.mock("next/image", () => ({
  default: (props: Record<string, unknown>) =>
    createElement("img", { src: props.src, alt: props.alt }),
}));
vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: { href: string; children: unknown } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const TOKEN = "3f1c2b7a-8d4e-4b6f-9a1c-2e3d4f5a6b7c";

const quote: PublicQuote = {
  uuid: "9b2f0c1d-1111-4222-8333-444455556666",
  display_no: "Q-000042",
  organization_name: "Olex Kadıköy",
  currency: "TRY",
  subtotal: "12000.00",
  discount_total: "500.00",
  tax_total: "0.00",
  grand_total: "11500.00",
  valid_until: "2026-11-30T20:59:59Z",
  lines: [
    {
      line_type: "catalog_service",
      description: "Cam filmi uygulaması",
      quantity: "1",
      unit_price: "2000.00",
      discount_amount: "0.00",
      line_total: "2000.00",
      sort_order: 2,
    },
    {
      line_type: "product",
      description: "Olex PPF Gloss",
      quantity: "1",
      unit_price: "10000.00",
      discount_amount: "500.00",
      line_total: "9500.00",
      sort_order: 1,
    },
  ],
  pdf: { url: `/v1/public/quotes/${TOKEN}/pdf` },
};

let container: HTMLDivElement;
let root: Root;

beforeAll(() => {
  registerMessages("tr", tr);
  registerMessages("ar", ar);
});

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

function render(
  result: PublicQuoteResult,
  locale: "tr" | "ar" = "tr",
  pdfNotice: "pending" | "rate_limited" | "unavailable" | null = null,
) {
  act(() => {
    root.render(
      createElement(PublicQuoteView, {
        result,
        locale,
        timeZone: "Europe/Istanbul",
        path: `/teklif/${TOKEN}`,
        token: TOKEN,
        pdfNotice,
        now: new Date("2026-10-07T10:00:00Z"),
      }),
    );
  });
}

const t = (key: string, params?: Record<string, string | number>) =>
  translate("tr", key, params);

describe("PublicQuoteView (TEC-320)", () => {
  it("shows the organization, ordered lines, totals, validity and PDF link", () => {
    render({ kind: "ok", quote });
    expect(
      container.querySelector("[data-slot='organization-name']")?.textContent,
    ).toBe("Olex Kadıköy");
    expect(container.textContent).toContain("Q-000042");
    const lines = [
      ...container.querySelectorAll("[data-slot='quote-line']"),
    ].map((li) => li.textContent ?? "");
    expect(lines).toHaveLength(2);
    expect(lines[0]).toContain("Olex PPF Gloss");
    expect(lines[0]).toContain(t("landing.quote.discount"));
    expect(lines[1]).toContain("Cam filmi uygulaması");
    expect(lines[1]).not.toContain(t("landing.quote.discount"));
    const total = container.querySelector("[data-slot='grand-total']");
    expect(total?.textContent).toMatch(/11\.500,00/);
    const totals = container.querySelector("[data-slot='quote-totals']");
    expect(totals?.textContent).toContain(t("landing.quote.discount_total"));
    // Zero tax is not listed.
    expect(totals?.textContent).not.toContain(t("landing.quote.tax_total"));
    expect(
      container.querySelector("[data-slot='valid-until']")?.textContent,
    ).toContain("2026");
    expect(container.querySelector("[data-slot='quote-expired']")).toBeNull();
    const pdf = container.querySelector<HTMLAnchorElement>(
      "[data-slot='pdf-download']",
    );
    expect(pdf?.getAttribute("href")).toBe(`/teklif/${TOKEN}/pdf?lang=tr`);
    expect(container.querySelector("table")).toBeNull();
  });

  it("never shows a phone number or an e-mail address", () => {
    // Even if a future API version leaked recipient data, the page only
    // renders whitelisted fields.
    const leaky = {
      ...quote,
      recipient_phone_e164: "+905321234567",
      recipient_email: "musteri@example.com",
      candidate_phone_e164: "+905551112233",
    } as PublicQuote;
    render({ kind: "ok", quote: leaky });
    const html = container.innerHTML;
    expect(html).not.toMatch(/\+90\d{10}/);
    expect(html).not.toMatch(/[^\s@"]+@[^\s@"]+\.[a-z]{2,}/i);
    expect(container.querySelector("a[href^='tel:']")).toBeNull();
    expect(container.querySelector("a[href^='mailto:']")).toBeNull();
  });

  it("marks an expired quote and shows the PDF notice", () => {
    render(
      { kind: "ok", quote: { ...quote, valid_until: "2026-09-01T00:00:00Z" } },
      "tr",
      "pending",
    );
    expect(
      container.querySelector("[data-slot='quote-expired']")?.textContent,
    ).toBe(t("landing.quote.expired"));
    const notice = container.querySelector("[data-slot='pdf-notice']");
    expect(notice?.getAttribute("data-notice")).toBe("pending");
    expect(notice?.textContent).toBe(t("landing.quote.pdf_pending"));
  });

  it("renders right to left in Arabic", () => {
    render({ kind: "ok", quote }, "ar");
    const main = container.querySelector("main");
    expect(main?.getAttribute("dir")).toBe("rtl");
    expect(main?.getAttribute("lang")).toBe("ar");
    expect(container.textContent).toContain(
      translate("ar", "landing.quote.grand_total"),
    );
  });

  it("has rate limited and error screens", () => {
    render({ kind: "rate_limited", retryAfter: 60 });
    expect(
      container.querySelector("[data-screen='rate-limited']"),
    ).not.toBeNull();
    expect(container.textContent).toContain(
      t("landing.quote.retry_after", { seconds: 60 }),
    );
    render({ kind: "error" });
    expect(
      container.querySelector("[data-screen='error']")?.getAttribute("role"),
    ).toBe("alert");
  });
});
