import { describe, expect, it } from "vitest";

import nextConfig from "../../next.config";

describe("next.config redirects", () => {
  it("sends Go's quote links to the public quote page (TEC-320)", async () => {
    const redirects = (await nextConfig.redirects?.()) ?? [];
    expect(redirects).toContainEqual({
      source: "/portal/quotes/:token",
      destination: "/teklif/:token",
      permanent: false,
    });
  });
});
