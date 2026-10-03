// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  createElement,
  Fragment,
  useSyncExternalStore,
  type ComponentType,
  type ReactNode,
} from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

/**
 * TEC-227: an organization switch is one-way. The previous organization's
 * context must not scope the session back while the switch is in flight.
 */

const ORG_A = "11111111-1111-1111-1111-111111111111";
const ORG_B = "22222222-2222-2222-2222-222222222222";
const UUID_BY_SLUG: Record<string, string> = {
  "bayi-a": ORG_A,
  "bayi-b": ORG_B,
};

// Minimal reactive session store standing in for next-auth's SessionProvider.
let sessionOrg: string | null = ORG_A;
const sessionListeners = new Set<() => void>();
function setSessionOrg(next: string | null) {
  sessionOrg = next;
  for (const listener of sessionListeners) listener();
}
function subscribeSession(listener: () => void) {
  sessionListeners.add(listener);
  return () => {
    sessionListeners.delete(listener);
  };
}

const listMine = vi.fn();
const switchOrganizationContext = vi.fn();
const sessionUpdate = vi.fn();
const routerPush = vi.fn();

vi.mock("next-auth/react", () => ({
  useSession: () => {
    const org = useSyncExternalStore(subscribeSession, () => sessionOrg);
    return {
      data: { organizationUuid: org },
      status: "authenticated",
      update: sessionUpdate,
    };
  },
}));
vi.mock("next/navigation", () => ({
  useParams: () => ({ slug: "bayi-a" }),
  useRouter: () => ({ push: routerPush }),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));
vi.mock("@/providers/auth-provider", () => ({
  useAuth: () => ({
    bootstrapped: true,
    isAuthenticated: true,
    user: {
      organizations: [
        { uuid: ORG_A, slug: "bayi-a" },
        { uuid: ORG_B, slug: "bayi-b" },
      ],
    },
  }),
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

import {
  beginOrgSwitch,
  endOrgSwitch,
  getOrgSwitchTarget,
} from "@/features/organizations/lib/org-switch";

import { OrganizationSwitcher } from "./organization-switcher";
import { TenantOrganizationContext } from "./tenant-organization-context";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

// Children go positionally (react/no-children-prop); the cast lets
// createElement accept them for the required `children` prop.
const Context = TenantOrganizationContext as ComponentType<{
  slug: string;
  children?: ReactNode;
}>;
const tenantContext = (slug: string) =>
  createElement(Context, { slug }, "child");

async function flush() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

// The switcher renders its items only once `useMyOrganizations` resolves.
// TanStack Query delivers results through a setTimeout(0) scheduled after the
// fetch settles, so a single flush can win that race; poll until it lands.
async function findSwitcherItem(container: HTMLElement, slug: string) {
  const selector = `[data-testid="organization-switcher-item-${slug}"]`;
  for (let i = 0; i < 50; i++) {
    const item = container.querySelector<HTMLButtonElement>(selector);
    if (item) return item;
    await flush();
  }
  throw new Error(`switcher item ${slug} never rendered`);
}

describe("TenantOrganizationContext during an organization switch", () => {
  let container: HTMLDivElement;
  let root: Root;

  const render = (node: ReactNode) => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    act(() => {
      root.render(createElement(QueryClientProvider, { client }, node));
    });
  };

  beforeEach(() => {
    sessionOrg = ORG_A;
    sessionListeners.clear();
    endOrgSwitch(getOrgSwitchTarget() ?? "");
    listMine.mockReset().mockResolvedValue([
      {
        uuid: ORG_A,
        slug: "bayi-a",
        name: "Bayi A",
        role: "owner",
        status: "active",
        type: "dealer",
        brand: { slug: "olex" },
        parent: null,
      },
      {
        uuid: ORG_B,
        slug: "bayi-b",
        name: "Bayi B",
        role: "staff",
        status: "active",
        type: "dealer",
        brand: { slug: "olex" },
        parent: null,
      },
    ]);
    switchOrganizationContext
      .mockReset()
      .mockImplementation(async (slug: string) => {
        setSessionOrg(UUID_BY_SLUG[slug] ?? null);
        return {};
      });
    sessionUpdate.mockReset().mockResolvedValue(null);
    routerPush.mockReset();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    endOrgSwitch(getOrgSwitchTarget() ?? "");
  });

  it("re-scopes the session to its slug when no switch is pending", async () => {
    render(tenantContext("bayi-a"));
    await flush();
    expect(switchOrganizationContext).not.toHaveBeenCalled();

    // Session drifts to another org without a switch: the context fixes it.
    act(() => setSessionOrg(ORG_B));
    await flush();
    expect(switchOrganizationContext.mock.calls).toEqual([["bayi-a"]]);
  });

  it("pauses its sync while a switch to another organization is pending", async () => {
    render(tenantContext("bayi-a"));
    await flush();

    act(() => beginOrgSwitch("bayi-b"));
    act(() => setSessionOrg(ORG_B));
    await flush();
    expect(switchOrganizationContext).not.toHaveBeenCalled();
    expect(container.textContent).toBe("child");
  });

  it("the target organization's context ends the switch", async () => {
    act(() => beginOrgSwitch("bayi-b"));
    setSessionOrg(ORG_B);
    render(tenantContext("bayi-b"));
    await flush();
    expect(getOrgSwitchTarget()).toBeNull();
    expect(switchOrganizationContext).not.toHaveBeenCalled();
    expect(container.textContent).toBe("child");
  });

  it("switcher + mounted old context: every org-context call targets the new org", async () => {
    render(
      createElement(
        Fragment,
        null,
        createElement(OrganizationSwitcher),
        tenantContext("bayi-a"),
      ),
    );

    const item = await findSwitcherItem(container, "bayi-b");
    await act(async () => {
      item.click();
    });
    await flush();
    await flush();

    expect(switchOrganizationContext.mock.calls).toEqual([["bayi-b"]]);
    expect(sessionOrg).toBe(ORG_B);
    expect(routerPush).toHaveBeenCalledWith("/t/bayi-b");
    // Still pending until the target's context mounts (navigation lands).
    expect(getOrgSwitchTarget()).toBe("bayi-b");
  });

  it("a failed switch releases the guard", async () => {
    switchOrganizationContext.mockReset().mockRejectedValue(new Error("x"));
    render(createElement(OrganizationSwitcher));
    const item = await findSwitcherItem(container, "bayi-b");
    await act(async () => {
      item.click();
    });
    await flush();
    expect(getOrgSwitchTarget()).toBeNull();
    expect(routerPush).not.toHaveBeenCalled();
  });
});
