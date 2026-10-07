import { describe, expect, it } from "vitest";

import { site } from "@/config/site";

import robots from "./robots";

describe("robots (TEC-251)", () => {
  it("disallows the signed-in and internal areas", () => {
    const result = robots();
    const rules = Array.isArray(result.rules) ? result.rules : [result.rules];
    expect(rules).toHaveLength(1);
    const rule = rules[0]!;
    expect(rule.userAgent).toBe("*");
    expect(rule.allow).toEqual(["/"]);
    expect(rule.disallow).toEqual([
      "/panel",
      "/portal",
      "/api",
      "/platform",
      "/t/",
      "/profile",
      "/share/",
      "/teklif/",
      "/abonelik-iptal/",
    ]);
  });

  it("points at the sitemap", () => {
    expect(robots().sitemap).toBe(`${site.url}/sitemap.xml`);
  });
});
