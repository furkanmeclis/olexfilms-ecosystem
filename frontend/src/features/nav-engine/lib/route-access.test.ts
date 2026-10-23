import { Circle as Icon } from "lucide-react";
import { describe, expect, it } from "vitest";

import type { NavAccess } from "./access";
import { matchNavEntries, navRouteAccess } from "./route-access";
import type { NavCatalog } from "../types";

const catalog: NavCatalog = {
  id: "t",
  groups: [
    {
      id: "main",
      labelKey: "g",
      items: [
        { id: "home", titleKey: "h", href: "/t/a", icon: Icon },
        {
          id: "stock-mine",
          titleKey: "s",
          href: "/t/a/stock",
          icon: Icon,
          permission: "stock.read",
          orgTypes: ["dealer"],
        },
        {
          id: "stock-center",
          titleKey: "s",
          href: "/t/a/stock",
          icon: Icon,
          permission: "stock.read",
          orgTypes: ["center"],
        },
        {
          id: "orders-new",
          titleKey: "o",
          href: "/t/a/orders/new",
          icon: Icon,
          permission: "orders.write",
        },
        {
          id: "orders",
          titleKey: "o",
          href: "/t/a/orders",
          icon: Icon,
          permission: "orders.read",
          feature: "orders",
        },
      ],
    },
  ],
};

function access(
  perms: string[],
  org: NavAccess["org"] = {
    type: "dealer",
    role: "owner",
    features: ["orders"],
  },
): NavAccess {
  return {
    can: (p) => (Array.isArray(p) ? p : [p]).every((x) => perms.includes(x)),
    canAny: (ps) => ps.some((x) => perms.includes(x)),
    org,
  };
}

describe("matchNavEntries (TEC-771)", () => {
  it("picks the longest href prefix and keeps entries sharing it", () => {
    expect(
      matchNavEntries("/t/a/orders/new", [catalog], "/t/a").map(
        (m) => m.item.id,
      ),
    ).toEqual(["orders-new"]);
    expect(
      matchNavEntries("/t/a/orders/123?tab=x", [catalog], "/t/a").map(
        (m) => m.item.id,
      ),
    ).toEqual(["orders"]);
    expect(
      matchNavEntries("/t/a/stock/units/B1", [catalog], "/t/a").map(
        (m) => m.item.id,
      ),
    ).toEqual(["stock-mine", "stock-center"]);
  });

  it("matches the shell home only exactly", () => {
    expect(matchNavEntries("/t/a/profile", [catalog], "/t/a")).toEqual([]);
    expect(matchNavEntries("/t/a/", [catalog], "/t/a")[0]?.item.id).toBe(
      "home",
    );
  });
});

describe("navRouteAccess (TEC-771)", () => {
  it("opens pages without a nav entry", () => {
    expect(navRouteAccess("/t/a/profile", [catalog], access([]), "/t/a")).toBe(
      "allowed",
    );
  });

  it("denies a page the menu hides", () => {
    expect(
      navRouteAccess(
        "/t/a/orders/new",
        [catalog],
        access(["orders.read"]),
        "/t/a",
      ),
    ).toBe("denied");
    expect(
      navRouteAccess(
        "/t/a/orders/new",
        [catalog],
        access(["orders.write"]),
        "/t/a",
      ),
    ).toBe("allowed");
  });

  it("allows when any entry sharing the href is visible", () => {
    expect(
      navRouteAccess(
        "/t/a/stock",
        [catalog],
        access(["stock.read"], { type: "center", role: "staff", features: [] }),
        "/t/a",
      ),
    ).toBe("allowed");
    expect(
      navRouteAccess(
        "/t/a/stock",
        [catalog],
        access(["stock.read"], {
          type: "distributor",
          role: "owner",
          features: [],
        }),
        "/t/a",
      ),
    ).toBe("denied");
  });

  it("gates only the exact href of an exactRoute entry (TEC-866)", () => {
    const exact: NavCatalog = {
      id: "x",
      groups: [
        {
          id: "main",
          labelKey: "g",
          items: [
            {
              id: "list",
              titleKey: "l",
              href: "/t/a/warranties",
              icon: Icon,
              orgTypes: ["center"],
              exactRoute: true,
            },
          ],
        },
      ],
    };
    const dealer = access([]);
    expect(navRouteAccess("/t/a/warranties", [exact], dealer, "/t/a")).toBe(
      "denied",
    );
    expect(navRouteAccess("/t/a/warranties/w1", [exact], dealer, "/t/a")).toBe(
      "allowed",
    );
  });

  it("leaves module gates to the page's own off screen", () => {
    const off = { type: "dealer", role: "owner", features: [] };
    expect(
      navRouteAccess(
        "/t/a/orders",
        [catalog],
        access(["orders.read"], off),
        "/t/a",
      ),
    ).toBe("allowed");
    expect(
      navRouteAccess("/t/a/orders", [catalog], access([], off), "/t/a"),
    ).toBe("denied");
  });
});
