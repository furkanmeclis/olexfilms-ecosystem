// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  enabled: true,
  type: "dealer" as string,
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/features/modules/hooks/use-features", () => ({
  useFeature: (_slug: string, key: string) => ({
    enabled: key === "mcp" && state.enabled,
    isLoading: false,
    isError: false,
  }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ slug: "acme", type: state.type }),
}));

import { McpEndpointsCard } from "./mcp-endpoints-card";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  state.enabled = true;
  state.type = "dealer";
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function render() {
  await act(async () => {
    root.render(createElement(McpEndpointsCard, { slug: "acme" }));
  });
}

const urls = () => [...container.querySelectorAll("input")].map((i) => i.value);

describe("McpEndpointsCard", () => {
  it("is hidden while the mcp module is off", async () => {
    state.enabled = false;
    await render();
    expect(container.innerHTML).toBe("");
  });

  it("lists the dealer endpoint only for dealer organizations and copies", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    await render();
    const origin = window.location.origin;
    expect(urls()).toEqual([
      `${origin}/mcp/dealer`,
      `${origin}/mcp/user`,
      `${origin}/mcp/customer`,
    ]);
    await act(async () =>
      container
        .querySelector<HTMLButtonElement>('[data-testid="mcp-copy-user"]')
        ?.click(),
    );
    expect(writeText).toHaveBeenCalledWith(`${origin}/mcp/user`);

    act(() => root.unmount());
    root = createRoot(container);
    state.type = "center";
    await render();
    expect(urls()).toEqual([`${origin}/mcp/user`, `${origin}/mcp/customer`]);
  });
});
