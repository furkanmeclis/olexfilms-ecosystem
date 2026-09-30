"use client";

import {
  Eye,
  KeyRound,
  Pencil,
  ShieldCheck,
  ShieldOff,
  UserRoundSearch,
} from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { PublicUser } from "@/features/users/services/users.service";
import { useLocale } from "@/providers/locale-provider";

export type UserRowActionHandlers = {
  onView: (user: PublicUser) => void;
  onEdit: (user: PublicUser) => void;
  onEnable: (user: PublicUser) => void;
  onDisable: (user: PublicUser) => void;
  onSetPassword: (user: PublicUser) => void;
  onImpersonate?: (user: PublicUser) => void;
};

type UserRowActionsProps = {
  user: PublicUser;
  handlers: UserRowActionHandlers;
  currentUserUuid?: string;
  canImpersonateSuperAdmin?: boolean;
  isImpersonating?: boolean;
};

export function UserRowActionsMenu({
  user,
  handlers,
  currentUserUuid,
  canImpersonateSuperAdmin = false,
  isImpersonating = false,
}: UserRowActionsProps) {
  const { t } = useLocale();

  const actions = useMemo<EntityRowAction[]>(() => {
    const items: EntityRowAction[] = [
      {
        id: "view",
        label: t("users.actions.view"),
        icon: Eye,
        permission: permissions.users.read,
        onSelect: () => handlers.onView(user),
      },
      {
        id: "edit",
        label: t("users.actions.edit"),
        icon: Pencil,
        permission: permissions.users.write,
        onSelect: () => handlers.onEdit(user),
      },
      {
        id: "set-password",
        label: t("users.actions.set_password"),
        icon: KeyRound,
        permission: permissions.users.write,
        onSelect: () => handlers.onSetPassword(user),
      },
    ];

    if (user.status === "disabled" || user.status === "pending") {
      items.push({
        id: "enable",
        label: t("users.actions.enable"),
        icon: ShieldCheck,
        permission: permissions.users.write,
        onSelect: () => handlers.onEnable(user),
      });
    }

    if (user.status === "active") {
      items.push({
        id: "disable",
        label: t("users.actions.disable"),
        icon: ShieldOff,
        permission: permissions.users.write,
        onSelect: () => handlers.onDisable(user),
      });
    }

    const canImpersonateTarget =
      user.status === "active" &&
      user.uuid !== currentUserUuid &&
      !isImpersonating &&
      (!user.is_super_admin || canImpersonateSuperAdmin);

    if (handlers.onImpersonate && canImpersonateTarget) {
      items.push({
        id: "impersonate",
        label: t("users.actions.impersonate"),
        icon: UserRoundSearch,
        permission: permissions.users.impersonate,
        onSelect: () => handlers.onImpersonate?.(user),
      });
    }

    return items;
  }, [
    canImpersonateSuperAdmin,
    currentUserUuid,
    handlers,
    isImpersonating,
    t,
    user,
  ]);

  return <EntityRowActions actions={actions} />;
}
