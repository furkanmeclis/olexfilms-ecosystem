import type { LucideIcon } from "lucide-react";
import {
  ArrowLeftRight,
  ClipboardCheck,
  PackageCheck,
  PackagePlus,
  Timer,
  Truck,
} from "lucide-react";

import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import type { WarehouseOrgType } from "@/features/warehouse/lib/sections";

/** Which list endpoint a KPI counts; the component maps it to a service. */
export type WarehouseKpiSource = "orders" | "entries" | "transfers" | "counts";

/**
 * One live counter of the warehouse hub (TEC-1032, legacy "Kontrol paneli":
 * Hazırlık kuyruğu, Açık sipariş, Hazır/kargoda, Açık transfer, Açık sayım).
 * The count is the `total` of the list endpoint called with `limit=1`, so
 * every KPI is also a link to that filtered list.
 */
export type WarehouseKpi = {
  id: string;
  /** i18n key under `warehouse.kpis.*`. */
  key: string;
  icon: LucideIcon;
  source: WarehouseKpiSource;
  /** Query of the list endpoint, limit/offset excluded. */
  params: Record<string, string>;
  href: (slug: string) => string;
  /** Every permission is needed. */
  permissions: string[];
  orgTypes: WarehouseOrgType[];
  /** Highlighted when the count is above zero (work waiting). */
  attention?: boolean;
};

/** Seller orders still to pick and scan (approved, legacy processing, preparing). */
export const PREPARE_STATUSES = "approved,processing,preparing";
/** Seller orders packed or on the road. */
export const READY_SHIPPED_STATUSES = "ready,shipped";
/** Every seller order not yet closed. */
export const OPEN_ORDER_STATUSES =
  "submitted,approved,processing,preparing,ready,shipped,delivered";
export const OPEN_TRANSFER_STATUSES = "draft,in_transit";
export const OPEN_COUNT_STATUSES = "draft,in_progress,pending_review";

export const WAREHOUSE_KPIS: WarehouseKpi[] = [
  {
    id: "prepare_queue",
    key: "warehouse.kpis.prepare_queue",
    icon: Timer,
    source: "orders",
    params: { side: "seller", status: PREPARE_STATUSES },
    href: (slug) =>
      `${routes.tenant.orders.list(slug)}?status=${PREPARE_STATUSES}`,
    permissions: [Permission.OrdersRead, Permission.OrdersShip],
    orgTypes: ["center", "distributor"],
    attention: true,
  },
  {
    id: "ready_shipped",
    key: "warehouse.kpis.ready_shipped",
    icon: Truck,
    source: "orders",
    params: { side: "seller", status: READY_SHIPPED_STATUSES },
    href: (slug) =>
      `${routes.tenant.orders.list(slug)}?status=${READY_SHIPPED_STATUSES}`,
    permissions: [Permission.OrdersRead, Permission.OrdersShip],
    orgTypes: ["center", "distributor"],
  },
  {
    id: "open_orders",
    key: "warehouse.kpis.open_orders",
    icon: PackageCheck,
    source: "orders",
    params: { side: "seller", status: OPEN_ORDER_STATUSES },
    href: (slug) =>
      `${routes.tenant.orders.list(slug)}?status=${OPEN_ORDER_STATUSES}`,
    permissions: [Permission.OrdersRead],
    orgTypes: ["center", "distributor"],
  },
  {
    id: "draft_entries",
    key: "warehouse.kpis.draft_entries",
    icon: PackagePlus,
    source: "entries",
    params: { status: "draft" },
    href: (slug) => `${routes.tenant.warehouse.entries(slug)}?status=draft`,
    permissions: [Permission.WarehouseRead],
    orgTypes: ["center", "distributor"],
    attention: true,
  },
  {
    id: "open_transfers",
    key: "warehouse.kpis.open_transfers",
    icon: ArrowLeftRight,
    source: "transfers",
    params: { status: OPEN_TRANSFER_STATUSES },
    href: (slug) =>
      `${routes.tenant.warehouse.transfers(slug)}?status=${OPEN_TRANSFER_STATUSES}`,
    permissions: [Permission.WarehouseRead],
    orgTypes: ["center", "distributor"],
    attention: true,
  },
  {
    id: "open_counts",
    key: "warehouse.kpis.open_counts",
    icon: ClipboardCheck,
    source: "counts",
    params: { status: OPEN_COUNT_STATUSES },
    href: (slug) =>
      `${routes.tenant.warehouse.counts(slug)}?status=${OPEN_COUNT_STATUSES}`,
    permissions: [Permission.WarehouseRead],
    orgTypes: ["center", "distributor"],
    attention: true,
  },
];

/** KPIs the viewer may see: full warehouse org plus every permission. */
export function visibleKpis(
  can: (permission: string) => boolean,
  orgType: string | null | undefined,
): WarehouseKpi[] {
  return WAREHOUSE_KPIS.filter(
    (k) =>
      k.permissions.every((p) => can(p)) &&
      k.orgTypes.includes(orgType as WarehouseOrgType),
  );
}
