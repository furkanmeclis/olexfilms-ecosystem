// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  me: vi.fn(),
  pathname: "/portal/profile",
  dir: "ltr",
}));
vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string;
    children: React.ReactNode;
  }) => createElement("a", { href, ...props }, children),
}));
vi.mock("next/navigation", () => ({
  usePathname: () => state.pathname,
  useRouter: () => ({ replace: vi.fn(), refresh: vi.fn() }),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: state.dir === "rtl" ? "ar" : "en",
    dir: state.dir,
  }),
}));
vi.mock("@/features/portal/lib/portal-client", () => ({
  portalApi: { me: state.me },
  portalSignOut: vi.fn(),
}));
vi.mock("@/features/portal/components/ai-consent-dialog", () => ({
  AIConsentDialog: () => null,
}));
vi.mock("@/features/portal/components/portal-fleet", () => ({
  PortalFleetHome: () => createElement("div", { "data-testid": "fleet-home" }),
}));
vi.mock("@/features/ai-assistant/hooks/use-portal-assistant-visible", () => ({
  usePortalAssistantVisible: () => false,
}));

import { PortalHome } from "./portal-home";
import { withSidebar } from "@/test/sidebar";
import { PortalNav } from "./portal-nav";
import { PortalLoading } from "./portal-page";

let host: HTMLDivElement;
let root: Root;
let client: QueryClient;
async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}
async function mount(component: typeof PortalHome | typeof PortalNav) {
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        withSidebar(createElement(component)),
      ),
    );
  });
  await flush();
}
beforeEach(() => {
  (
    globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true;
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  state.dir = "ltr";
  state.pathname = "/portal/profile";
  state.me.mockReset().mockResolvedValue({ user: {}, roles: ["customer"] });
});
afterEach(() => {
  act(() => root.unmount());
  client.clear();
  host.remove();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("portal view states", () => {
  it("announces loading without exposing skeletons to screen readers", async () => {
    await act(async () => {
      root.render(createElement(PortalLoading, { label: "Loading", rows: 2 }));
    });
    expect(host.querySelector('[role="status"]')?.textContent).toBe("Loading");
    expect(host.querySelector("[aria-hidden]")?.children).toHaveLength(2);
  });

  it("waits for the account before showing customer shortcuts", async () => {
    state.me.mockReturnValue(new Promise(() => {}));
    await mount(PortalHome);
    expect(host.querySelector('[role="status"]')?.textContent).toBe(
      "common.loading",
    );
    expect(host.querySelector('[data-testid="portal-shortcuts"]')).toBeNull();
  });

  it("retries a failed account load and shows all six shortcuts in RTL", async () => {
    state.dir = "rtl";
    state.me.mockRejectedValueOnce(new Error("offline"));
    await mount(PortalHome);
    expect(host.querySelector('[role="alert"]')).not.toBeNull();
    expect(host.querySelector('[data-testid="portal-shortcuts"]')).toBeNull();
    const retry = host.querySelector(
      '[role="alert"] button',
    ) as HTMLButtonElement;
    await act(async () => {
      retry.click();
    });
    await flush();
    expect(host.querySelector('[role="alert"]')).toBeNull();
    expect(
      host
        .querySelector('[data-testid="portal-home-page"]')
        ?.getAttribute("dir"),
    ).toBe("rtl");
    expect(
      host.querySelectorAll('[data-testid="portal-shortcuts"] a'),
    ).toHaveLength(6);
    expect(
      host
        .querySelector('[data-testid="portal-book-link"]')
        ?.getAttribute("href"),
    ).toBe("/portal/appointments/new");
  });

  it("shows the fleet home without customer booking actions", async () => {
    state.me.mockResolvedValue({ user: {}, roles: ["fleet"] });
    await mount(PortalHome);
    expect(host.querySelector('[data-testid="fleet-home"]')).not.toBeNull();
    expect(host.querySelector('[data-testid="portal-book-link"]')).toBeNull();
  });

  it("scrolls the active profile tab into view for notification preferences", async () => {
    state.pathname = "/portal/preferences";
    const scroll = vi.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      configurable: true,
      value: scroll,
    });
    await mount(PortalNav);
    const active = host.querySelector('[aria-current="page"]');
    expect(active?.getAttribute("href")).toBe("/portal/profile");
    expect(scroll).toHaveBeenCalledWith({ block: "nearest", inline: "center" });
    delete (HTMLElement.prototype as { scrollIntoView?: unknown })
      .scrollIntoView;
  });
});
