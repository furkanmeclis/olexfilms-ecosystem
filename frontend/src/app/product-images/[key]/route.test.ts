import { beforeEach, describe, expect, it, vi } from "vitest";

const upstream = vi.hoisted(() => ({ fetch: vi.fn() }));
vi.mock("@/lib/server/upstream", () => ({
  fetchUpstreamStream: upstream.fetch,
}));

import { GET } from "./route";

const KEY = `${"a".repeat(32)}.png`;

function call(key: string, headers: HeadersInit = {}) {
  return GET(
    new Request(`http://localhost/product-images/${key}`, { headers }),
    {
      params: Promise.resolve({ key }),
    },
  );
}

beforeEach(() => upstream.fetch.mockReset());

describe("GET /product-images/[key]", () => {
  it("404s a malformed key without calling the backend", async () => {
    const res = await call("../secret.svg");
    expect(res.status).toBe(404);
    expect(upstream.fetch).not.toHaveBeenCalled();
  });

  it("streams the image with its cache headers", async () => {
    upstream.fetch.mockResolvedValue({
      status: 200,
      headers: new Headers({
        "content-type": "image/png",
        etag: '"abc"',
        "cache-control": "public, max-age=86400",
        "set-cookie": "x=1",
      }),
      body: new Response("png").body,
    });
    const res = await call(KEY, { "If-None-Match": '"old"' });
    expect(res.status).toBe(200);
    expect(res.headers.get("etag")).toBe('"abc"');
    expect(res.headers.get("cache-control")).toBe("public, max-age=86400");
    expect(res.headers.get("set-cookie")).toBeNull();
    expect(await res.text()).toBe("png");
    const [path, init] = upstream.fetch.mock.calls[0] as [
      string,
      { headers: Headers },
    ];
    expect(path).toBe(`public/product-images/${KEY}`);
    expect(init.headers.get("If-None-Match")).toBe('"old"');
  });

  it("passes 304 through and hides other upstream errors", async () => {
    upstream.fetch.mockResolvedValueOnce({
      status: 304,
      headers: new Headers({ etag: '"abc"' }),
      body: null,
    });
    expect((await call(KEY)).status).toBe(304);
    upstream.fetch.mockResolvedValueOnce({
      status: 500,
      headers: new Headers(),
      body: null,
    });
    expect((await call(KEY)).status).toBe(502);
  });
});
