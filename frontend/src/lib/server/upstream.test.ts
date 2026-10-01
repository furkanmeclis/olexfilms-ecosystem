import { describe, expect, it } from "vitest";

import { forwardedHostFromHeaders } from "./upstream";

describe("forwardedHostFromHeaders", () => {
  it("prefers the first X-Forwarded-Host hop", () => {
    const h = new Headers({
      "x-forwarded-host": "warranty.glorianppf.com, edge.internal",
      host: "frontend:3000",
    });
    expect(forwardedHostFromHeaders(h)).toBe("warranty.glorianppf.com");
  });

  it("falls back to Host", () => {
    expect(
      forwardedHostFromHeaders(new Headers({ host: "olexfilms.app" })),
    ).toBe("olexfilms.app");
  });

  it("returns null without host headers", () => {
    expect(forwardedHostFromHeaders(new Headers())).toBeNull();
  });
});
