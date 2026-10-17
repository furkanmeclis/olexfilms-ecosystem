// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const portal = vi.hoisted(() => ({
  me: vi.fn(),
  updateProfile: vi.fn(),
}));
const account = vi.hoisted(() => ({
  listSessions: vi.fn(),
  revokeSession: vi.fn(),
  revokeOtherSessions: vi.fn(),
  changePassword: vi.fn(),
}));
const totp = vi.hoisted(() => ({
  getTOTPStatus: vi.fn(),
  setupTOTP: vi.fn(),
  confirmTOTP: vi.fn(),
  disableTOTP: vi.fn(),
}));
const signOut = vi.hoisted(() => vi.fn());
const setLocale = vi.hoisted(() => vi.fn());
const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string;
    children: unknown;
  } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));
vi.mock("next/navigation", () => ({
  usePathname: () => "/portal/profile",
  useRouter: () => ({ replace: vi.fn(), push: vi.fn(), refresh: vi.fn() }),
}));
vi.mock("sonner", () => ({ toast: toasts }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    dir: "ltr",
    setLocale,
    t: (key: string) => key,
    format: { dateTime: (v: string) => `dt(${v})`, date: (v: string) => v },
  }),
}));
vi.mock("@/providers/auth-provider", () => ({
  useAuth: () => ({ logout: vi.fn() }),
}));
vi.mock("@/features/portal/lib/portal-client", async (orig) => ({
  ...(await orig<object>()),
  portalApi: portal,
  portalAccountApi: account,
  portalTotpApi: totp,
  portalSignOut: signOut,
}));

import { PortalProfile } from "./portal-profile";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  account.listSessions.mockResolvedValue({ items: [] });
  totp.getTOTPStatus.mockResolvedValue({ enabled: false });
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
});

async function flush() {
  for (let i = 0; i < 6; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(PortalProfile),
      ),
    );
  });
  await flush();
}

function setValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

describe("PortalProfile (TEC-771)", () => {
  it("saves the name through the portal profile call", async () => {
    portal.me.mockResolvedValue({
      user: { name: "Ayşe", surname: "Kaya", locale: "tr" },
      roles: ["customer"],
    });
    portal.updateProfile.mockResolvedValue({});
    await render();

    const name = container.querySelector<HTMLInputElement>(
      "#portal-profile-name",
    );
    expect(name?.value).toBe("Ayşe");
    await act(async () => setValue(name!, " Ayşe Nur "));
    const form = container.querySelector("form");
    await act(async () => {
      form?.dispatchEvent(
        new Event("submit", { bubbles: true, cancelable: true }),
      );
    });
    await flush();

    expect(portal.updateProfile).toHaveBeenCalledWith({
      name: "Ayşe Nur",
      surname: "Kaya",
      locale: "tr",
    });
    // The page language follows the saved profile language.
    expect(setLocale).toHaveBeenCalledWith("tr");
    expect(toasts.success).toHaveBeenCalledWith("portal.profile.saved");
  });

  it("links preferences and contracts and lists sessions for a customer", async () => {
    portal.me.mockResolvedValue({ user: { name: "Ali" }, roles: ["customer"] });
    await render();
    const hrefs = Array.from(container.querySelectorAll("a")).map((a) =>
      a.getAttribute("href"),
    );
    expect(hrefs).toEqual(
      expect.arrayContaining(["/portal/preferences", "/portal/contracts"]),
    );
    expect(account.listSessions).toHaveBeenCalled();
    expect(container.textContent).toContain("portal.profile.phone_login_hint");
    expect(
      container.querySelector('[data-testid="portal-totp-card"]'),
    ).toBeNull();
  });

  it("gives fleet accounts password and 2FA, without contracts", async () => {
    portal.me.mockResolvedValue({
      user: { name: "Filo", email: "filo@example.test" },
      roles: ["fleet"],
    });
    await render();
    expect(
      container.querySelector('[data-testid="portal-totp-card"]'),
    ).not.toBeNull();
    expect(totp.getTOTPStatus).toHaveBeenCalled();
    expect(container.textContent).toContain("auth.password.title");
    const hrefs = Array.from(container.querySelectorAll("a")).map((a) =>
      a.getAttribute("href"),
    );
    expect(hrefs).not.toContain("/portal/contracts");
  });

  it("signs out of the portal only", async () => {
    portal.me.mockResolvedValue({ user: { name: "Ali" }, roles: ["customer"] });
    signOut.mockResolvedValue(undefined);
    await render();
    await act(async () => {
      container
        .querySelector<HTMLButtonElement>(
          '[data-testid="portal-profile-logout"]',
        )
        ?.click();
    });
    await flush();
    expect(signOut).toHaveBeenCalled();
  });
});
