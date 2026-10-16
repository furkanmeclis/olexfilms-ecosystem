import fs from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

import { platformNav, tenantNav } from "@/config/nav";
import { routes } from "@/config/routes";
import { platformSettingsNav, tenantSettingsNav } from "@/config/settings-nav";
import { settingsRedirects } from "@/config/settings-redirects";
import type { NavCatalog } from "@/features/nav-engine/types";
import nextConfig from "../../next.config";

const APP = path.resolve(__dirname, "../app");

const hrefs = (catalog: NavCatalog) =>
  catalog.groups.flatMap((g) => g.items.map((i) => i.href));

/** Whether a page.tsx serves `href` (route groups and [params] resolved). */
function hasPage(href: string, dynamic = true): boolean {
  const segments = href.split(/[?#]/, 1)[0].split("/").filter(Boolean);
  const walk = (dir: string, rest: string[]): boolean => {
    if (!rest.length) {
      if (fs.existsSync(path.join(dir, "page.tsx"))) return true;
    }
    const entries = fs.readdirSync(dir, { withFileTypes: true });
    for (const entry of entries) {
      if (!entry.isDirectory()) continue;
      const name = entry.name;
      const next = path.join(dir, name);
      if (name.startsWith("(") && walk(next, rest)) return true;
      if (!rest.length) continue;
      if (name === rest[0] && walk(next, rest.slice(1))) return true;
      if (dynamic && /^\[[^.]+\]$/.test(name) && walk(next, rest.slice(1)))
        return true;
    }
    return false;
  };
  return walk(APP, segments);
}

describe("settings area (TEC-771)", () => {
  it("keeps every settings page under the settings root", () => {
    for (const href of hrefs(platformSettingsNav)) {
      expect(href.startsWith(`${routes.platform.settings.root}/`), href).toBe(
        true,
      );
    }
    for (const href of hrefs(tenantSettingsNav("acme"))) {
      expect(
        href.startsWith(`${routes.tenant.settings.root("acme")}/`),
        href,
      ).toBe(true);
    }
  });

  it("links only to pages that exist", () => {
    const all = [
      ...hrefs(platformNav),
      ...hrefs(platformSettingsNav),
      ...hrefs(tenantNav("acme")),
      ...hrefs(tenantSettingsNav("acme")),
    ];
    const missing = all.filter((href) => !hasPage(href));
    expect(missing).toEqual([]);
  });

  it("does not list a page in both the sidebar and settings", () => {
    const main = new Set([...hrefs(platformNav), ...hrefs(tenantNav("acme"))]);
    const both = [
      ...hrefs(platformSettingsNav),
      ...hrefs(tenantSettingsNav("acme")),
    ].filter((href) => main.has(href));
    expect(both).toEqual([]);
  });

  it("keeps the platform sidebar short", () => {
    const count = platformNav.groups.reduce((n, g) => n + g.items.length, 0);
    expect(count).toBeLessThanOrEqual(8);
  });

  it("redirects every moved path to an existing page", async () => {
    const redirects = (await nextConfig.redirects?.()) ?? [];
    for (const redirect of settingsRedirects) {
      expect(redirects).toContainEqual(redirect);
      const target = redirect.destination
        .replace("/:path*", "")
        .replace(":slug", "acme");
      // A moved folder may hold only sub pages (integrations/*).
      expect(hasPage(target) || hasPage(`${target}/github`), target).toBe(true);
      const source = redirect.source
        .replace("/:path*", "")
        .replace(":slug", "[slug]");
      // Redirects run before routing, so a [param] sibling is harmless;
      // only a static page left behind would be shadowed.
      expect(hasPage(source, false), `${source} still has a page`).toBe(false);
    }
  });
});
