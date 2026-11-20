import { describe, expect, it } from "vitest";

import { serializeJsonLd } from "./json-ld";

describe("serializeJsonLd", () => {
  it("cannot close the script element", () => {
    const value = {
      name: "Bayi</script><script>alert(1)</script>",
      address: "<!-- & > \u2028",
    };
    const out = serializeJsonLd(value);
    expect(out).not.toMatch(/[<>&\u2028\u2029]/);
    expect(out.toLowerCase()).not.toContain("</script");
    expect(JSON.parse(out)).toEqual(value);
  });

  it("keeps plain values unchanged", () => {
    const value = { "@type": "AutoRepair", name: "Olex Kadıköy" };
    expect(serializeJsonLd(value)).toBe(JSON.stringify(value));
  });
});
