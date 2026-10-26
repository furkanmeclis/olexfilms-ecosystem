// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";

import { registerMessages } from "@/lib/i18n/messages";
import ar from "@/locales/ar";
import tr from "@/locales/tr";

import type {
  PublicService,
  PublicWarrantyResult,
} from "../lib/public-warranty";
import { PublicWarrantyView } from "./public-warranty-view";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const service: PublicService = {
  service_no: "DS7K2M9QX4",
  package: "Full body",
  service_date: "2026-01-15T10:00:00Z",
  brand: { name: "Olexfilms", slug: "olex" },
  dealer: { name: "AFM Florya", city: "İstanbul" },
  vehicle: {
    brand_name: "BMW",
    brand_logo_uuid: "0b5c4a39-6a43-4d47-9a3f-1f3a2b4c5d6e",
    model_name: "M3",
    model_year: 2024,
    plate_masked: "34 *** 12",
    vin_last4: "6752",
  },
  warranties: [
    {
      public_code: "AbCdEfGhIjKlMnOpQrSt_-",
      status: "active",
      start_at: "2026-01-15T10:00:00Z",
      end_at: "2027-01-15T20:59:59Z",
      days_remaining: 105,
      product: { name: "Olex PPF Gloss" },
      category: "PPF",
      parts: ["body_kaput"],
    },
  ],
};

function withStatus(status: "expired" | "void"): PublicService {
  return {
    ...service,
    warranties: service.warranties.map((w) => ({
      ...w,
      status,
      days_remaining: 0,
    })),
  };
}

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
  result: PublicWarrantyResult,
  locale: "en" | "tr" | "ar" = "en",
  extra: { pdfNotice?: "rate_limited" | "unavailable" | null } = {},
) {
  act(() =>
    root.render(
      createElement(PublicWarrantyView, {
        result,
        locale,
        timeZone: "Europe/Istanbul",
        path: "/warranty/DS7K2M9QX4",
        // Half way through 2026-01-15 → 2027-01-15.
        now: new Date("2026-07-16T15:30:00Z"),
        ...extra,
      }),
    ),
  );
}

