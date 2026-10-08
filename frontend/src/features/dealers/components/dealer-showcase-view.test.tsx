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

const mapProps = vi.hoisted(() => ({
  last: null as null | {
    center: { lat: number; lng: number };
    markers: { id: string; lat: number; lng: number }[];
  },
}));

vi.mock("@/components/common/leaflet-map", () => ({
  LeafletMap: (props: {
    center: { lat: number; lng: number };
    markers: { id: string; lat: number; lng: number }[];
    testId: string;
  }) => {
    mapProps.last = props;
    return createElement("div", { "data-testid": props.testId });
  },
}));

import { registerMessages } from "@/lib/i18n/messages";
import ar from "@/locales/ar";
import tr from "@/locales/tr";

import type { PublicDealer, PublicDealerResult } from "../lib/dealer-showcase";
import { DealerShowcaseView } from "./dealer-showcase-view";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const DEALER: PublicDealer = {
  code: "olex-kadikoy",
  name: "Olex Kadıköy",
  logo_url:
    "/v1/public/organizations/logo/0b5c4a39-6a43-4d47-9a3f-1f3a2b4c5d6e",
  address: "Moda Cd. 1",
  city: "İstanbul",
  district: "Kadıköy",
  latitude: 40.99,
  longitude: 29.03,
  whatsapp: "+905321234567",
};

const SHOWCASE: NonNullable<PublicDealer["showcase"]> = {
  locale: "en",
  headline: "Premium PPF studio",
  about: "Certified paint protection film applications.",
  working_hours: [
    { day: "monday", windows: [{ start: "09:00", end: "18:00" }] },
    { day: "tuesday", windows: [] },
  ],
  open_now: true,
  timezone: "Europe/Istanbul",
  services: [
    { kind: "custom", title: "Full body PPF", description: "Gloss film" },
  ],
  photos: [
    {
      url: "/v1/public/dealers/olex-kadikoy/photos/11111111-1111-4111-8111-111111111111",
      caption: "Workshop",
    },
  ],
  social_links: { instagram: "https://instagram.com/olex" },
  seo_keywords: ["ppf"],
  google_rating: 4.8,
  google_review_count: 128,
  google_rating_source: "places",
  google_place_id: "ChIJ1",
  lead_form_enabled: false,
  whatsapp_chat_url: "https://wa.me/905321234567?text=showcase",
  published_at: "2026-10-08T09:00:00Z",
};

let container: HTMLDivElement;
let root: Root;

beforeAll(() => {
  registerMessages("tr", tr);
  registerMessages("ar", ar);
});

beforeEach(() => {
  mapProps.last = null;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

function render(result: PublicDealerResult, locale: "en" | "tr" | "ar" = "en") {
  act(() => {
    root.render(
      createElement(DealerShowcaseView, {
        result,
        locale,
        path: "/bayi/olex-kadikoy",
      }),
    );
  });
}

const q = (selector: string) => container.querySelector(selector);

describe("DealerShowcaseView", () => {
  it("renders the dealer with map, logo, address and WhatsApp", () => {
    render({ kind: "ok", dealer: DEALER });
    expect(q('[data-screen="dealer"]')).not.toBeNull();
    expect(q("h1")?.textContent).toBe("Olex Kadıköy");
    expect(container.textContent).toContain("Kadıköy, İstanbul");
    expect(q('[data-slot="dealer-address"]')?.textContent).toContain(
      "Moda Cd. 1",
    );
    expect(q('[data-slot="dealer-logo"]')?.getAttribute("src")).toBe(
      `/api${DEALER.logo_url}`,
    );
    expect(q('[data-testid="dealer-showcase-map"]')).not.toBeNull();
    expect(mapProps.last?.center).toEqual({ lat: 40.99, lng: 29.03 });
    expect(mapProps.last?.markers).toHaveLength(1);
    expect(q('[data-slot="dealer-no-location"]')).toBeNull();
    const wa = q('[data-slot="dealer-whatsapp"]');
    expect(wa?.getAttribute("href")).toMatch(
      /^https:\/\/wa\.me\/905321234567\?text=/,
    );
    expect(q('[data-slot="dealer-finder-link"]')?.getAttribute("href")).toBe(
      "/portal/dealers",
    );
    expect(q("main")?.getAttribute("dir")).toBe("ltr");
    expect(q('[data-slot="showcase-services"]')).toBeNull();
    expect(q('[data-slot="showcase-gallery"]')).toBeNull();
  });

  it("renders showcase sections when the published block exists", () => {
    render({ kind: "ok", dealer: { ...DEALER, showcase: SHOWCASE } });
    expect(q('[data-slot="showcase-intro"]')?.textContent).toContain(
      "Premium PPF studio",
    );
    expect(q('[data-slot="google-rating"]')?.textContent).toContain("4.8");
    expect(q('[data-slot="showcase-open-now"]')?.textContent).toContain(
      "Open now",
    );
    expect(q('[data-slot="showcase-services"]')?.textContent).toContain(
      "Full body PPF",
    );
    expect(q('[data-slot="showcase-gallery"] img')?.getAttribute("src")).toBe(
      "/api/v1/public/dealers/olex-kadikoy/photos/11111111-1111-4111-8111-111111111111",
    );
    expect(q('[data-slot="showcase-social"]')?.textContent).toContain(
      "instagram",
    );
    expect(q('[data-slot="dealer-whatsapp"]')?.getAttribute("href")).toBe(
      SHOWCASE.whatsapp_chat_url,
    );
  });

  it("shows no map for a dealer without coordinates", () => {
    render({
      kind: "ok",
      dealer: { ...DEALER, latitude: null, longitude: null },
    });
    expect(q('[data-testid="dealer-showcase-map"]')).toBeNull();
    expect(mapProps.last).toBeNull();
    expect(q('[data-slot="dealer-map-link"]')).toBeNull();
    expect(q('[data-slot="dealer-no-location"]')).not.toBeNull();
    expect(q('[data-slot="dealer-whatsapp"]')).not.toBeNull();
  });

  it("hides the WhatsApp button and logo when missing", () => {
    render({
      kind: "ok",
      dealer: { ...DEALER, whatsapp: null, logo_url: null, address: "" },
    });
    expect(q('[data-slot="dealer-whatsapp"]')).toBeNull();
    expect(q('[data-slot="dealer-logo"]')).toBeNull();
    expect(q('[data-slot="dealer-address"]')).toBeNull();
  });

  it("renders the not found, rate limited and error screens", () => {
    render({ kind: "not_found" }, "tr");
    expect(q('[data-screen="not-found"]')?.textContent).toContain(
      "Bayi bulunamadı",
    );
    render({ kind: "rate_limited", retryAfter: 20 });
    expect(q('[data-screen="rate-limited"]')).not.toBeNull();
    render({ kind: "error" });
    expect(q('[data-screen="error"]')).not.toBeNull();
  });

  it("is right-to-left in Arabic with language links", () => {
    render({ kind: "ok", dealer: DEALER }, "ar");
    expect(q("main")?.getAttribute("dir")).toBe("rtl");
    expect(q('a[hreflang="tr"]')?.getAttribute("href")).toBe(
      "/bayi/olex-kadikoy?lang=tr",
    );
  });
});
