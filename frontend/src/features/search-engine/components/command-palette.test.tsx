// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { GlobalSearchResponse } from "@/features/search-engine/services/search.service";

const routerPush = vi.fn();
const setOpen = vi.fn();
const fetchGlobalSearch = vi.fn();
const fetchSearchHits = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: routerPush }),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true, canAny: () => true }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ type: "dealer", role: "owner" }),
}));
vi.mock("@/features/modules/hooks/use-features", () => ({
  useEnabledFeatures: () => [],
}));
vi.mock("@/features/search-engine/lib/nav-pages", () => ({
  buildNavPageItems: () => [],
}));
vi.mock("@/features/search-engine/providers/command-palette-provider", () => ({
  useCommandPalette: () => ({ open: true, setOpen }),
}));
vi.mock("@/features/search-engine/services/search.service", () => ({
  fetchSearchSpecs: () => Promise.resolve({ items: [], enabled: true }),
  fetchSearchHits: (...args: unknown[]) => fetchSearchHits(...args),
  fetchGlobalSearch: (...args: unknown[]) => fetchGlobalSearch(...args),
}));

import { CommandPalette } from "./command-palette";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
// cmdk and Radix measure / scroll; jsdom has neither.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;
Element.prototype.scrollIntoView ??= function scrollIntoView() {};

const hit = (spec: string, id: string, title: string, href: string) => ({
  spec,
  id,
  title,
  href,
});

const grouped: GlobalSearchResponse = {
  enabled: true,
  info: null,
  groups: [
    {
      spec: "customers",
      label_key: "search.specs_customers",
      icon: "users",
      items: [hit("customers", "c1", "Ahmet Yılmaz", "/customers/c1")],
    },
    {
      spec: "vehicles",
      label_key: "search.specs_vehicles",
      icon: "car",
      items: [hit("vehicles", "v1", "34 ABC 123", "/vehicles/v1")],
    },
    {
      spec: "stock_units",
      label_key: "search.specs_stock_units",
      icon: "barcode",
      items: [
        hit("stock_units", "u1", "OLX-00000001", "/stock/units/OLX-00000001"),
      ],
    },
  ],
};

let root: Root;
let host: HTMLDivElement;

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
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
        createElement(CommandPalette, {
          variant: "tenant",
          tenantSlug: "bayi-a",
        }),
      ),
    );
  });
  await flush();
}

function input(): HTMLInputElement {
  const el = document.querySelector<HTMLInputElement>("[cmdk-input]");
  if (!el) throw new Error("no palette input");
  return el;
}

async function type(value: string) {
  const el = input();
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )!.set!;
  await act(async () => {
    setter.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await flush();
}

async function key(k: string) {
  await act(async () => {
    input().dispatchEvent(
      new KeyboardEvent("keydown", { key: k, bubbles: true, cancelable: true }),
    );
  });
  await flush();
}

const headings = () =>
  [...document.querySelectorAll("[cmdk-group-heading]")].map(
    (el) => el.textContent,
  );
const items = () => [...document.querySelectorAll<HTMLElement>("[cmdk-item]")];
const selected = () =>
  items().find((el) => el.getAttribute("aria-selected") === "true");

beforeEach(() => {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  routerPush.mockReset();
  fetchGlobalSearch.mockReset();
  fetchSearchHits.mockReset();
  window.localStorage.clear();
});

afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  document.body.innerHTML = "";
});

describe("CommandPalette global search (TEC-213)", () => {
  it("renders the hits grouped by type in server order", async () => {
    fetchGlobalSearch.mockResolvedValue(grouped);
    await render();
    await type("ahmet");
    expect(fetchGlobalSearch).toHaveBeenCalledWith("ahmet", undefined);
    expect(fetchSearchHits).not.toHaveBeenCalled();
    expect(headings()).toEqual([
      "search.specs_customers",
      "search.specs_vehicles",
      "search.specs_stock_units",
    ]);
    expect(items().map((el) => el.textContent)).toEqual([
      "Ahmet Yılmaz",
      "34 ABC 123",
      "OLX-00000001",
    ]);
  });

  it("moves the vehicles group up for a plate and stock units for a barcode", async () => {
    fetchGlobalSearch.mockResolvedValue(grouped);
    await render();
    await type("34ABC123");
    expect(headings()[0]).toBe("search.specs_vehicles");
    await type("OLX-00000001");
    expect(headings()[0]).toBe("search.specs_stock_units");
  });

  it("navigates with the keyboard and opens the tenant detail page", async () => {
    fetchGlobalSearch.mockResolvedValue(grouped);
    await render();
    await type("ahmet");
    expect(selected()?.textContent).toBe("Ahmet Yılmaz");
    await key("ArrowDown");
    expect(selected()?.textContent).toBe("34 ABC 123");
    await key("Enter");
    expect(routerPush).toHaveBeenCalledWith("/t/bayi-a/vehicles/v1");
    expect(setOpen).toHaveBeenCalledWith(false);
  });

  it("opens the stock page filtered on the barcode on click", async () => {
    fetchGlobalSearch.mockResolvedValue(grouped);
    await render();
    await type("olx");
    const unit = items().find((el) => el.textContent === "OLX-00000001");
    await act(async () => unit!.click());
    expect(routerPush).toHaveBeenCalledWith(
      "/t/bayi-a/stock?barcode=OLX-00000001",
    );
  });

  it("shows the empty state and the search-disabled info", async () => {
    fetchGlobalSearch.mockResolvedValue({
      enabled: false,
      info: "search_disabled",
      groups: [
        {
          spec: "customers",
          label_key: "search.specs_customers",
          items: [],
        },
      ],
    } satisfies GlobalSearchResponse);
    await render();
    await type("nothing");
    expect(items()).toHaveLength(0);
    expect(document.body.textContent).toContain("search.empty");
    expect(
      document.querySelector('[data-testid="search-info"]')?.textContent,
    ).toBe("search.global_disabled");
  });
});
