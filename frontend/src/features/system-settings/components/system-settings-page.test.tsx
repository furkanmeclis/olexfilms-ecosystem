// @vitest-environment jsdom
import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";

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

import { platformNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import type { SystemSetting } from "@/features/system-settings/services/system-settings.service";
import { ApiError } from "@/lib/api/errors";

import { SystemSettingsPage } from "./system-settings-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

// Radix Switch measures its thumb; jsdom has no ResizeObserver.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
  state.grants = new Set();
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
    description: "Read-only grace period",
    min: 0,
    max: 365,
    value: 0,
    is_default: true,
    schema_version: 1,
    ...patch,
  };
}

const catalog: SystemSetting[] = [
  setting({}),
  setting({
    key: "forecast_min_days",
    group: "forecast",
    default: 30,
    value: 45,
    min: 1,
    max: 3650,
    is_default: false,
    updated_at: "2026-10-01T10:00:00Z",
  }),
  setting({
    key: "photo_standard_enabled",
    group: "services",
    kind: "bool",
    default: false,
    value: false,
    min: undefined,
    max: undefined,
  }),
  setting({
    key: "smtp.host",
    group: "smtp",
    kind: "string",
    default: "",
    value: "mail.example.com",
    min: undefined,
    max: undefined,
    max_len: 253,
    is_default: false,
  }),
  setting({
    key: "smtp.password",
    group: "smtp",
    kind: "string",
    default: "",
    value: "********",
    min: undefined,
    max: undefined,
    max_len: 255,
    secret: true,
    is_default: false,
  }),
];

const rw = [Permission.PlatformSettingsRead, Permission.PlatformSettingsWrite];

function row(key: string) {
  const el = container.querySelector<HTMLElement>(
    `[data-testid="setting-${key}"]`,
  );
  if (!el) throw new Error(`row ${key} not rendered`);
  return el;
}

function input(key: string) {
  return row(key).querySelector<HTMLInputElement>("input")!;
}

