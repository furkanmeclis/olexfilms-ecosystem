import {
  Ban,
  CalendarPlus,
  CircleCheck,
  Lock,
  CircleOff,
  Flag,
  FolderInput,
  ListChecks,
  Trash2,
  UserCheck,
  UserPlus,
  UserX,
  type LucideIcon,
} from "lucide-react";

import type {
  BulkActionDef,
  BulkActionMeta,
  BulkResource,
} from "@/features/bulk-engine/types";

const bulkActionIconCatalog: Record<
  BulkResource,
  Record<string, LucideIcon>
> = {
  "platform.users": {
    disable: UserX,
    enable: UserCheck,
  },
  "platform.roles": {
    delete: Trash2,
  },
  "catalog.products": {
    activate: CircleCheck,
    deactivate: CircleOff,
    set_category: FolderInput,
  },
  tasks: {
    assign: UserPlus,
    set_status: ListChecks,
    set_priority: Flag,
  },
  "platform.organizations": {
    activate: CircleCheck,
    suspend: Ban,
    set_read_only: Lock,
    extend_access: CalendarPlus,
  },
  "catalog.categories": {
    activate: CircleCheck,
    deactivate: CircleOff,
    delete: Trash2,
  },
  "vehicle_catalog.brands": {
    activate: CircleCheck,
    deactivate: CircleOff,
  },
  "vehicle_catalog.models": {
    activate: CircleCheck,
    deactivate: CircleOff,
  },
  leads: {
    assign: UserPlus,
    set_status: ListChecks,
  },
};

export function resolveBulkActionIcon(
  resource: BulkResource,
  actionId: string,
): LucideIcon {
  const icon = bulkActionIconCatalog[resource]?.[actionId];
  if (!icon) {
    throw new Error(
      `bulk-engine: missing icon for ${resource} action "${actionId}"`,
    );
  }
  return icon;
}

/** Attach mandatory icons to meta bulk actions for UI rendering. */
export function resolveBulkActionsWithIcons(
  resource: BulkResource,
  actions: BulkActionMeta[],
): BulkActionDef[] {
  return actions.map((action) => ({
    ...action,
    icon: resolveBulkActionIcon(resource, action.id),
  }));
}
