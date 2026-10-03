import { existsSync } from "node:fs";
import path from "node:path";

import { describe, expect, it, vi } from "vitest";

import { site } from "@/config/site";

vi.mock("next/headers", () => ({
  cookies: async () => ({ get: () => undefined }),
  headers: async () => new Headers(),
}));

vi.mock("@/features/landing/components/landing-view", () => ({
  LandingView: () => null,
}));

import { generateMetadata } from "./page";

describe("landing metadata (TEC-251)", () => {
  it("has og:title, og:image and hreflang alternates", async () => {
    const meta = await generateMetadata({
      searchParams: Promise.resolve({ lang: "en" }),
    });
    const og = meta.openGraph as {
      title?: string;
      images?: { url: string; width?: number; height?: number }[];
    };
    expect(og.title).toBeTruthy();
    expect(og.title).toBe(
      (meta.title as { absolute: string } | undefined)?.absolute,
    );
    expect(og.images?.[0]?.url).toBe(site.ogImage.url);
    expect(og.images?.[0]?.width).toBe(1200);
    expect(og.images?.[0]?.height).toBe(630);
    const languages = meta.alternates?.languages as Record<string, string>;
    expect(Object.keys(languages)).toHaveLength(14);
    expect(languages.tr).toBe(`${site.url}/?lang=tr`);
  });

  it("ships the default OG image as a local asset", () => {
    const file = path.resolve(
      __dirname,
      "../../../public",
      site.ogImage.url.slice(1),
    );
    expect(existsSync(file)).toBe(true);
  });
});
