// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const getPrefs = vi.fn();
const registerWebPush = vi.fn();

vi.mock("@/services/notification-preferences.service", () => ({
  notificationPreferencesService: { get: () => getPrefs() },
}));

vi.mock("@/lib/web-push/register", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/lib/web-push/register")>();
  return { ...actual, registerWebPush: () => registerWebPush() };
});

import { hasBrowserPushSupport } from "@/lib/web-push/register";

import { useWebPushSync } from "./use-web-push-sync";

function Probe() {
  useWebPushSync();
  return null;
}

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

describe("useWebPushSync", () => {
  let container: HTMLDivElement;

  beforeEach(() => {
    getPrefs.mockReset();
    registerWebPush.mockReset();
    container = document.createElement("div");
    document.body.appendChild(container);
  });

  afterEach(() => {
    container.remove();
    vi.unstubAllGlobals();
  });

  it("does not throw when Notification is undefined (iOS Safari tab)", () => {
    vi.stubGlobal("Notification", undefined);
    expect(typeof Notification).toBe("undefined");
    expect(hasBrowserPushSupport()).toBe(false);

    const root = createRoot(container);
    expect(() => {
      act(() => {
        root.render(createElement(Probe));
      });
    }).not.toThrow();
    act(() => root.unmount());

    expect(getPrefs).not.toHaveBeenCalled();
    expect(registerWebPush).not.toHaveBeenCalled();
  });
});
