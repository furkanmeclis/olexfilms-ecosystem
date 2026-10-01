import { createElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

const request = vi.hoisted(() => ({
  cookies: {} as Record<string, string>,
  headers: {} as Record<string, string>,
}));

vi.mock("next/headers", () => ({
  cookies: async () => ({
    get: (name: string) =>
      name in request.cookies ? { value: request.cookies[name] } : undefined,
  }),
  headers: async () => new Headers(request.headers),
}));

vi.mock("next/font/local", () => ({
  default: () => ({ variable: "font" }),
}));

vi.mock("./globals.css", () => ({}));

vi.mock("@/lib/i18n/messages", () => ({
  loadMessages: async () => ({}),
}));

vi.mock("@/providers/app-providers", () => ({
  AppProviders: ({ children }: { children: ReactNode }) => children,
}));

import RootLayout from "./layout";

async function renderHtml() {
  const tree = await RootLayout({ children: createElement("main") });
  return renderToStaticMarkup(tree);
}

describe("RootLayout <html lang dir> (TEC-142)", () => {
  beforeEach(() => {
    request.cookies = {};
    request.headers = {};
  });

  it("renders Arabic RTL from Accept-Language when there is no locale cookie", async () => {
    request.headers = { "accept-language": "ar" };
    const html = await renderHtml();
    expect(html).toMatch(/<html[^>]*lang="ar"/);
    expect(html).toMatch(/<html[^>]*dir="rtl"/);
  });

  it("lets the locale cookie win over Accept-Language", async () => {
    request.cookies = { NEXT_LOCALE: "en" };
    request.headers = { "accept-language": "ar" };
    const html = await renderHtml();
    expect(html).toMatch(/<html[^>]*lang="en"/);
    expect(html).toMatch(/<html[^>]*dir="ltr"/);
  });

  it("falls back to tr when no header language is supported", async () => {
    request.headers = { "accept-language": "ja,ko;q=0.8" };
    const html = await renderHtml();
    expect(html).toMatch(/<html[^>]*lang="tr"/);
    expect(html).toMatch(/<html[^>]*dir="ltr"/);
  });
});
