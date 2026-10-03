import type { LucideIcon } from "lucide-react";
import {
  ArrowLeftRight,
  Barcode,
  ClipboardCheck,
  FolderTree,
  PackagePlus,
  ScanLine,
  Sunset,
} from "lucide-react";

import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";

export type WarehouseOrgType = "center" | "distributor";

/**
 * One screen of the warehouse feature. The hub page and the tests read
 * this registry; slice 2 (TEC-232) fills in `href` for transfers, counts
 * and end of day and adds their routes, pages and nav items.
 */
export type WarehouseSection = {
  id: string;
  /** i18n key prefix: `<key>.title`, `<key>.description`. */
  key: string;
  icon: LucideIcon;
  /** Null while the screen is planned for a later slice. */
  href: ((slug: string) => string) | null;
  /** Every permission is needed. */
  permissions: string[];
  orgTypes: WarehouseOrgType[];
  slice: "TEC-231" | "TEC-232";
};

export const WAREHOUSE_SECTIONS: WarehouseSection[] = [
  {
    id: "locations",
    key: "warehouse.sections.locations",
    icon: FolderTree,
    href: routes.tenant.warehouse.locations,
    permissions: [Permission.WarehouseRead],
    orgTypes: ["center", "distributor"],
    slice: "TEC-231",
  },
  {
    id: "scan",
    key: "warehouse.sections.scan",
    icon: ScanLine,
    href: routes.tenant.warehouse.scan,
    permissions: [Permission.WarehouseRead],
    orgTypes: ["center", "distributor"],
    slice: "TEC-231",
  },
  {
    id: "entries",
    key: "warehouse.sections.entries",
    icon: PackagePlus,
    href: routes.tenant.warehouse.entries,
    permissions: [Permission.WarehouseRead],
    orgTypes: ["center", "distributor"],
    slice: "TEC-231",
  },
  {
    id: "barcodes",
    key: "warehouse.sections.barcodes",
    icon: Barcode,
    href: routes.tenant.warehouse.barcodes,
    permissions: [Permission.StockRead],
    orgTypes: ["center"],
    slice: "TEC-231",
  },
  // --- TEC-232 (slice 2): set href when the screens land. ---
  {
    id: "transfers",
    key: "warehouse.sections.transfers",
    icon: ArrowLeftRight,
    href: null,
    permissions: [Permission.WarehouseRead],
    orgTypes: ["center", "distributor"],
    slice: "TEC-232",
  },
  {
    id: "counts",
    key: "warehouse.sections.counts",
    icon: ClipboardCheck,
    href: null,
    permissions: [Permission.WarehouseRead],
    orgTypes: ["center", "distributor"],
    slice: "TEC-232",
  },
  {
    id: "end_of_day",
    key: "warehouse.sections.end_of_day",
    icon: Sunset,
    href: null,
    permissions: [Permission.WarehouseRead],
    orgTypes: ["center", "distributor"],
    slice: "TEC-232",
  },
];

/** Sections the viewer may open (or will, once planned ones land). */
export function visibleSections(
  can: (permission: string) => boolean,
  orgType: string | null | undefined,
): WarehouseSection[] {
  return WAREHOUSE_SECTIONS.filter(
    (s) =>
      s.permissions.every((p) => can(p)) &&
      s.orgTypes.includes(orgType as WarehouseOrgType),
  );
}
