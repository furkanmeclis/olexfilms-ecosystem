import { ScanBarcode, ShoppingBag, Tags, Truck } from "lucide-react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { defineNavGroup } from "@/features/nav-engine";

/**
 * TEC-348: dealer sale prices, quick sale, suppliers and purchases. Same
 * gates as /v1/dealer-prices, /v1/product-sales, /v1/suppliers and
 * /v1/purchases: a dealer with the accounting and dealer_accounting
 * modules, each page behind its own permission (dealer_owner and
 * dealer_accounting hold them, dealer_staff does not).
 */
export function dealerSalesNavGroup(slug: string) {
  return defineNavGroup({
    id: "dealer-sales",
    labelKey: "dealer_sales.nav",
    icon: ShoppingBag,
    defaultOpen: true,
    orgTypes: ["dealer"],
    feature: "dealer_accounting",
    anyPermission: [
      permissions.dealerSales.pricingWrite,
      permissions.dealerSales.salesWrite,
      permissions.dealerSales.suppliersManage,
      permissions.dealerSales.purchasesWrite,
    ],
    items: [
      {
        id: "dealer-sales-quick-sale",
        titleKey: "dealer_sales.nav_quick_sale",
        href: routes.tenant.dealerSales.quickSale(slug),
        icon: ScanBarcode,
        permission: permissions.dealerSales.salesWrite,
        orgTypes: ["dealer"],
        feature: "accounting",
      },
      {
        id: "dealer-sales-prices",
        titleKey: "dealer_sales.nav_prices",
        href: routes.tenant.dealerSales.prices(slug),
        icon: Tags,
        permission: permissions.dealerSales.pricingWrite,
        orgTypes: ["dealer"],
        feature: "accounting",
      },
      {
        id: "dealer-sales-suppliers",
        titleKey: "dealer_sales.nav_suppliers",
        href: routes.tenant.dealerSales.suppliers(slug),
        icon: Truck,
        permission: permissions.dealerSales.suppliersManage,
        orgTypes: ["dealer"],
        feature: "accounting",
      },
      {
        id: "dealer-sales-purchases",
        titleKey: "dealer_sales.nav_purchases",
        href: routes.tenant.dealerSales.purchases(slug),
        icon: ShoppingBag,
        permission: permissions.dealerSales.purchasesWrite,
        orgTypes: ["dealer"],
        feature: "accounting",
      },
    ],
  });
}
