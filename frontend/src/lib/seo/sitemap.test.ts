import { beforeEach, describe, expect, it, vi } from "vitest";

import { SUPPORTED_LOCALES } from "@/config/i18n";
import { site } from "@/config/site";

const upstream = vi.hoisted(() => ({
  calls: [] as { path: string; headers: Headers }[],
  status: 200,
  body: "" as string,
  /** Answer of the dealer application config (TEC-320). */
  configBody: "" as string,
}));

const CONFIG_PATH = "public/dealer-applications/config";

vi.mock("@/lib/server/upstream", () => ({
  fetchUpstream: async (
    path: string,
    init: { method: string; headers?: HeadersInit },
  ) => {
    upstream.calls.push({ path, headers: new Headers(init.headers) });
    const config = path === CONFIG_PATH;
    return {
      status: config ? 200 : upstream.status,
      headers: new Headers(),
      body: new TextEncoder().encode(
        config ? upstream.configBody : upstream.body,
      ).buffer,
    };
  },
}));

import sitemap from "@/app/sitemap";

import { buildSitemap, localeAlternates } from "./sitemap";

const landing = `${site.url}/`;

describe("localeAlternates (TEC-251)", () => {
  it("lists the 13 languages via ?lang= plus x-default", () => {
    const langs = localeAlternates("https://olex.test/bayi/kadikoy");
    expect(Object.keys(langs)).toHaveLength(SUPPORTED_LOCALES.length + 1);
    expect(SUPPORTED_LOCALES).toHaveLength(13);
    expect(langs.tr).toBe("https://olex.test/bayi/kadikoy?lang=tr");
    expect(langs["zh-CN"]).toBe("https://olex.test/bayi/kadikoy?lang=zh-CN");
    expect(langs.ar).toBe("https://olex.test/bayi/kadikoy?lang=ar");
    expect(langs["x-default"]).toBe("https://olex.test/bayi/kadikoy");
  });
});

describe("buildSitemap (TEC-251)", () => {
  it("has landing and dealer URLs, never panel or portal", () => {
    const now = new Date("2026-10-03T00:00:00Z");
    const entries = buildSitemap(
      "https://olex.test/",
      [
        { code: "olex-kadikoy", updated_at: "2026-09-01T10:00:00Z" },
        { code: "olex-ankara", updated_at: "not-a-date" },
      ],
      now,
    );
    const urls = entries.map((e) => e.url);
    expect(urls).toEqual([
      "https://olex.test/",
      "https://olex.test/bayi/olex-kadikoy",
      "https://olex.test/bayi/olex-ankara",
    ]);
    expect(urls.some((u) => u.includes("/panel"))).toBe(false);
    expect(urls.some((u) => u.includes("/portal"))).toBe(false);
    expect(entries[1]!.lastModified).toEqual(new Date("2026-09-01T10:00:00Z"));
    expect(entries[2]!.lastModified).toEqual(now);
    for (const entry of entries) {
      expect(Object.keys(entry.alternates?.languages ?? {})).toHaveLength(14);
    }
  });
});

describe("sitemap route (TEC-251)", () => {
  beforeEach(() => {
    upstream.calls = [];
    upstream.status = 200;
    upstream.body = "";
    upstream.configBody = JSON.stringify({ data: { enabled: false } });
  });

  it("reads active dealers from the public list for the site host", async () => {
    upstream.body = JSON.stringify({
      success: true,
      data: {
        items: [
          { code: "olex-kadikoy", updated_at: "2026-09-01T10:00:00Z" },
          { code: "../panel", updated_at: "2026-09-01T10:00:00Z" },
        ],
      },
    });
    const entries = await sitemap();
    const dealersCall = upstream.calls.find((c) => c.path === "public/dealers");
    expect(dealersCall).toBeDefined();
    expect(dealersCall!.headers.get("x-forwarded-host")).toBe(
      new URL(site.url).host,
    );
    const urls = entries.map((e) => e.url);
    expect(urls).toEqual([landing, `${site.url}/bayi/olex-kadikoy`]);
    expect(urls.join(" ")).not.toMatch(/\/(panel|portal)/);
  });

  it("falls back to the landing page when Go fails", async () => {
    upstream.status = 500;
    const entries = await sitemap();
    expect(entries.map((e) => e.url)).toEqual([landing]);
  });

  it("lists /bayi-basvuru only while the form is open (TEC-320)", async () => {
    expect((await sitemap()).map((e) => e.url)).toEqual([landing]);
    upstream.configBody = JSON.stringify({ data: { enabled: true } });
    const entries = await sitemap();
    expect(entries.map((e) => e.url)).toEqual([
      landing,
      `${site.url}/bayi-basvuru`,
    ]);
    expect(Object.keys(entries[1]!.alternates?.languages ?? {})).toHaveLength(
      14,
    );
    const configCall = upstream.calls.find((c) => c.path === CONFIG_PATH);
    expect(configCall!.headers.get("x-forwarded-host")).toBe(
      new URL(site.url).host,
    );
  });

  it("never lists quote links (TEC-320)", () => {
    const urls = buildSitemap("https://olex.test", [], new Date(), {
      dealerApplication: true,
    }).map((e) => e.url);
    expect(urls).toEqual([
      "https://olex.test/",
      "https://olex.test/bayi-basvuru",
    ]);
    expect(urls.join(" ")).not.toContain("/teklif");
  });

  it("ignores a malformed body", async () => {
    upstream.body = "{oops";
    expect((await sitemap()).map((e) => e.url)).toEqual([landing]);
  });
});
