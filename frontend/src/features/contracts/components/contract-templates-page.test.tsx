// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { platformNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import type { ContractTemplate } from "@/features/contracts/services/contract-templates.service";

const state = vi.hoisted(() => ({ grants: new Set<string>() }));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/platform/contract-templates",
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: { date: () => "date" },
  }),
}));

vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => state.grants.has(x)),
    canAny: (ps: string[]) => ps.some((x) => state.grants.has(x)),
    hasPermission: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => state.grants.has(x)),
  }),
}));

vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));

import { ContractTemplatesPage } from "@/features/contracts/components/contract-templates-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

function template(
  uuid: string,
  name: string,
  isDefault: boolean,
): ContractTemplate {
  return {
    id: 1,
    uuid,
    name,
    kind: "vehicle_intake",
    is_default: isDefault,
    otp_required: true,
    signature_required: true,
    is_active: true,
    locales: [],
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-01T00:00:00Z",
  };
}

let container: HTMLDivElement;
let root: Root;
let server: ContractTemplate[];
let calls: { url: string; method: string }[];

function json(data: unknown) {
  return new Response(JSON.stringify({ success: true, data, meta: {} }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  state.grants = new Set([Permission.ContractsTemplatesManage]);
  server = [
    template("tpl-a", "Intake A", true),
    template("tpl-b", "Intake B", false),
  ];
  calls = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    calls.push({ url, method });
    const m = /\/v1\/platform\/contract-templates\/([^/]+)\/default$/.exec(url);
    if (m && method === "POST") {
      server = server.map((t) => ({ ...t, is_default: t.uuid === m[1] }));
      return json(server.find((t) => t.uuid === m[1]));
    }
    if (url.endsWith("/v1/platform/contract-templates") && method === "GET") {
      return json({ items: server });
    }
    return new Response("{}", { status: 404 });
  });
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

async function flush() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 10));
  });
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
        createElement(ContractTemplatesPage),
      ),
    );
  });
  await flush();
}

function row(uuid: string) {
  return document.querySelector(`[data-testid="contract-template-${uuid}"]`)!;
}

function buttonByText(scope: ParentNode, text: string) {
  return Array.from(scope.querySelectorAll("button")).find(
    (b) => b.textContent?.trim() === text,
  ) as HTMLButtonElement | undefined;
}

describe("ContractTemplatesPage (TEC-290)", () => {
  it("make default posts to the default endpoint and moves the badge", async () => {
    await render();
    expect(
      row("tpl-a").querySelector('[data-testid="default-badge"]'),
    ).not.toBeNull();
    expect(row("tpl-b").querySelector('[data-testid="default-badge"]')).toBe(
      null,
    );

    const action = buttonByText(
      row("tpl-b"),
      "contract_templates.actions.make_default",
    );
    expect(action).toBeDefined();
    await act(async () => action!.click());
    await flush();

    // confirmation dialog: nothing is sent before confirming
    expect(calls.some((c) => c.method === "POST")).toBe(false);
    const dialog = document.querySelector('[role="dialog"]')!;
    const confirm = buttonByText(
      dialog,
      "contract_templates.actions.make_default",
    );
    await act(async () => confirm!.click());
    await flush();

    const post = calls.find((c) => c.method === "POST");
    expect(post?.url).toBe("/api/v1/platform/contract-templates/tpl-b/default");
    expect(
      row("tpl-b").querySelector('[data-testid="default-badge"]'),
    ).not.toBeNull();
    expect(row("tpl-a").querySelector('[data-testid="default-badge"]')).toBe(
      null,
    );
  });

  it("does not render the list without contracts.templates.manage", async () => {
    state.grants = new Set();
    await render();
    expect(document.querySelector('[data-testid^="contract-template-"]')).toBe(
      null,
    );
    expect(document.body.textContent).toContain("common.error_forbidden");
  });
});

describe("platform nav: contract templates (TEC-290)", () => {
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

  it("links to the contract template screen", () => {
    const item = platformNav.groups
      .flatMap((g) => g.items)
      .find((i) => i.id === "contract-templates");
    expect(item?.href).toBe(routes.platform.contractTemplates.root);
    expect(item?.permission).toBe(Permission.ContractsTemplatesManage);
  });

  it("is hidden without contracts.templates.manage", () => {
    expect(visibleIds([])).not.toContain("contract-templates");
    expect(visibleIds([Permission.ServiceCatalogManage])).not.toContain(
      "contract-templates",
    );
    expect(visibleIds([Permission.ContractsTemplatesManage])).toContain(
      "contract-templates",
    );
  });
});
