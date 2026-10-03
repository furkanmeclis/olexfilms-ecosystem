import { describe, expect, it, vi } from "vitest";

import { mapConfig } from "@/config/map";

import {
  fetchNearbyDealers,
  formatDistanceKm,
  locateUser,
  nearbyQueryString,
  parseNearbyQuery,
  whatsappHref,
  type GeolocationLike,
} from "./dealers";

describe("formatDistanceKm", () => {
  it("shows one decimal under 10 km", () => {
    expect(formatDistanceKm(3.456, "en")).toBe("3.5 km");
    expect(formatDistanceKm(0.04, "en")).toBe("0 km");
  });

  it("shows whole kilometres from 10 km", () => {
    expect(formatDistanceKm(12.49, "en")).toBe("12 km");
    expect(formatDistanceKm(1234.6, "en")).toBe("1,235 km");
  });

  it("follows the page language", () => {
    expect(formatDistanceKm(3.456, "tr")).toBe("3,5 km");
    expect(formatDistanceKm(1234.6, "de")).toBe("1.235 km");
  });

  it("accepts the zh_CN spelling and never prints negatives", () => {
    expect(formatDistanceKm(5, "zh_CN")).toMatch(/5/);
    expect(formatDistanceKm(-2, "en")).toBe("0 km");
    expect(formatDistanceKm(Number.NaN, "en")).toBe("0 km");
  });
});

describe("whatsappHref", () => {
  it("builds wa.me from E.164 digits", () => {
    expect(whatsappHref("+905321234567")).toBe("https://wa.me/905321234567");
  });

  it("drops separators", () => {
    expect(whatsappHref("+90 (532) 123-45-67")).toBe(
      "https://wa.me/905321234567",
    );
  });

  it("adds an encoded text", () => {
    expect(whatsappHref("+4915112345678", "Hallo & tschüss")).toBe(
      "https://wa.me/4915112345678?text=Hallo%20%26%20tsch%C3%BCss",
    );
  });

  it("rejects numbers that are not E.164", () => {
    expect(whatsappHref(null)).toBeNull();
    expect(whatsappHref("")).toBeNull();
    expect(whatsappHref("05321234567")).toBeNull();
    expect(whatsappHref("+0532123")).toBeNull();
    expect(whatsappHref("+90532abc4567")).toBeNull();
    expect(whatsappHref("+1234567890123456")).toBeNull();
  });
});

function geo(
  outcome:
    | { ok: { latitude: number; longitude: number } }
    | { code: number }
    | "throw",
): GeolocationLike {
  return {
    getCurrentPosition: (success, error) => {
      if (outcome === "throw") throw new Error("blocked");
      if ("ok" in outcome) {
        success({ coords: outcome.ok } as GeolocationPosition);
      } else {
        error?.({ code: outcome.code } as GeolocationPositionError);
      }
    },
  };
}

describe("locateUser (location fallback)", () => {
  it("uses the browser position when allowed", async () => {
    await expect(
      locateUser(geo({ ok: { latitude: 41.01, longitude: 28.97 } })),
    ).resolves.toEqual({
      kind: "located",
      center: { lat: 41.01, lng: 28.97 },
    });
  });

  it("falls back to the default centre when permission is denied", async () => {
    await expect(locateUser(geo({ code: 1 }))).resolves.toEqual({
      kind: "fallback",
      center: mapConfig.defaultCenter,
      reason: "denied",
    });
  });

  it("falls back when the position is unavailable or times out", async () => {
    for (const code of [2, 3]) {
      const r = await locateUser(geo({ code }));
      expect(r).toMatchObject({ kind: "fallback", reason: "unavailable" });
    }
    await expect(locateUser(geo("throw"))).resolves.toMatchObject({
      kind: "fallback",
      reason: "unavailable",
    });
  });

  it("falls back when the browser has no geolocation", async () => {
    await expect(locateUser(undefined)).resolves.toMatchObject({
      kind: "fallback",
      reason: "unsupported",
      center: mapConfig.defaultCenter,
    });
  });
});

describe("parseNearbyQuery", () => {
  const q = (s: string) => parseNearbyQuery(new URLSearchParams(s));

  it("reads lat, lng and radius", () => {
    expect(q("lat=41&lng=29&radius_km=50")).toEqual({
      lat: 41,
      lng: 29,
      radiusKm: 50,
    });
    expect(q("lat=-90&lng=180")).toEqual({ lat: -90, lng: 180, radiusKm: 100 });
  });

  it("rejects missing or out of range values", () => {
    expect(q("lng=29")).toBeNull();
    expect(q("lat=91&lng=29")).toBeNull();
    expect(q("lat=41&lng=-181")).toBeNull();
    expect(q("lat=abc&lng=29")).toBeNull();
    expect(q("lat=41&lng=29&radius_km=0")).toBeNull();
    expect(q("lat=41&lng=29&radius_km=1001")).toBeNull();
  });

  it("round-trips through the query string", () => {
    const s = nearbyQueryString({ lat: 41.0082, lng: 28.9784, radiusKm: 25 });
    expect(parseNearbyQuery(new URLSearchParams(s))).toEqual({
      lat: 41.0082,
      lng: 28.9784,
      radiusKm: 25,
    });
  });
});

describe("fetchNearbyDealers", () => {
  const query = { lat: 41, lng: 29, radiusKm: 100 };

  it("returns the items of the envelope", async () => {
    const item = {
      slug: "kadikoy",
      name: "Kadıköy",
      city: "İstanbul",
      district: "Kadıköy",
      latitude: 40.99,
      longitude: 29.03,
      distance_km: 2.5,
      whatsapp: "+905321234567",
    };
    const fetchImpl = vi.fn(
      async () =>
        new Response(
          JSON.stringify({ success: true, data: { items: [item] } }),
          { status: 200 },
        ),
    );
    await expect(
      fetchNearbyDealers(query, fetchImpl as unknown as typeof fetch),
    ).resolves.toEqual({ kind: "ok", dealers: [item] });
    expect(String(fetchImpl.mock.calls[0]![0 as never])).toBe(
      "/portal/dealers/nearby?lat=41.000000&lng=29.000000&radius_km=100",
    );
  });

  it("maps 429 and failures", async () => {
    const r429 = vi.fn(async () => new Response("{}", { status: 429 }));
    await expect(
      fetchNearbyDealers(query, r429 as unknown as typeof fetch),
    ).resolves.toEqual({ kind: "rate_limited" });
    const r500 = vi.fn(async () => new Response("{}", { status: 500 }));
    await expect(
      fetchNearbyDealers(query, r500 as unknown as typeof fetch),
    ).resolves.toEqual({ kind: "error" });
    const boom = vi.fn(async () => {
      throw new Error("offline");
    });
    await expect(
      fetchNearbyDealers(query, boom as unknown as typeof fetch),
    ).resolves.toEqual({ kind: "error" });
  });
});
