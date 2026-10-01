import { afterEach, describe, expect, it, vi } from "vitest";

import {
  brandLogoUpstreamPath,
  proxyPublicImage,
  vehicleHeroUpstreamPath,
} from "./public-image-proxy";

const UUID = "0b5c4a39-6a43-4d47-9a3f-1f3a2b4c5d6e";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("public image paths", () => {
  it("maps brand logos and heroes to the Go public routes", () => {
    expect(brandLogoUpstreamPath(UUID)).toBe(`public/brand-logos/${UUID}`);
    expect(brandLogoUpstreamPath("../etc")).toBeNull();
    expect(vehicleHeroUpstreamPath(["default"])).toBe(
      "public/vehicle-heroes/default",
    );
    expect(vehicleHeroUpstreamPath(["models", UUID])).toBe(
      `public/vehicle-heroes/models/${UUID}`,
    );
    expect(vehicleHeroUpstreamPath(["brands", "x"])).toBeNull();
    expect(vehicleHeroUpstreamPath(["other", UUID])).toBeNull();
  });
});

describe("proxyPublicImage", () => {
  it("keeps ETag and Cache-Control and forwards If-None-Match", async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response("png", {
          status: 200,
          headers: {
            "content-type": "image/png",
            etag: '"abc"',
            "cache-control": "public, max-age=86400",
            "set-cookie": "leak=1",
          },
        }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await proxyPublicImage(
      new Request(`http://localhost/brand-logos/${UUID}`, {
        headers: { "if-none-match": '"old"', cookie: "session=1" },
      }),
      brandLogoUpstreamPath(UUID),
    );

    expect(res.status).toBe(200);
    expect(res.headers.get("etag")).toBe('"abc"');
    expect(res.headers.get("cache-control")).toBe("public, max-age=86400");
    expect(res.headers.get("content-type")).toBe("image/png");
    expect(res.headers.get("set-cookie")).toBeNull();
    const calls = fetchMock.mock.calls as unknown as [string, RequestInit][];
    const [url, init] = calls[0]!;
    expect(url).toMatch(new RegExp(`/v1/public/brand-logos/${UUID}$`));
    const sent = new Headers(init.headers);
    expect(sent.get("if-none-match")).toBe('"old"');
    expect(sent.get("cookie")).toBeNull();
    expect(sent.get("authorization")).toBeNull();
  });

  it("passes 304 through with the cache headers", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(null, {
            status: 304,
            headers: {
              etag: '"abc"',
              "cache-control": "public, max-age=86400",
            },
          }),
      ),
    );
    const res = await proxyPublicImage(
      new Request("http://localhost/vehicle-heroes/default", {
        headers: { "if-none-match": '"abc"' },
      }),
      vehicleHeroUpstreamPath(["default"]),
    );
    expect(res.status).toBe(304);
    expect(res.headers.get("etag")).toBe('"abc"');
    expect(res.headers.get("cache-control")).toBe("public, max-age=86400");
  });

  it("answers 404 for a malformed path without calling Go", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const res = await proxyPublicImage(
      new Request("http://localhost/brand-logos/nope"),
      brandLogoUpstreamPath("nope"),
    );
    expect(res.status).toBe(404);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
