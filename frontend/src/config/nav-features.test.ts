import { describe, expect, it } from "vitest";

import { platformNav, tenantNav } from "@/config/nav";
import type { NavGroupDef, NavItemDef } from "@/features/nav-engine/types";
import { wizardSteps } from "@/features/services/lib/wizard";

/**
 * TEC-509 add-on menu audit: every tenant menu entry of an F5 add-on screen
 * carries the add-on's module key in `feature`, so it is hidden while the
 * module is off (same gate as RequireFeature on the API, TEC-508). The key
 * is the tenant path segment after /t/{slug}.
 */
const F5_ADDON_SEGMENTS: Record<string, string> = {
  showcase: "dealer_showcase",
  fleets: "fleet",
  certificates: "certificates",
  "stock-forecast": "stock_forecast",
  performance: "performance",
  efficiency: "efficiency",
  "photo-standard": "photo_standard",
  // F5-08d screens; the entry must carry the key once it is added.
  "e-invoices": "e_invoice",
  "e-invoice": "e_invoice",
};

/** Add-ons that already have tenant screens (and so menu entries). */
const F5_ADDONS_WITH_MENU = [
  "dealer_showcase",
  "fleet",
  "certificates",
  "stock_forecast",
  "performance",
  "efficiency",
  "photo_standard",
];

/**
 * Platform entries of F5 add-ons are center catalog / review screens
 * (showcase approval queue, certificate types, expected consumption, photo
 * angles); the center switches modules for the network, so they have no
 * module gate (same exceptions as the backend route audit).
 */
const PLATFORM_EXEMPT = new Set([
  "showcases",
  "certificate-types",
  "part-consumption-expectations",
  "photo-angles",
]);

const SLUG = "acme";

function entries(groups: NavGroupDef[]) {
  return groups.flatMap((group) =>
    group.items.map((item) => ({ group, item })),
  );
}

function addonOf(item: NavItemDef): string | undefined {
  const prefix = `/t/${SLUG}/`;
  if (!item.href.startsWith(prefix)) return undefined;
  const segment = item.href.slice(prefix.length).split(/[/?#]/)[0];
  return F5_ADDON_SEGMENTS[segment];
}

describe("F5 add-on menu audit (TEC-509)", () => {
  const tenant = entries(tenantNav(SLUG).groups);

  it("every F5 add-on menu entry carries its module key", () => {
    const missing = tenant
      .map(({ group, item }) => ({ group, item, key: addonOf(item) }))
      .filter(({ key }) => key)
      .filter(({ item, key }) => item.feature !== key)
      .map(
        ({ item, key }) => `${item.id}: feature=${item.feature} want ${key}`,
      );
    expect(missing).toEqual([]);
  });

  it("groups holding add-on entries are gated by a module too", () => {
    const groups = tenant
      .filter(({ item }) => addonOf(item))
      .map(({ group }) => group)
      .filter((group) => group.id !== "tenant");
    for (const group of groups) expect(group.feature, group.id).toBeTruthy();
  });

  it("each add-on with screens has at least one gated menu entry", () => {
    const gated = new Set(
      tenant
        .filter(({ item }) => addonOf(item))
        .map(({ item }) => item.feature),
    );
    for (const key of F5_ADDONS_WITH_MENU) expect(gated, key).toContain(key);
  });

  it("platform F5 entries are only the documented center exceptions", () => {
    const f5Platform = entries(platformNav.groups)
      .map(({ item }) => item)
      .filter((item) =>
        /showcase|certificate|consumption|photo|fleet|forecast|performance|efficiency|invoice/.test(
          item.href,
        ),
      )
      .map((item) => item.id);
    for (const id of f5Platform) expect(PLATFORM_EXEMPT, id).toContain(id);
  });

  it("the photo step of the service wizard follows photo_standard", () => {
    expect(wizardSteps(true, false)).not.toContain("photos");
    expect(wizardSteps(true, true)).toContain("photos");
  });
});
