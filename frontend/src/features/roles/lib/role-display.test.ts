import { describe, expect, it } from "vitest";

import tr from "@/locales/tr/roles.json";

import { roleDisplayDescription, roleDisplayName } from "./role-display";

const dict = tr as Record<string, string>;
const t = (key: string) => dict[key.replace(/^roles\./, "")] ?? key;

describe("role display (TEC-1089)", () => {
  it("names and describes center_sales", () => {
    expect(
      roleDisplayName({ slug: "center_sales", name: "Center sales" }, t),
    ).toBe("Satış temsilcisi");
    expect(roleDisplayDescription({ slug: "center_sales" }, t)).toContain(
      "alış fiyatı",
    );
  });

  it("falls back for custom roles", () => {
    expect(roleDisplayName({ slug: "custom_x", name: "Custom" }, t)).toBe(
      "Custom",
    );
    expect(roleDisplayDescription({ slug: "custom_x" }, t)).toBeNull();
  });
});