function button(key: string, label: string) {
  return [...row(key).querySelectorAll("button")].find((b) =>
    b.textContent?.includes(label),
  ) as HTMLButtonElement;
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

async function click(el: HTMLElement) {
  await act(async () => {
    el.click();
  });
  await flush();
}

describe("SystemSettingsPage (TEC-222)", () => {
  beforeEach(() => {
    api.list.mockResolvedValue({ items: catalog });
  });

  it("renders the catalog grouped, with grace default 0", async () => {
    state.grants = new Set(rw);
    await render();
    expect(api.list).toHaveBeenCalledTimes(1);
    const groups = [
      ...container.querySelectorAll<HTMLElement>("[data-testid^='group-']"),
    ].map((g) => g.dataset.testid);
    expect(groups).toEqual([
      "group-contracts",
      "group-forecast",
      "group-services",
      "group-smtp",
    ]);
    expect(input("contract_grace_days").value).toBe("0");
    expect(input("forecast_min_days").value).toBe("45");
    expect(row("contract_grace_days").textContent).toContain(
      "settings.system.default_badge",
    );
    expect(row("forecast_min_days").textContent).toContain(
      "settings.system.custom_badge",
    );
    expect(
      row("photo_standard_enabled").querySelector("[role='switch']"),
    ).not.toBeNull();
  });

  it("validates the integer range before saving", async () => {
    state.grants = new Set(rw);
    await render();
    const field = input("contract_grace_days");
    await type(field, "400");
    expect(row("contract_grace_days").textContent).toContain(
      'settings.system.validation.max {"max":365}',
    );
    expect(button("contract_grace_days", "settings.system.save").disabled).toBe(
      true,
    );
    await type(field, "-1");
    expect(row("contract_grace_days").textContent).toContain(
      'settings.system.validation.min {"min":0}',
    );
    await type(field, "1.5");
    expect(row("contract_grace_days").textContent).toContain(
      "settings.system.validation.integer",
    );
    expect(api.put).not.toHaveBeenCalled();
  });

  it("saves with PUT and resets with DELETE", async () => {
    state.grants = new Set(rw);
    api.put.mockResolvedValue(
      setting({
        value: 7,
        is_default: false,
        updated_at: "2026-10-03T00:00:00Z",
      }),
    );
    api.reset.mockResolvedValue(
      setting({
        key: "forecast_min_days",
        group: "forecast",
        default: 30,
        value: 30,
        min: 1,
        max: 3650,
      }),
    );
    await render();

    await type(input("contract_grace_days"), "7");
    await click(button("contract_grace_days", "settings.system.save"));
    expect(api.put).toHaveBeenCalledWith("contract_grace_days", 7);
    expect(input("contract_grace_days").value).toBe("7");

    await click(button("forecast_min_days", "settings.system.reset"));
    expect(api.reset).toHaveBeenCalledWith("forecast_min_days");
    expect(input("forecast_min_days").value).toBe("30");
  });

  it("saves a bool toggle", async () => {
    state.grants = new Set(rw);
    api.put.mockResolvedValue(
      setting({
        key: "photo_standard_enabled",
        group: "services",
        kind: "bool",
        default: false,
        value: true,
        is_default: false,
      }),
    );
    await render();
    const sw = row("photo_standard_enabled").querySelector<HTMLElement>(
      "[role='switch']",
    )!;
    await click(sw);
    await click(button("photo_standard_enabled", "settings.system.save"));
    expect(api.put).toHaveBeenCalledWith("photo_standard_enabled", true);
  });

  it("toggles contracts.intake_required in the contracts group (TEC-291)", async () => {
    state.grants = new Set(rw);
    const intake = setting({
      key: "contracts.intake_required",
      group: "contracts",
      kind: "bool",
      default: false,
      value: false,
      min: undefined,
      max: undefined,
    });
    api.list.mockResolvedValue({ items: [...catalog, intake] });
    api.put.mockResolvedValue({ ...intake, value: true, is_default: false });
    await render();
    const group = container.querySelector("[data-testid='group-contracts']");
    expect(
      group?.querySelector("[data-testid='setting-contracts.intake_required']"),
    ).not.toBeNull();
    const sw = row("contracts.intake_required").querySelector<HTMLElement>(
      "[role='switch']",
    )!;
    await click(sw);
    await click(button("contracts.intake_required", "settings.system.save"));
    expect(api.put).toHaveBeenCalledWith("contracts.intake_required", true);

    // Every locale names the key (the page falls back to the raw key).
    const dir = path.join(process.cwd(), "src/locales");
    const locales = readdirSync(dir).filter((l) =>
      existsSync(path.join(dir, l, "settings.json")),
    );
    expect(locales).toHaveLength(13);
    for (const locale of locales) {
      const messages = JSON.parse(
        readFileSync(path.join(dir, locale, "settings.json"), "utf8"),
      ) as Record<string, string>;
      expect(
        messages["system.keys.contracts_intake_required"],
        locale,
      ).toBeTruthy();
      expect(
        messages["system.keys.contracts_intake_required_hint"],
        locale,
      ).toBeTruthy();
    }
  });

  it("masks a secret value and keeps it unless retyped", async () => {
    state.grants = new Set(rw);
    api.put.mockResolvedValue(catalog[4]);
    await render();
    const field = input("smtp.password");
    expect(field.type).toBe("password");
    expect(field.value).toBe("");
    expect(field.placeholder).toBe("********");
    expect(container.textContent).not.toContain("secret-value");
    expect(button("smtp.password", "settings.system.save").disabled).toBe(true);

    await type(field, "new-secret");
    await click(button("smtp.password", "settings.system.save"));
    expect(api.put).toHaveBeenCalledWith("smtp.password", "new-secret");
  });

  it("shows a 400 VALIDATION_ERROR on the field", async () => {
    state.grants = new Set(rw);
    api.put.mockRejectedValue(
      new ApiError({
        status: 400,
        code: "VALIDATION_ERROR",
        message: "Validation failed",
        details: [
          { field: "value", message: "must be at most 253 characters" },
        ],
      }),
    );
    await render();
    await type(input("smtp.host"), "smtp.other.example");
    await click(button("smtp.host", "settings.system.save"));
    const alert = row("smtp.host").querySelector("[role='alert']");
    expect(alert?.textContent).toBe("must be at most 253 characters");
    expect(input("smtp.host").getAttribute("aria-invalid")).toBe("true");
  });

  it("is read-only without platform.settings.write", async () => {
    state.grants = new Set([Permission.PlatformSettingsRead]);
    await render();
    expect(container.textContent).toContain("settings.system.read_only");
    expect(input("contract_grace_days").disabled).toBe(true);
    expect(
      button("contract_grace_days", "settings.system.save"),
    ).toBeUndefined();
  });

  it("shows hub links only for granted topic pages", async () => {
    state.grants = new Set([
      Permission.PlatformSettingsRead,
      Permission.PlatformLegalTextsWrite,
    ]);
    await render();
    const links = [
      ...container.querySelectorAll<HTMLAnchorElement>(
        "[data-testid^='hub-link-']",
      ),
    ];
    const hrefs = links.map((a) => a.getAttribute("href"));
    expect(hrefs).toContain(routes.platform.legalTexts);
    expect(hrefs).toContain(routes.platform.plateFormats.root);
    expect(hrefs).not.toContain(routes.platform.integrations.whatsapp);
    expect(hrefs).not.toContain(routes.platform.exchangeRates.root);
  });

  it("does not load settings without platform.settings.read", async () => {
    await render();
    expect(api.list).not.toHaveBeenCalled();
    expect(container.querySelector("[data-testid^='group-']")).toBeNull();
  });
});

describe("platform nav: system settings (TEC-222)", () => {
  function visibleIds(granted: string[]) {
    const access = {
      can: (p: string | string[]) =>
        (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
      canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
    };
    return platformNav.groups.flatMap((group) =>
      visibleNavItems(group, access).map((item) => item.id),
    );
  }

  it("links to the hub page behind platform.settings.read", () => {
    const item = platformNav.groups
      .flatMap((g) => g.items)
      .find((i) => i.id === "system-settings");
    expect(item?.href).toBe(routes.platform.systemSettings.root);
    expect(item?.permission).toBe(Permission.PlatformSettingsRead);
  });

  it("is hidden without the permission", () => {
    expect(visibleIds([])).not.toContain("system-settings");
    expect(visibleIds([Permission.PlatformSettingsRead])).toContain(
      "system-settings",
    );
  });
});
