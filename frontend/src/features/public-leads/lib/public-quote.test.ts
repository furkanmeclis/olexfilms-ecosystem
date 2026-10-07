import { describe, expect, it, vi } from "vitest";

import type { UpstreamFetcher } from "@/features/warranty/lib/public-warranty";

import {
  fetchPublicQuote,
  isQuoteExpired,
  parseQuotePdfNotice,
  publicQuotePdfHref,
  type PublicQuote,
} from "./public-quote";

const TOKEN = "3f1c2b7a-8d4e-4b6f-9a1c-2e3d4f5a6b7c";
const request = { clientIp: "203.0.113.5", forwardedHost: "olexfilms.app" };

function fetcher(status: number, body = "", headers: HeadersInit = {}) {
  return vi.fn(async () => ({
    status,
    headers: new Headers(headers),
    body: new TextEncoder().encode(body).buffer as ArrayBuffer,
  })) as unknown as UpstreamFetcher & ReturnType<typeof vi.fn>;
}

describe("fetchPublicQuote (TEC-320)", () => {
  it("reads the quote and forwards the client IP and host", async () => {
    const f = fetcher(
      200,
      JSON.stringify({ data: { uuid: "u", lines: [], display_no: "Q-1" } }),
    );
    const result = await fetchPublicQuote(TOKEN, request, f);
    expect(result.kind).toBe("ok");
    const [path, init] = f.mock.calls[0]!;
    expect(path).toBe(`public/quotes/${TOKEN}`);
    const h = new Headers(init.headers);
    expect(h.get("x-forwarded-for")).toBe("203.0.113.5");
    expect(h.get("x-forwarded-host")).toBe("olexfilms.app");
  });

  it("is not_found for a malformed token without calling Go", async () => {
    const f = fetcher(200);
    expect(await fetchPublicQuote("../panel", request, f)).toEqual({
      kind: "not_found",
    });
    expect(f).not.toHaveBeenCalled();
  });

  it("maps 404, 429 and failures", async () => {
    expect(await fetchPublicQuote(TOKEN, request, fetcher(404))).toEqual({
      kind: "not_found",
    });
    expect(
      await fetchPublicQuote(
        TOKEN,
        request,
        fetcher(429, "", { "Retry-After": "120" }),
      ),
    ).toEqual({ kind: "rate_limited", retryAfter: 120 });
    expect(await fetchPublicQuote(TOKEN, request, fetcher(500))).toEqual({
      kind: "error",
    });
    expect(await fetchPublicQuote(TOKEN, request, fetcher(200, "{x"))).toEqual({
      kind: "error",
    });
  });
});

describe("public quote helpers (TEC-320)", () => {
  it("builds the PDF link and parses its notice", () => {
    expect(publicQuotePdfHref(TOKEN, "zh-CN")).toBe(
      `/teklif/${TOKEN}/pdf?lang=zh-CN`,
    );
    expect(parseQuotePdfNotice("pending")).toBe("pending");
    expect(parseQuotePdfNotice(["rate_limited"])).toBe("rate_limited");
    expect(parseQuotePdfNotice("other")).toBeNull();
  });

  it("tells an expired quote", () => {
    const now = new Date("2026-10-07T00:00:00Z");
    const q = (valid_until: string | null) =>
      ({ valid_until }) as unknown as PublicQuote;
    expect(isQuoteExpired(q("2026-10-06T00:00:00Z"), now)).toBe(true);
    expect(isQuoteExpired(q("2026-10-08T00:00:00Z"), now)).toBe(false);
    expect(isQuoteExpired(q(null), now)).toBe(false);
  });
});