describe("PublicWarrantyView", () => {
  it("shows the status badge, remaining days and masked vehicle data", () => {
    render({ kind: "ok", service });
    expect(container.querySelector('[data-screen="warranty"]')).not.toBeNull();
    const badge = container.querySelector("[data-status]");
    expect(badge?.getAttribute("data-status")).toBe("active");
    expect(badge?.textContent).toBe("Active");
    expect(
      container.querySelector('[data-slot="days-remaining"]')?.textContent,
    ).toBe("105");
    const text = container.textContent ?? "";
    expect(text).toContain("34 *** 12");
    expect(text).toContain("6752");
    expect(text).toContain("Olex PPF Gloss");
    expect(text).toContain("AFM Florya");
    expect(text).toContain("DS7K2M9QX4");
    expect(text).toContain("PPF");
    expect(text).toContain("Full body");
    expect(text).toContain("İstanbul");
    expect(text).toContain("BMW M3");
    const logo = container.querySelector<HTMLImageElement>(
      '[data-slot="vehicle-brand-logo"]',
    );
    expect(logo?.getAttribute("src")).toContain(
      "/brand-logos/0b5c4a39-6a43-4d47-9a3f-1f3a2b4c5d6e",
    );
  });

  it("hides remaining days for an expired warranty", () => {
    render({
      kind: "ok",
      service: withStatus("expired"),
    });
    expect(container.querySelector("[data-status]")?.textContent).toBe(
      "Expired",
    );
    expect(container.querySelector('[data-slot="days-remaining"]')).toBeNull();
  });

  it("renders the not found screen", () => {
    render({ kind: "not_found" }, "tr");
    expect(container.querySelector('[data-screen="not-found"]')).not.toBeNull();
    expect(container.textContent).toContain("Garanti bulunamadı");
  });

  it("renders the rate limited screen with the wait time", () => {
    render({ kind: "rate_limited", retryAfter: 30 });
    expect(
      container.querySelector('[data-screen="rate-limited"]'),
    ).not.toBeNull();
    expect(container.textContent).toContain("Try again in 30 seconds.");
  });

  it("renders the error screen", () => {
    render({ kind: "error" });
    expect(container.querySelector('[data-screen="error"]')).not.toBeNull();
  });

  it("is right-to-left in Arabic and keeps the plate left-to-right", () => {
    render({ kind: "ok", service }, "ar");
    const main = container.querySelector("main");
    expect(main?.getAttribute("dir")).toBe("rtl");
    expect(main?.getAttribute("lang")).toBe("ar");
    expect(container.querySelector("[data-status]")?.textContent).toBe("نشط");
    const plate = Array.from(container.querySelectorAll('[dir="ltr"]')).map(
      (n) => n.textContent,
    );
    expect(plate).toContain("34 *** 12");
  });

  it("shows the elapsed-period progress bar (TEC-248)", () => {
    render({ kind: "ok", service });
    const bar = container.querySelector('[role="progressbar"]');
    expect(bar).not.toBeNull();
    expect(bar?.getAttribute("aria-valuenow")).toBe("50");
    expect(
      container
        .querySelector('[data-slot="warranty-progress"]')
        ?.getAttribute("data-percent"),
    ).toBe("50");
  });

  it("links the anonymous PDF of an active warranty (TEC-248)", () => {
    render({ kind: "ok", service }, "tr");
    const link = container.querySelector<HTMLAnchorElement>(
      '[data-slot="pdf-download"]',
    );
    expect(link?.getAttribute("href")).toBe(
      "/warranty/DS7K2M9QX4/pdf?lang=tr&tz=Europe%2FIstanbul",
    );
    expect(link?.textContent).toContain("Garanti belgesini indir");
    expect(container.querySelector('[data-slot="pdf-notice"]')).toBeNull();
  });

  it("has no PDF link for an expired or void warranty", () => {
    render({
      kind: "ok",
      service: withStatus("void"),
    });
    expect(container.querySelector('[data-slot="pdf-download"]')).toBeNull();
    expect(container.querySelector('[role="progressbar"]')).not.toBeNull();
  });

  it("shows the notice of a failed PDF download", () => {
    render({ kind: "ok", service }, "en", { pdfNotice: "rate_limited" });
    const notice = container.querySelector('[data-slot="pdf-notice"]');
    expect(notice?.getAttribute("role")).toBe("alert");
  });

  it("renders not found without logo, progress or PDF link", () => {
    render({ kind: "not_found" });
    expect(container.querySelector('[data-screen="not-found"]')).not.toBeNull();
    expect(
      container.querySelector('[data-slot="vehicle-brand-logo"]'),
    ).toBeNull();
    expect(container.querySelector('[role="progressbar"]')).toBeNull();
    expect(container.querySelector('[data-slot="pdf-download"]')).toBeNull();
  });

  it("lists every warranty of the service with its applied parts", () => {
    const w = service.warranties[0]!;
    render(
      {
        kind: "ok",
        service: {
          ...service,
          warranties: [
            w,
            {
              ...w,
              public_code: "Second1234",
              product: { name: "Olex Ceramic" },
              parts: [],
            },
          ],
        },
      },
      "tr",
    );
    expect(
      container.querySelectorAll('[data-slot="warranty-item"]'),
    ).toHaveLength(2);
    expect(container.textContent).toContain("Olex Ceramic");
    expect(container.textContent).toContain("Kaput");
  });

  it("links every language with ?lang=", () => {
    render({ kind: "not_found" });
    const links = container.querySelectorAll("nav a");
    expect(links).toHaveLength(13);
    expect(links[0]?.getAttribute("href")).toBe("/warranty/DS7K2M9QX4?lang=tr");
  });
});
