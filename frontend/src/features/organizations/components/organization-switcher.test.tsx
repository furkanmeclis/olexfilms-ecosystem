// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const listMine = vi.fn();
const switchOrganizationContext = vi.fn();
const sessionUpdate = vi.fn();
const routerPush = vi.fn();

vi.mock("next-auth/react", () => ({
  useSession: () => ({
    data: null,
    status: "authenticated",
    update: sessionUpdate,
  }),
}));
vi.mock("next/navigation", () => ({
  useParams: () => ({ slug: "bayi-a" }),
  useRouter: () => ({ push: routerPush }),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));
vi.mock("@/providers/auth-provider", () => ({
  useAuth: () => ({ isAuthenticated: true }),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key }),
}));
vi.mock("@/services/auth.service", () => ({
  authService: {
    switchOrganizationContext: (slug: string) =>
      switchOrganizationContext(slug),
  },
}));
vi.mock("@/features/organizations/services/organizations.service", () => ({
  organizationsService: { listMine: () => listMine() },
}));
// Render the menu inline so items are clickable without pointer emulation.
vi.mock("@/components/ui/dropdown-menu", () => {
  const pass = ({ children }: { children?: ReactNode }) =>
    createElement("div", null, children);
  return {
    DropdownMenu: pass,
    DropdownMenuTrigger: pass,
    DropdownMenuContent: pass,
    DropdownMenuLabel: pass,
    DropdownMenuSeparator: () => null,
    DropdownMenuItem: ({
      children,
      onClick,
      disabled,
      ...rest
    }: {
      children?: ReactNode;
      onClick?: () => void;
      disabled?: boolean;
    }) =>
      createElement(
        "button",
        { type: "button", onClick, disabled, ...rest },
        children,
      ),
  };
});

import { OrganizationSwitcher } from "./organization-switcher";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const organizations = [
  {
    uuid: "11111111-1111-1111-1111-111111111111",
    slug: "bayi-a",
    name: "Bayi A",
    role: "owner",
    status: "active",
    type: "dealer",
    brand: { slug: "olex" },
    parent: {
      uuid: "33333333-3333-3333-3333-333333333333",
      name: "Olex Merkez",
    },
  },
  {
    uuid: "22222222-2222-2222-2222-222222222222",
    slug: "bayi-b",
    name: "Bayi B",
    role: "staff",
    status: "active",
    type: "dealer",
    brand: { slug: "olex" },
    parent: null,
  },
];

async function flush() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

describe("OrganizationSwitcher", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    listMine.mockReset().mockResolvedValue(organizations);
    switchOrganizationContext.mockReset().mockResolvedValue({});
    sessionUpdate.mockReset().mockResolvedValue(null);
    routerPush.mockReset();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    act(() => {
      root.render(
        createElement(
          QueryClientProvider,
          { client },
          createElement(OrganizationSwitcher),
        ),
      );
    });
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it("lists both organizations of the user", async () => {
    await flush();
    expect(listMine).toHaveBeenCalledTimes(1);
    expect(
      container.querySelector(
        '[data-testid="organization-switcher-item-bayi-a"]',
      ),
    ).not.toBeNull();
    expect(
      container.querySelector(
        '[data-testid="organization-switcher-item-bayi-b"]',
      ),
    ).not.toBeNull();
    expect(container.textContent).toContain("organizations.types.dealer");
    expect(container.textContent).toContain("Olex Merkez");
  });

  it("switches context, refreshes the session and opens the org panel", async () => {
    await flush();
    const item = container.querySelector<HTMLButtonElement>(
      '[data-testid="organization-switcher-item-bayi-b"]',
    );
    await act(async () => {
      item!.click();
    });
    await flush();
    expect(switchOrganizationContext).toHaveBeenCalledWith("bayi-b");
    expect(sessionUpdate).toHaveBeenCalledTimes(1);
    expect(routerPush).toHaveBeenCalledWith("/t/bayi-b");
  });

  it("does not switch when the active organization is chosen", async () => {
    await flush();
    const item = container.querySelector<HTMLButtonElement>(
      '[data-testid="organization-switcher-item-bayi-a"]',
    );
    await act(async () => {
      item!.click();
    });
    expect(switchOrganizationContext).not.toHaveBeenCalled();
  });
});
