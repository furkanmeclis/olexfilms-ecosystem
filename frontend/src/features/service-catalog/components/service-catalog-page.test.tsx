// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { PlatformModule } from "@/features/service-catalog/services/service-catalog.service";

const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  items: [] as unknown[],
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    format: {
      currency: (value: number | null, currency: string) =>
        value === null ? "—" : `${value.toFixed(2)} ${currency}`,
      dateTime: () => "date",
    },
  }),
}));

vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => state.grants.has(x)),
    canAny: (ps: string[]) => ps.some((x) => state.grants.has(x)),
    canScope: (p: string) => state.grants.has(p),
  }),
}));

vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@/features/modules/services/modules.service", () => ({
  modulesService: {
    platformList: () => Promise.resolve({ items: [] }),
  },
}));

vi.mock(
  "@/features/service-catalog/services/service-catalog.service",
  async (orig) => {
    const actual =
      await orig<
        typeof import("@/features/service-catalog/services/service-catalog.service")
      >();
    return {
      ...actual,
      serviceCatalogService: {
        ...actual.serviceCatalogService,
        listVisible: () => Promise.resolve({ items: state.items }),
        listPlatform: () => Promise.resolve({ items: state.items }),
        listContractTemplates: () => Promise.resolve({ items: [] }),
        listDistributors: () => Promise.resolve([]),
      },
    };
  },
);

import {
  deleteOverrideRow,
  editableModules,
  overrideRowFromLookup,
  ServiceCatalogPage,
  showsModulePicker,
} from "@/features/service-catalog/components/service-catalog-page";
import { serviceCatalogService } from "@/features/service-catalog/services/service-catalog.service";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

class TestResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

(
  globalThis as unknown as { ResizeObserver: typeof TestResizeObserver }
).ResizeObserver = TestResizeObserver;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  state.grants = new Set();
  state.items = [];
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

async function render(node: ReturnType<typeof createElement>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 10));
  });
}

const baseForm = {
  name: "Starter",
  description: "",
  category: "training",
  default_price: "100.00",
  currency: "EUR",
  recurrence: "monthly",
  cancellation_fee: "0",
  contract_template_id: "",
  is_active: true,
  modules: [],
} as const;

const moduleRows: PlatformModule[] = [
  {
    key: "organizations",
    level: "core",
    enabled: true,
    paid: false,
    default_enabled: true,
  },
  {
    key: "service_catalog",
    level: "standard",
    enabled: true,
    paid: true,
    default_enabled: false,
  },
  {
    key: "ai_assistant",
    level: "addon",
    enabled: true,
    paid: true,
    default_enabled: false,
  },
];

describe("ServiceCatalogPage module bundles", () => {
  it("does not show module selection outside module_bundle", () => {
    expect(showsModulePicker(baseForm.category)).toBe(false);
    expect(showsModulePicker("training")).toBe(false);
  });

  it("filters core modules out of the module selector", () => {
    expect(showsModulePicker("module_bundle")).toBe(true);
    expect(editableModules(moduleRows).map((m) => m.key)).toEqual([
      "service_catalog",
      "ai_assistant",
    ]);
  });
});

describe("service catalog overrides", () => {
  it("sends the override PUT body to the distributor endpoint", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            organization_uuid: "org-1",
            price: "90.00",
            currency: "EUR",
          },
          meta: {},
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await serviceCatalogService.putOverride("svc-1", "org-1", {
      price: "90.00",
      currency: "EUR",
    });

    const [, init] = fetchMock.mock.calls[0];
    expect(fetchMock.mock.calls[0][0]).toContain(
      "/v1/platform/service-catalog/svc-1/overrides/org-1",
    );
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({
      price: "90.00",
      currency: "EUR",
    });
  });

  it("drops a deleted override from the local list", () => {
    const rows = [
      { org: { uuid: "org-1", name: "A" }, price: "90", currency: "EUR" },
      { org: { uuid: "org-2", name: "B" }, price: "95", currency: "EUR" },
    ];
    expect(deleteOverrideRow(rows, "org-1")).toEqual([rows[1]]);
  });

  it("maps an existing override lookup into the editable row", () => {
    expect(
      overrideRowFromLookup(
        { uuid: "org-1", name: "Distribütör A" },
        { organization_uuid: "org-1", price: "88.50", currency: "EUR" },
      ),
    ).toEqual({
      org: { uuid: "org-1", name: "Distribütör A" },
      price: "88.50",
      currency: "EUR",
    });
  });
});

describe("dealer service catalog view", () => {
  it("does not render edit buttons for read-only dealer view", async () => {
    state.grants = new Set(["service_catalog.read"]);
    state.items = [
      {
        uuid: "svc-1",
        name: "Starter",
        description: "",
        category: "training",
        currency: "EUR",
        recurrence: "monthly",
        is_active: true,
        effective_price: {
          amount: "100.00",
          currency: "EUR",
          source: "default",
        },
        created_at: "2026-10-01T00:00:00Z",
        updated_at: "2026-10-01T00:00:00Z",
      },
    ];
    await render(
      createElement(ServiceCatalogPage, { mode: "tenant", slug: "acme" }),
    );
    const labels = Array.from(document.body.querySelectorAll("button")).map(
      (el) => el.getAttribute("aria-label") ?? el.textContent?.trim() ?? "",
    );
    expect(document.body.textContent).toContain("Starter");
    expect(labels).not.toContain("common.edit");
    expect(labels).not.toContain("common.delete");
    expect(labels).not.toContain("catalog.services.create");
  });
});
