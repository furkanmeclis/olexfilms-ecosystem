import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(
  granted: string[],
  orgType: string,
  features: string[] = ["stock", "stock_forecast"],
) {
  const access = {
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
    canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
    org: { type: orgType, role: "staff", features },
  };
  return tenantNav("acme").groups.flatMap((group) =>
    visibleNavItems(group, access).map((item) => item.id),
  );
}

describe("tenant nav: stock forecast (TEC-755)", () => {
  it("links the forecast page from its own group", () => {
    const group = tenantNav("acme").groups.find(
      (g) => g.id === "stock-forecast",
    );
    expect(group?.feature).toBe("stock_forecast");
    expect(group?.permission).toBe(Permission.StockForecastRead);
    expect(group?.items[0]?.href).toBe(
      routes.tenant.stockForecast.list("acme"),
    );
  });

  it("shows the item to the center (network demand)", () => {
    expect(
      visibleIds(
        [Permission.StockForecastRead, Permission.StockForecastNetworkRead],
        "center",
      ),
    ).toContain("stock-forecast");
  });

  // Dealer staff and distributor staff hold stock_forecast.read without
  // stock.read (backend rbac catalog).
  it("shows the item to dealer and distributor staff without stock.read", () => {
    expect(visibleIds([Permission.StockForecastRead], "dealer")).toContain(
      "stock-forecast",
    );
    expect(visibleIds([Permission.StockForecastRead], "distributor")).toContain(
      "stock-forecast",
    );
  });

  it("is hidden without the module or stock_forecast.read", () => {
    expect(
      visibleIds([Permission.StockForecastRead], "dealer", ["stock"]),
    ).not.toContain("stock-forecast");
    expect(visibleIds([Permission.StockRead], "center")).not.toContain(
      "stock-forecast",
    );
  });
});
