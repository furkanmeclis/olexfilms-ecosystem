import { describe, expect, it, vi } from "vitest";

import {
  fetchPublicWarranty,
  parseRetryAfter,
  hasActiveWarranty,
  sortWarranties,
  statusTone,
  type PublicService,
  type UpstreamFetcher,
} from "./public-warranty";

const CODE = "AbCdEfGhIjKlMnOpQrSt_-";

const sampleService: PublicService = {
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
      public_code: CODE,
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
  it("returns the service page and forwards IP and host", async () => {
    const fetcher = reply(200, { success: true, data: sampleService });
    const result = await fetchPublicWarranty(CODE, req, fetcher);
    expect(result).toEqual({ kind: "ok", service: sampleService });
    const [path, init] = vi.mocked(fetcher).mock.calls[0]!;
    expect(path).toBe(`public/service-warranties/${CODE}`);
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
      data: sampleService,
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
  it("orders active warranties first and spots an active one", () => {
    const w = sampleService.warranties[0]!;
    const list = sortWarranties([
      { ...w, public_code: "void1", status: "void" },
      { ...w, public_code: "exp1", status: "expired" },
      { ...w, public_code: "act2", end_at: "2028-01-01T00:00:00Z" },
      w,
    ]);
    expect(list.map((x) => x.public_code)).toEqual([
      CODE,
      "act2",
      "exp1",
      "void1",
    ]);
    expect(hasActiveWarranty(sampleService)).toBe(true);
    expect(
      hasActiveWarranty({
        ...sampleService,
        warranties: [{ ...w, status: "void" }],
      }),
    ).toBe(false);
  });

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
