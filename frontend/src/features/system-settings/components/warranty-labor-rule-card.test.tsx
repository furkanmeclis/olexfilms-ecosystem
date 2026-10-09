// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  put: vi.fn(),
  reset: vi.fn(),
}));
const state = vi.hoisted(() => ({ grants: new Set<string>() }));

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
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock(
  "@/features/system-settings/services/system-settings.service",
  async (orig) => ({
    ...(await orig<object>()),
    systemSettingsService: api,
  }),
);

import { Permission } from "@/config/permissions";
import type { SystemSetting } from "@/features/system-settings/services/system-settings.service";

import { chooseValue, installRadixPolyfills } from "@/test/form-controls";
import { SystemSettingsPage } from "./system-settings-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
installRadixPolyfills();

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  state.grants = new Set([
    Permission.PlatformSettingsRead,
    Permission.PlatformSettingsWrite,
  ]);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
});

async function flush() {
  for (let i = 0; i < 5; i++) {
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
        createElement(SystemSettingsPage),
      ),
    );
  });
  await flush();
}

function setting(patch: Partial<SystemSetting>): SystemSetting {
  return {
    key: "contract_grace_days",
    group: "contracts",
    kind: "int",
    default: 0,
    description: "Grace",
    min: 0,
    max: 365,
    value: 0,
    is_default: true,
    schema_version: 1,
    ...patch,
  };
}

const laborCatalog: SystemSetting[] = [
  setting({}),
  setting({
    key: "warranty_claims.labor_rule",
    group: "general",
    kind: "string",
    default: "dealer",
    value: "dealer",
    min: undefined,
    max: undefined,
  }),
  setting({
    key: "warranty_claims.labor_amount",
    group: "general",
    default: 0,
    value: 0,
    max: undefined,
  }),
  setting({
    key: "warranty_claims.labor_share_percent",
    group: "general",
    default: 50,
    value: 50,
    max: 100,
  }),
];

const $ = (sel: string) => container.querySelector(sel);

async function select(el: Element | null, value: string) {
  await chooseValue(el, value);
  await flush();
}

async function type(el: Element | null, value: string) {
  if (!(el instanceof HTMLInputElement)) throw new Error("input not found");
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  await act(async () => {
    setter?.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await flush();
}

async function click(el: Element | null) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
  await flush();
}

describe("WarrantyLaborRuleCard", () => {
  it("is hidden while the catalog has no labor rule key", async () => {
    api.list.mockResolvedValue({ items: [setting({})] });
    await render();
    expect($("[data-testid=warranty-labor-rule]")).toBeNull();
    expect($('[data-testid="setting-contract_grace_days"]')).not.toBeNull();
  });

  it("shows the percentage only for shared and rejects values outside 0–100", async () => {
    api.list.mockResolvedValue({ items: laborCatalog });
    await render();

    expect($("[data-testid=warranty-labor-rule]")).not.toBeNull();
    // The labor keys are not duplicated as generic rows.
    expect($('[data-testid="setting-warranty_claims.labor_rule"]')).toBeNull();
    expect($("[data-testid=labor-percent]")).toBeNull();

    await select($("[data-testid=labor-rule-select]"), "shared");
    expect($("[data-testid=labor-percent]")).not.toBeNull();

    const save = () => $("[data-testid=labor-save]") as HTMLButtonElement;
    await type($("[data-testid=labor-percent]"), "101");
    expect($("[data-testid=labor-percent-error]")?.textContent).toBe(
      "settings.system.labor.errors.percent_range",
    );
    expect(save().disabled).toBe(true);

    await type($("[data-testid=labor-percent]"), "-1");
    expect($("[data-testid=labor-percent-error]")?.textContent).toBe(
      "settings.system.labor.errors.percent_range",
    );

    await type($("[data-testid=labor-percent]"), "40");
    expect($("[data-testid=labor-percent-error]")).toBeNull();
    expect(save().disabled).toBe(false);

    api.put.mockImplementation((key: string, value: unknown) =>
      Promise.resolve({ ...setting({ key }), value }),
    );
    await click(save());
    expect(api.put.mock.calls).toEqual([
      ["warranty_claims.labor_rule", "shared"],
      ["warranty_claims.labor_share_percent", 40],
    ]);

    await select($("[data-testid=labor-rule-select]"), "center");
    expect($("[data-testid=labor-percent]")).toBeNull();
  });
});
