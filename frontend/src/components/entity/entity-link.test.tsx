// @vitest-environment jsdom
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { reportLinkHref } from "@/components/analytics/adapters";
import { entityHref, ENTITY_KINDS } from "@/config/routes";
import { resolveSearchHitHref } from "@/features/search-engine/lib/hit-href";

const granted = new Set<string>();

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key }),
}));

vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (slug: string) => granted.has(slug) }),
}));

vi.mock("@/features/organizations/providers/tenant-provider", () => ({
  useOptionalTenant: () => ({ slug: "olex" }),
}));

vi.mock("next/link", async () => {
  const { createElement: h } = await import("react");
  return {
    default: ({ href, children, ...rest }: Record<string, unknown>) =>
      h("a", { href, ...rest }, children as ReactNode),
  };
});

import { EntityLink } from "./entity-link";
import { ENTITY_READ_PERMISSIONS } from "./entity-registry";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  granted.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

function render(node: ReactNode) {
  act(() => root.render(node));
}

describe("entityHref (TEC-1217)", () => {
  it("maps kinds with a page and returns null for the rest", () => {
    expect(entityHref("service", "s1", "olex")).toBe("/t/olex/services/s1");
    expect(entityHref("product", "p1", "olex")).toBe(
      "/t/olex/catalog/products/p1",
    );
    expect(entityHref("stock_unit", "OLX-1", "olex")).toBe(
      "/t/olex/stock/units/OLX-1",
    );
    expect(entityHref("cari", "c1", "olex")).toBe("/t/olex/accounting/cari/c1");
    expect(entityHref("announcement", "a1", "olex")).toBe(
      "/t/olex/announcements/a1",
    );
    expect(entityHref("user", "u1", "olex")).toBe("/t/olex/users/u1");
    expect(entityHref("service", "", "olex")).toBeNull();
    expect(entityHref("service", "s1", null)).toBeNull();
    expect(entityHref("nope", "s1", "olex")).toBeNull();
  });

  it("gives every kind a read permission", () => {
    for (const kind of ENTITY_KINDS) {
      expect(ENTITY_READ_PERMISSIONS[kind].length).toBeGreaterThan(0);
    }
  });

  it("drives report links and search hits", () => {
    expect(reportLinkHref({ kind: "lead", uuid: "l1" }, "olex")).toBe(
      "/t/olex/leads/l1",
    );
    expect(resolveSearchHitHref("/leads/l1", "olex")).toBe("/t/olex/leads/l1");
    expect(resolveSearchHitHref("/fleets/f1", "olex")).toBe(
      "/t/olex/fleets/f1",
    );
  });
});

describe("EntityLink (TEC-1217)", () => {
  it("links the name with the read permission", () => {
    granted.add("customers.read");
    render(
      createElement(EntityLink, {
        kind: "customer",
        entity: { uuid: "c1", name: "Ayşe Yılmaz" },
      }),
    );
    const a = container.querySelector("a");
    expect(a?.getAttribute("href")).toBe("/t/olex/customers/c1");
    expect(a?.textContent).toBe("Ayşe Yılmaz");
  });

  it("renders plain text without the permission", () => {
    render(
      createElement(EntityLink, {
        kind: "customer",
        entity: { uuid: "c1", name: "Ayşe Yılmaz" },
      }),
    );
    expect(container.querySelector("a")).toBeNull();
    expect(container.textContent).toBe("Ayşe Yılmaz");
  });

  it("renders plain text for a ref without an id and the fallback for null", () => {
    granted.add("announcements.read");
    render(
      createElement(
        "div",
        null,
        createElement(EntityLink, {
          kind: "announcement",
          // TEC-1218: every kind has a page now; no id → no link.
          entity: { uuid: "", name: "Kampanya" },
        }),
        createElement(EntityLink, { kind: "customer", entity: null }),
      ),
    );
    expect(container.querySelector("a")).toBeNull();
    expect(container.textContent).toBe("Kampanya—");
  });
});
