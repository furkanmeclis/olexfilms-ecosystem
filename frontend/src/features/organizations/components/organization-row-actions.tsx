"use client";

import { Eye, Pencil } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { Organization } from "@/features/organizations/services/organizations.service";
import { useLocale } from "@/providers/locale-provider";

export type OrganizationRowActionHandlers = {
  onView: (organization: Organization) => void;
  onEdit: (organization: Organization) => void;
};

type OrganizationRowActionsProps = {
  organization: Organization;
  handlers: OrganizationRowActionHandlers;
};

export function OrganizationRowActionsMenu({
  organization,
  handlers,
}: OrganizationRowActionsProps) {
  const { t } = useLocale();

  const actions = useMemo<EntityRowAction[]>(
    () => [
      {
        id: "view",
        label: t("organizations.actions.view"),
        icon: Eye,
        permission: permissions.organizations.read,
        onSelect: () => handlers.onView(organization),
      },
      {
        id: "edit",
        label: t("organizations.actions.edit"),
        icon: Pencil,
        permission: permissions.organizations.write,
        onSelect: () => handlers.onEdit(organization),
      },
    ],
    [handlers, organization, t],
  );

  return <EntityRowActions actions={actions} />;
}
