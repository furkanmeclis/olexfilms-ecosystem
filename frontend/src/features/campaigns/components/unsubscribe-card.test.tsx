// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { UNSUBSCRIBE_TOKEN_RE } from "../lib/unsubscribe";
import { UnsubscribeCard, type UnsubscribeTexts } from "./unsubscribe-card";

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: { href: string; children: unknown } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const TEXTS: UnsubscribeTexts = {
  title: "T",
  body: "B",
  confirm: "Confirm",
  done_title: "Done",
  done_body: "DB",
  invalid_title: "Invalid",
  invalid_body: "IB",
  error: "Err",
  home: "Home",
};
const TOKEN = "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCd";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function render(fetchImpl: typeof fetch, token = TOKEN) {
  await act(async () => {
    root.render(
      createElement(UnsubscribeCard, {
        token,
        texts: TEXTS,
        lang: "ar",
        dir: "rtl",
        fetchImpl,
      }),
    );
  });
}

async function click() {
  const button = container.querySelector("button")!;
  await act(async () => {
    button.click();
  });
}

describe("UnsubscribeCard (TEC-407)", () => {
  it("unsubscribes only after the explicit click", async () => {
    expect(UNSUBSCRIBE_TOKEN_RE.test(TOKEN)).toBe(true);
    const fetchImpl = vi.fn(
      async () => new Response("{}", { status: 200 }),
    ) as unknown as typeof fetch;
    await render(fetchImpl);
    expect(fetchImpl).not.toHaveBeenCalled();
    expect(container.querySelector("main")?.getAttribute("dir")).toBe("rtl");
    await click();
    expect(fetchImpl).toHaveBeenCalledWith(
      "/api/v1/public/campaigns/unsubscribe",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ token: TOKEN }),
      }),
    );
    expect(container.textContent).toContain("Done");
    expect(container.querySelector("a")?.getAttribute("href")).toBe("/");
  });

  it("shows the invalid link state on 404", async () => {
    const fetchImpl = vi.fn(
      async () => new Response("{}", { status: 404 }),
    ) as unknown as typeof fetch;
    await render(fetchImpl);
    await click();
    expect(container.textContent).toContain("Invalid");
  });

  it("never posts a malformed token", async () => {
    const never = vi.fn() as unknown as typeof fetch;
    await render(never, "bad token");
    await click();
    expect(never).not.toHaveBeenCalled();
    expect(container.textContent).toContain("Invalid");
  });

  it("keeps the button and shows an alert on a server error", async () => {
    const fetchImpl = vi.fn(
      async () => new Response("{}", { status: 500 }),
    ) as unknown as typeof fetch;
    await render(fetchImpl);
    await click();
    expect(container.querySelector('[role="alert"]')?.textContent).toBe("Err");
    expect(container.querySelector("button")).not.toBeNull();
  });
});
