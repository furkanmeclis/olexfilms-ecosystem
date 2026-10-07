import { createElement, type ReactElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

const upstream = vi.hoisted(() => ({
  calls: [] as { path: string; headers: Headers }[],
  status: 200,
  body: "",
}));

vi.mock("next/headers", () => ({
  cookies: async () => ({ get: () => undefined }),
  headers: async () =>
    new Headers({ "x-real-ip": "203.0.113.5", host: "olexfilms.app" }),
}));
vi.mock("next/navigation", () => ({
  notFound: () => {
    throw new Error("NEXT_NOT_FOUND");
  },
}));
vi.mock("next/image", () => ({
  default: (props: Record<string, unknown>) =>
    createElement("img", { src: props.src, alt: props.alt }),
}));
vi.mock("@/lib/server/upstream", () => ({
  fetchUpstream: async (
    path: string,
    init: { method: string; headers?: HeadersInit },
  ) => {
    upstream.calls.push({ path, headers: new Headers(init.headers) });
    return {
      status: upstream.status,
      headers: new Headers(),
      body: new TextEncoder().encode(upstream.body).buffer,
    };
  },
  clientIpFromHeaders: (h: Headers) => h.get("x-real-ip"),
  forwardedHostFromHeaders: (h: Headers) => h.get("host"),
}));

import DealerApplicationPage, { generateMetadata } from "./page";

const props = { searchParams: Promise.resolve({ lang: "tr" }) };

beforeEach(() => {
  upstream.calls = [];
  upstream.status = 200;
  upstream.body = JSON.stringify({ data: { enabled: true } });
});

describe("/bayi-basvuru (TEC-320)", () => {
  it("renders the form while the config is enabled", async () => {
    const html = renderToStaticMarkup(
      (await DealerApplicationPage(props)) as ReactElement,
    );
    expect(html).toContain('data-slot="dealer-application-form"');
    expect(html).toContain('name="kvkk_consent"');
    expect(upstream.calls[0]!.path).toBe("public/dealer-applications/config");
    expect(upstream.calls[0]!.headers.get("x-forwarded-host")).toBe(
      "olexfilms.app",
    );
  });

  it("does not render the form (404) while the config is disabled", async () => {
    upstream.body = JSON.stringify({ data: { enabled: false } });
    await expect(DealerApplicationPage(props)).rejects.toThrow(
      "NEXT_NOT_FOUND",
    );
    const meta = await generateMetadata(props);
    expect(meta.robots).toEqual({ index: false, follow: false });
  });

  it("is a 404 when the host has no brand", async () => {
    upstream.status = 404;
    await expect(DealerApplicationPage(props)).rejects.toThrow(
      "NEXT_NOT_FOUND",
    );
  });

  it("shows an error card, not the form, when Go fails", async () => {
    upstream.status = 500;
    const html = renderToStaticMarkup(
      (await DealerApplicationPage(props)) as ReactElement,
    );
    expect(html).toContain('data-screen="error"');
    expect(html).not.toContain("dealer-application-form");
  });

  it("has hreflang alternates while open", async () => {
    const meta = await generateMetadata(props);
    const languages = meta.alternates?.languages as Record<string, string>;
    expect(Object.keys(languages)).toHaveLength(14);
    expect(languages.ar).toMatch(/\/bayi-basvuru\?lang=ar$/);
  });
});
