import { createElement, type ReactElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

const upstream = vi.hoisted(() => ({
  calls: [] as string[],
  status: 200,
  body: "",
}));

vi.mock("next/headers", () => ({
  cookies: async () => ({ get: () => undefined }),
  headers: async () => new Headers({ host: "olexfilms.app" }),
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
  fetchUpstream: async (path: string) => {
    upstream.calls.push(path);
    return {
      status: upstream.status,
      headers: new Headers(),
      body: new TextEncoder().encode(upstream.body).buffer,
    };
  },
  clientIpFromHeaders: () => null,
  forwardedHostFromHeaders: (h: Headers) => h.get("host"),
}));

import PublicQuotePage, { generateMetadata } from "./page";

const TOKEN = "3f1c2b7a-8d4e-4b6f-9a1c-2e3d4f5a6b7c";

function props(token = TOKEN) {
  return {
    params: Promise.resolve({ token }),
    searchParams: Promise.resolve({ lang: "tr" }),
  };
}

beforeEach(() => {
  upstream.calls = [];
  upstream.status = 200;
  upstream.body = JSON.stringify({
    data: {
      uuid: "9b2f0c1d-1111-4222-8333-444455556666",
      display_no: "Q-000042",
      organization_name: "Olex Kadıköy",
      currency: "TRY",
      subtotal: "100.00",
      discount_total: "0.00",
      tax_total: "0.00",
      grand_total: "100.00",
      lines: [],
      pdf: { url: "/x" },
    },
  });
});

describe("/teklif/[token] (TEC-320)", () => {
  it("renders the quote and is never indexed", async () => {
    const html = renderToStaticMarkup(
      (await PublicQuotePage(props())) as ReactElement,
    );
    expect(html).toContain("Olex Kadıköy");
    expect(upstream.calls).toEqual([`public/quotes/${TOKEN}`]);
    const meta = await generateMetadata(props());
    expect(meta.robots).toMatchObject({ index: false, follow: false });
    expect(meta.referrer).toBe("no-referrer");
  });

  it("answers 404 for an unknown token", async () => {
    upstream.status = 404;
    await expect(PublicQuotePage(props())).rejects.toThrow("NEXT_NOT_FOUND");
  });

  it("answers 404 for a malformed token without calling Go", async () => {
    await expect(PublicQuotePage(props("not-a-token"))).rejects.toThrow(
      "NEXT_NOT_FOUND",
    );
    expect(upstream.calls).toEqual([]);
  });
});
