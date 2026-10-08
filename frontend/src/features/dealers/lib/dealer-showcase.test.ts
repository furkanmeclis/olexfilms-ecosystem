import { describe, expect, it, vi } from "vitest";

import {
  dealerLocality,
  dealerLogoSrc,
  dealerMapHref,
  dealerJsonLd,
  dealerOgImage,
  dealerPhotoSrc,
  dealerPosition,
  fetchPublicDealer,
  fetchLeadFormConfig,
  googleProfileHref,
  normalizeDealerCode,
  type PublicDealer,
} from "./dealer-showcase";

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
  headline: "Premium PPF",
  about: "Paint protection specialists.",
  working_hours: [
    { day: "monday", windows: [{ start: "09:00", end: "18:00" }] },
    { day: "tuesday", windows: [] },
  ],
  open_now: true,
  timezone: "Europe/Istanbul",
  services: [{ kind: "custom", title: "PPF", description: "Full body" }],
  photos: [{ url: "/v1/public/dealers/olex-kadikoy/photos/1", caption: "" }],
  social_links: { instagram: "https://instagram.com/olex" },
  seo_keywords: ["ppf"],
  google_rating: 4.8,
  google_review_count: 128,
  google_rating_source: "places",
  google_place_id: "places-1",
  lead_form_enabled: true,
  whatsapp_chat_url: "https://wa.me/905321234567?text=showcase",
  published_at: "2026-10-08T09:00:00Z",
};

function upstream(status: number, body: unknown, headers?: HeadersInit) {
  return {
    status,
    headers: new Headers(headers),
    body: new TextEncoder().encode(JSON.stringify(body)).buffer as ArrayBuffer,
  };
}

const REQUEST = { clientIp: "203.0.113.7", forwardedHost: "olexfilms.app" };

describe("normalizeDealerCode", () => {
  it("accepts slugs, lower-cases and trims", () => {
    expect(normalizeDealerCode("olex-kadikoy")).toBe("olex-kadikoy");
    expect(normalizeDealerCode(" Olex-Kadikoy ")).toBe("olex-kadikoy");
  });
  it("rejects anything that cannot be a slug", () => {
    for (const code of [
      "",
      "-x",
      "a/b",
      "a b",
      "ö",
      "%E0%A4%A",
      "a".repeat(101),
    ]) {
      expect(normalizeDealerCode(code)).toBeNull();
    }
  });
});

describe("fetchPublicDealer", () => {
  it("returns the dealer and forwards IP and host", async () => {
    const fetcher = vi.fn().mockResolvedValue(upstream(200, { data: DEALER }));
    const result = await fetchPublicDealer("olex-kadikoy", REQUEST, fetcher);
    expect(result).toEqual({ kind: "ok", dealer: DEALER });
    const [path, init] = fetcher.mock.calls[0]!;
    expect(path).toBe("public/dealers/olex-kadikoy");
    const headers = init.headers as Headers;
    expect(headers.get("X-Forwarded-For")).toBe("203.0.113.7");
    expect(headers.get("X-Forwarded-Host")).toBe("olexfilms.app");
  });

  it("is not_found for a malformed code without calling Go", async () => {
    const fetcher = vi.fn();
    expect(await fetchPublicDealer("a/b", REQUEST, fetcher)).toEqual({
      kind: "not_found",
    });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("maps 404, 429 and failures", async () => {
    expect(
      await fetchPublicDealer(
        "x",
        REQUEST,
        vi.fn().mockResolvedValue(upstream(404, {})),
      ),
    ).toEqual({ kind: "not_found" });
    expect(
      await fetchPublicDealer(
        "x",
        REQUEST,
        vi.fn().mockResolvedValue(upstream(429, {}, { "retry-after": "30" })),
      ),
    ).toEqual({ kind: "rate_limited", retryAfter: 30 });
    expect(
      await fetchPublicDealer(
        "x",
        REQUEST,
        vi.fn().mockResolvedValue(upstream(500, {})),
      ),
    ).toEqual({ kind: "error" });
    expect(
      await fetchPublicDealer(
        "x",
        REQUEST,
        vi.fn().mockRejectedValue(new Error("down")),
      ),
    ).toEqual({ kind: "error" });
    expect(
      await fetchPublicDealer(
        "x",
        REQUEST,
        vi.fn().mockResolvedValue(upstream(200, { data: {} })),
      ),
    ).toEqual({ kind: "error" });
  });
});

describe("helpers", () => {
  it("reads the position only when both coordinates are numbers", () => {
    expect(dealerPosition(DEALER)).toEqual({ lat: 40.99, lng: 29.03 });
    expect(
      dealerPosition({ ...DEALER, latitude: null, longitude: null }),
    ).toBeNull();
    expect(dealerPosition({ ...DEALER, longitude: null })).toBeNull();
  });

  it("joins district and city without empty parts", () => {
    expect(dealerLocality(DEALER)).toBe("Kadıköy, İstanbul");
    expect(dealerLocality({ ...DEALER, district: "" })).toBe("İstanbul");
    expect(dealerLocality({ ...DEALER, district: "", city: " " })).toBe("");
  });

  it("builds the logo and map links", () => {
    expect(dealerLogoSrc(DEALER)).toBe(`/api${DEALER.logo_url}`);
    expect(dealerLogoSrc({ ...DEALER, logo_url: null })).toBeNull();
    expect(dealerPhotoSrc("/v1/public/dealers/x/photos/1")).toBe(
      "/api/v1/public/dealers/x/photos/1",
    );
    expect(dealerMapHref({ lat: 40.99, lng: 29.03 })).toBe(
      "https://www.openstreetmap.org/?mlat=40.990000&mlon=29.030000#map=17/40.990000/29.030000",
    );
  });

  it("builds SEO helpers and JSON-LD rating only for Places", () => {
    const dealer = { ...DEALER, showcase: SHOWCASE };
    expect(dealerOgImage(dealer)).toBe(
      "/api/v1/public/dealers/olex-kadikoy/photos/1",
    );
    expect(googleProfileHref(SHOWCASE)).toContain("query_place_id=places-1");
    expect(dealerJsonLd(dealer).aggregateRating).toEqual({
      "@type": "AggregateRating",
      ratingValue: 4.8,
      reviewCount: 128,
    });
    expect(
      dealerJsonLd({
        ...dealer,
        showcase: { ...SHOWCASE, google_rating_source: "manual" },
      }).aggregateRating,
    ).toBeUndefined();
  });
});

describe("fetchLeadFormConfig", () => {
  it("reads the public lead form config through the BFF", async () => {
    const config = {
      dealer_code: "olex-kadikoy",
      dealer_name: "Olex Kadıköy",
      fields: ["name"],
      kvkk_text_version: 1,
      kvkk_text: "KVKK",
      services: [],
      whatsapp_chat_url: "https://wa.me/905321234567",
      form_token: "token",
      min_fill_seconds: 1,
      default_phone_country: "TR",
      preferred_locales: ["tr", "en"],
    };
    const fetcher = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ success: true, data: config }), {
        status: 200,
      }),
    );
    await expect(
      fetchLeadFormConfig("OLEX-KADIKOY", "en", fetcher),
    ).resolves.toEqual({
      kind: "ok",
      config,
    });
    expect(fetcher.mock.calls[0]![0]).toBe(
      "/api/v1/public/dealers/olex-kadikoy/lead-form/config?lang=en",
    );
  });
});
