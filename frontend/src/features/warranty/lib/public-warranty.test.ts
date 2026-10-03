import { describe, expect, it, vi } from "vitest";

import {
  fetchPublicWarranty,
  parseRetryAfter,
  statusTone,
  type PublicWarranty,
  type UpstreamFetcher,
} from "./public-warranty";

const CODE = "AbCdEfGhIjKlMnOpQrSt_-";

const sampleWarranty: PublicWarranty = {
  public_code: CODE,
  status: "active",
  start_at: "2026-01-15T10:00:00Z",
  end_at: "2027-01-15T20:59:59Z",
  days_remaining: 105,
  product: { name: "Olex PPF Gloss" },
  brand: { name: "Olexfilms", slug: "olex" },
  dealer: { name: "Kadıköy Bayi", city: "İstanbul" },
  vehicle: {
    brand_name: "BMW",
    brand_logo_uuid: "0b5c4a39-6a43-4d47-9a3f-1f3a2b4c5d6e",
    model_name: "M3",
    model_year: 2024,
    plate_masked: "34 *** 12",
    vin_last4: "6752",
  },
};

function reply(status: number, body: unknown, headers: HeadersInit = {}) {
  const fetcher: UpstreamFetcher = vi.fn(async () => ({
    status,
    headers: new Headers(headers),
    body: new TextEncoder().encode(JSON.stringify(body)).buffer as ArrayBuffer,
  }));
  return fetcher;
}

const req = { clientIp: "203.0.113.5", forwardedHost: "olexfilms.app" };

describe("fetchPublicWarranty", () => {
  it("returns the warranty and forwards IP and host", async () => {
    const fetcher = reply(200, { success: true, data: sampleWarranty });
    const result = await fetchPublicWarranty(CODE, req, fetcher);
    expect(result).toEqual({ kind: "ok", warranty: sampleWarranty });
    const [path, init] = vi.mocked(fetcher).mock.calls[0]!;
    expect(path).toBe(`public/warranties/${CODE}`);
    const h = new Headers(init.headers);
    expect(h.get("x-forwarded-for")).toBe("203.0.113.5");
    expect(h.get("x-forwarded-host")).toBe("olexfilms.app");
  });

  it("treats a malformed code as not found without a request", async () => {
    const fetcher = reply(200, {});
    expect(await fetchPublicWarranty("bad code", req, fetcher)).toEqual({
      kind: "not_found",
    });
    expect(await fetchPublicWarranty("x".repeat(33), req, fetcher)).toEqual({
      kind: "not_found",
    });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("accepts an old hub warranty number (TEC-248)", async () => {
    const fetcher = reply(200, {
      data: { ...sampleWarranty, public_code: "DS7K2M9QX4" },
    });
    const out = await fetchPublicWarranty("DS7K2M9QX4", req, fetcher);
    expect(out.kind).toBe("ok");
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("maps 404, 429 and failures", async () => {
    expect(await fetchPublicWarranty(CODE, req, reply(404, {}))).toEqual({
      kind: "not_found",
    });
    expect(
      await fetchPublicWarranty(
        CODE,
        req,
        reply(429, {}, { "Retry-After": "42" }),
      ),
    ).toEqual({ kind: "rate_limited", retryAfter: 42 });
    expect(await fetchPublicWarranty(CODE, req, reply(500, {}))).toEqual({
      kind: "error",
    });
    const broken: UpstreamFetcher = async () => {
      throw new Error("down");
    };
    expect(await fetchPublicWarranty(CODE, req, broken)).toEqual({
      kind: "error",
    });
  });
});

describe("helpers", () => {
  it("parses Retry-After seconds", () => {
    expect(parseRetryAfter("30")).toBe(30);
    expect(parseRetryAfter(null)).toBeNull();
    expect(parseRetryAfter("soon")).toBeNull();
    expect(parseRetryAfter("0")).toBeNull();
  });

  it("maps status to badge tone", () => {
    expect(statusTone("active")).toBe("success");
    expect(statusTone("expired")).toBe("warning");
    expect(statusTone("void")).toBe("danger");
  });
});
