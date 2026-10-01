// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const manager = vi.hoisted(() => ({
  connect: vi.fn(async () => {}),
  reconnectAfterAuthRefresh: vi.fn(async () => {}),
  disconnect: vi.fn(),
  subscribe: vi.fn(async () => () => {}),
  unsubscribe: vi.fn(),
  onEvent: vi.fn(() => () => {}),
  getStatus: vi.fn(() => "idle"),
  subscribeStatus: vi.fn(() => () => {}),
  subscribeOnline: vi.fn(() => () => {}),
  subscribeReconnect: vi.fn(() => () => {}),
}));

const sessionState = vi.hoisted(() => ({
  organizationUuid: null as string | null,
}));

const user = {
  uuid: "11111111-1111-4111-8111-111111111111",
  roles: ["dealer_owner"],
  realtimeUserChannel: null,
};

vi.mock("@/lib/realtime", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/realtime")>();
  return { ...actual, realtimeManager: manager, isRealtimeDebugEnabled: false };
});

vi.mock("@/config/realtime", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/config/realtime")>();
  return {
    realtimeConfig: { ...actual.realtimeConfig, enabled: true },
  };
});

vi.mock("next-auth/react", () => ({
  useSession: () => ({
    data: { organizationUuid: sessionState.organizationUuid },
    status: "authenticated",
  }),
}));

vi.mock("@/providers/auth-provider", () => ({
  useAuth: () => ({ isAuthenticated: true, bootstrapped: true, user }),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key }),
}));

vi.mock("@/providers/toast-provider", () => ({
  appToast: {
    info: vi.fn(),
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
  },
}));

import { RealtimeProvider } from "./realtime-provider";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

describe("RealtimeProvider organization context (TEC-142)", () => {
  let container: HTMLDivElement;
  let root: Root;

  async function render() {
    await act(async () => {
      root.render(createElement(RealtimeProvider, null, null));
    });
  }

  beforeEach(() => {
    Object.values(manager).forEach((fn) => fn.mockClear());
    sessionState.organizationUuid = null;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it("connects before the organization context is set and re-issues the token once it is", async () => {
    await render();
    expect(manager.connect).toHaveBeenCalledTimes(1);
    expect(manager.reconnectAfterAuthRefresh).not.toHaveBeenCalled();
    expect(manager.subscribe).toHaveBeenCalledWith(
      `user:${user.uuid}`,
      expect.any(Function),
    );

    sessionState.organizationUuid = "22222222-2222-4222-8222-222222222222";
    await render();
    expect(manager.reconnectAfterAuthRefresh).toHaveBeenCalledTimes(1);
    expect(manager.subscribe).toHaveBeenCalledTimes(2);

    // Switching organization re-issues it again.
    sessionState.organizationUuid = "33333333-3333-4333-8333-333333333333";
    await render();
    expect(manager.reconnectAfterAuthRefresh).toHaveBeenCalledTimes(2);
  });

  it("does not re-issue the token when the organization is unchanged", async () => {
    sessionState.organizationUuid = "22222222-2222-4222-8222-222222222222";
    await render();
    await render();
    expect(manager.connect).toHaveBeenCalledTimes(1);
    expect(manager.reconnectAfterAuthRefresh).not.toHaveBeenCalled();
  });
});
