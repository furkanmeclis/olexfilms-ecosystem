"use client";

import { Eye, Pencil, Trash2 } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { RoleSummary } from "@/features/roles/services/roles.service";
import { useLocale } from "@/providers/locale-provider";

export type RoleRowActionHandlers = {
  onView: (role: RoleSummary) => void;
  onEdit: (role: RoleSummary) => void;
  onDelete: (role: RoleSummary) => void;
};

type RoleRowActionsProps = {
  role: RoleSummary;
  handlers: RoleRowActionHandlers;
};

export function RoleRowActionsMenu({ role, handlers }: RoleRowActionsProps) {
  const { t } = useLocale();

  const actions = useMemo<EntityRowAction[]>(() => {
    const items: EntityRowAction[] = [
      {
        id: "view",
        label: t("roles.actions.view"),
        icon: Eye,
        permission: permissions.roles.read,
        onSelect: () => handlers.onView(role),
      },
    ];

    if (!role.is_system) {
      items.push(
        {
          id: "edit",
          label: t("roles.actions.edit"),
          icon: Pencil,
          permission: permissions.roles.write,
          onSelect: () => handlers.onEdit(role),
        },
        {
          id: "delete",
          label: t("roles.actions.delete"),
          icon: Trash2,
          permission: permissions.roles.write,
          variant: "destructive",
          onSelect: () => handlers.onDelete(role),
        },
      );
    }

    return items;
  }, [handlers, role, t]);

  return <EntityRowActions actions={actions} />;
}
