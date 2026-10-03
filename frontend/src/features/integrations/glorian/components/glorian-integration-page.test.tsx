// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  get: vi.fn(),
  save: vi.fn(),
  test: vi.fn(),
  listSyncRuns: vi.fn(),
  getSyncRun: vi.fn(),
  triggerSync: vi.fn(),
  listOutbounds: vi.fn(),
  replayOutbound: vi.fn(),
  reconcile: vi.fn(),
}));
const state = vi.hoisted(() => ({ grants: new Set<string>() }));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
  }),
  useFormatter: () => ({ dateTime: (v: string) => v }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock(
  "@/features/integrations/glorian/services/glorian.service",
  async (orig) => ({
    ...(await orig<object>()),
    glorianService: api,
  }),
);

import { platformNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import type {
  GlorianConnection,
  GlorianOutbound,
  GlorianSyncRun,
} from "@/features/integrations/glorian/services/glorian.service";
import { HUB_LINKS } from "@/features/system-settings/components/system-settings-page";

import { GlorianIntegrationPage } from "./glorian-integration-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

const VIEW = Permission.IntegrationsGlorianView;
const MANAGE = Permission.IntegrationsGlorianManage;
const NOW = "2026-10-03T09:00:00Z";
const OUTBOUND = "0b9c4c1e-0000-4000-8000-0000000000b1";

const connection: GlorianConnection = {
  configured: true,
  uuid: "0b9c4c1e-0000-4000-8000-0000000000aa",
  key: "glorian",
  base_url: "https://hub.example.com",
  active: true,
  api_version: "1",
  default_warehouse_uuid: null,
  api_key_set: true,
  api_key_masked: "********",
  created_at: NOW,
  updated_at: NOW,
};

const reconcileRun: GlorianSyncRun = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000c1",
  kind: "reconcile",
  status: "succeeded",
  started_at: NOW,
  finished_at: NOW,
  watermark: null,
  counts: {
    only_remote: 2,
    only_local: 1,
    status_drift: 0,
    product_drift: 3,
    owner_drift: 4,
    remote: 10,
    local: 9,
    pages: 1,
    skipped: 0,
    details: {
      only_remote: [{ barcode: "GL-0001", remote_id: "r1" }],
      only_local: [],
      status_drift: [],
      product_drift: [],
      owner_drift: [],
    },
  },
  error: null,
};

const held: GlorianOutbound = {
  uuid: OUTBOUND,
  order_uuid: "0b9c4c1e-0000-4000-8000-0000000000d1",
  order_no: "ORD-0001",
  order_status: "approved",
  external_reference: "olex-ORD-0001",
  state: "held",
  held_reason: "missing_customer_link",
  attempts: 1,
  last_error: null,
  created_at: NOW,
  updated_at: NOW,
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  api.get.mockResolvedValue(connection);
  api.listSyncRuns.mockResolvedValue([reconcileRun]);
  api.listOutbounds.mockResolvedValue([held]);
  api.replayOutbound.mockResolvedValue(held);
  api.save.mockResolvedValue(connection);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
  state.grants = new Set();
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
        createElement(GlorianIntegrationPage),
      ),
    );
  });
  await flush();
}

