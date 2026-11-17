import { permissions } from "@/config/permissions";
import type { EntityKind } from "@/config/routes";

/**
 * TEC-1217: permission(s) that open the detail page of a kind. Any one of
 * the listed slugs is enough. Without one `EntityLink` renders plain text.
 */
export const ENTITY_READ_PERMISSIONS: Record<EntityKind, readonly string[]> = {
  service: [permissions.services.read],
  warranty: [permissions.warranties.read],
  warranty_claim: [permissions.warrantyClaims.read],
  customer: [permissions.customers.read],
  vehicle: [permissions.vehicles.read],
  order: [permissions.orders.read],
  product: [permissions.catalog.read],
  stock_unit: [permissions.stock.read],
  transfer: [permissions.transfers.request, permissions.transfers.approve],
  organization: [permissions.organizations.tenantRead],
  user: [permissions.members.read],
  lead: [permissions.leads.read],
  appointment: [permissions.appointments.read],
  task: [permissions.tasks.read],
  measurement: [permissions.measurements.read],
  contract: [permissions.contracts.read],
  invoice: [permissions.einvoice.read],
  cari: [permissions.accounting.read],
  announcement: [permissions.announcements.read],
  warehouse: [permissions.warehouse.read],
  subscription: [permissions.serviceSubscriptions.read],
};

/** Whether a permission check opens the detail page of the kind. */
export function canOpenEntity(
  kind: EntityKind,
  can: (permission: string) => boolean,
): boolean {
  return ENTITY_READ_PERMISSIONS[kind].some((slug) => can(slug));
}

/** i18n key of a kind's singular label (`entity.kinds.*`). */
export function entityKindLabelKey(kind: EntityKind): string {
  return `entity.kinds.${kind}`;
}
