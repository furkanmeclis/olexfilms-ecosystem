// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  pathname: "/t/acme/tasks",
  enabled: true,
  perms: new Set<string>(["ai.use"]),
}));

vi.mock("next/navigation", () => ({ usePathname: () => state.pathname }));
vi.mock("@/features/modules/hooks/use-features", () => ({
  useFeature: () => ({
    enabled: state.enabled,
    isLoading: false,
    isError: false,
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.perms.has(p) }),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (k: string) => k, locale: "tr", dir: "ltr" }),
}));

import { tenantNav } from "@/config/nav";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

import { AssistantHeaderButton, tenantSlugOf } from "./assistant-header-button";

function navIds(features: string[], perms: string[]) {
  const set = new Set(perms);
  const access = {
    can: (p: string | string[]) =>
      Array.isArray(p) ? p.every((x) => set.has(x)) : set.has(p),
    canAny: (list: string[]) => list.some((x) => set.has(x)),
    org: { type: "dealer", role: "owner", features },
  };
  return tenantNav("acme").groups.flatMap((g) =>
    visibleNavItems(g, access).map((i) => i.id),
  );
}

describe("assistant menu entry", () => {
  it("is hidden while the ai_assistant module is off", () => {
    expect(navIds([], ["ai.use"])).not.toContain("assistant");
  });

  it("needs ai.use", () => {
    expect(navIds(["ai_assistant"], [])).not.toContain("assistant");
  });

  it("links the full page when the module is on and ai.use is granted", () => {
    const group = tenantNav("acme").groups.find((g) =>
      g.items.some((i) => i.id === "assistant"),
    );
    expect(group?.items.find((i) => i.id === "assistant")?.href).toBe(
      "/t/acme/assistant",
    );
    expect(navIds(["ai_assistant"], ["ai.use"])).toContain("assistant");
  });
});

describe("AssistantHeaderButton", () => {
  let root: Root;
  let host: HTMLDivElement;

  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    state.pathname = "/t/acme/tasks";
    state.enabled = true;
    state.perms = new Set(["ai.use"]);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
  });

  async function render() {
    host = document.createElement("div");
    document.body.appendChild(host);
    root = createRoot(host);
    await act(async () => {
      root.render(createElement(AssistantHeaderButton));
    });
    return host.querySelector('[data-testid="ai-header-button"]');
  }

  it("is shown for a tenant route with the module on and ai.use", async () => {
    expect(await render()).not.toBeNull();
  });

  it("is absent when the organization's module is off", async () => {
    state.enabled = false;
    expect(await render()).toBeNull();
  });

  it("is absent without ai.use and outside tenant routes", async () => {
    state.perms = new Set();
    expect(await render()).toBeNull();
    act(() => root.unmount());
    host.remove();
    state.perms = new Set(["ai.use"]);
    state.pathname = "/platform/users";
    expect(await render()).toBeNull();
  });

  it("reads the tenant slug from the path", () => {
    expect(tenantSlugOf("/t/acme%20co/assistant")).toBe("acme co");
    expect(tenantSlugOf("/portal")).toBeNull();
  });
});