async function type(el: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )!.set!;
  await act(async () => {
    setter.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function click(el: Element | null | undefined) {
  if (!el) throw new Error("element not found");
  await act(async () => {
    (el as HTMLElement).click();
  });
  await flush();
}

const byTestId = <T extends Element = HTMLElement>(id: string) =>
  container.querySelector<T>(`[data-testid='${id}']`);
const input = (name: string) =>
  container.querySelector<HTMLInputElement>(`input[name='${name}']`)!;
const buttonByText = (text: string) =>
  [...container.querySelectorAll("button")].find(
    (b) => b.textContent?.trim() === text,
  );

describe("GlorianIntegrationPage (TEC-274)", () => {
  it("masks the API key: write only, never pre-filled", async () => {
    state.grants = new Set([VIEW, MANAGE]);
    await render();
    const key = input("api_key");
    expect(key.type).toBe("password");
    expect(key.value).toBe("");
    expect(key.placeholder).toBe("********");
    expect(container.textContent).toContain(
      "integrations.glorian.form.api_key_keep_hint",
    );
    expect(input("base_url").value).toBe("https://hub.example.com");
  });

  it("validates the form before saving", async () => {
    state.grants = new Set([VIEW, MANAGE]);
    await render();
    await type(input("base_url"), "ftp://hub");
    await type(input("default_warehouse_uuid"), "not-a-uuid");
    await click(buttonByText("integrations.glorian.form.save"));
    expect(api.save).not.toHaveBeenCalled();
    expect(container.textContent).toContain(
      "integrations.glorian.validation.url",
    );
    expect(container.textContent).toContain(
      "integrations.glorian.validation.uuid",
    );

    await type(input("base_url"), "https://hub2.example.com");
    await type(input("default_warehouse_uuid"), "");
    await click(buttonByText("integrations.glorian.form.save"));
    expect(api.save).toHaveBeenCalledWith({
      base_url: "https://hub2.example.com",
      active: true,
      api_version: "1",
      default_warehouse_uuid: null,
    });
  });

  it("renders the drift report category counts", async () => {
    state.grants = new Set([VIEW]);
    await render();
    const counts = Object.fromEntries(
      [
        "only_remote",
        "only_local",
        "status_drift",
        "product_drift",
        "owner_drift",
      ].map((cat) => [
        cat,
        byTestId(`drift-count-${cat}`)?.querySelector("dd")?.textContent,
      ]),
    );
    expect(counts).toEqual({
      only_remote: "2",
      only_local: "1",
      status_drift: "0",
      product_drift: "3",
      owner_drift: "4",
    });
    expect(container.textContent).toContain("GL-0001");
  });

  it("replays a held outbound", async () => {
    state.grants = new Set([VIEW, MANAGE]);
    await render();
    expect(api.listOutbounds).toHaveBeenCalledWith("held");
    await click(byTestId(`replay-${OUTBOUND}`));
    expect(api.replayOutbound).toHaveBeenCalledTimes(1);
    expect(api.replayOutbound.mock.calls[0][0]).toBe(OUTBOUND);
  });

  it("hides the write actions for view only", async () => {
    state.grants = new Set([VIEW]);
    await render();
    expect(byTestId(`outbound-${OUTBOUND}`)).not.toBeNull();
    expect(byTestId(`replay-${OUTBOUND}`)).toBeNull();
    expect(byTestId("reconcile-start")).toBeNull();
    expect(byTestId("glorian-test")).toBeNull();
    expect(byTestId("sync-pull")).toBeNull();
    expect(buttonByText("integrations.glorian.form.save")).toBeUndefined();
    expect(input("base_url").disabled).toBe(true);
    expect(input("api_key").disabled).toBe(true);
    expect(container.textContent).toContain("integrations.glorian.read_only");
  });

  it("shows only the form for an unconfigured connection", async () => {
    state.grants = new Set([VIEW, MANAGE]);
    api.get.mockResolvedValue({
      ...connection,
      configured: false,
      uuid: null,
      base_url: "",
      active: false,
      api_key_set: false,
      api_key_masked: null,
    });
    await render();
    expect(byTestId("glorian-not-configured")).not.toBeNull();
    expect(api.listSyncRuns).not.toHaveBeenCalled();
    expect(api.listOutbounds).not.toHaveBeenCalled();
    expect(input("api_key").placeholder).toBe(
      "integrations.glorian.form.api_key_placeholder",
    );
  });

  it("does not load without integrations.glorian.view", async () => {
    await render();
    expect(api.get).not.toHaveBeenCalled();
    expect(container.textContent).toContain("integrations.glorian.forbidden");
  });

  it("is linked from the nav and the settings hub behind view", () => {
    const nav = platformNav.groups
      .flatMap((g) => g.items)
      .find((i) => i.id === "glorian-integration");
    expect(nav?.href).toBe(routes.platform.integrations.glorian);
    expect(nav?.permission).toBe(VIEW);
    const hub = HUB_LINKS.find((l) => l.id === "glorian");
    expect(hub?.href).toBe(routes.platform.integrations.glorian);
    expect(hub?.permission).toBe(VIEW);
  });
});
