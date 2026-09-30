"use client";

import { Eye } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { permissions } from "@/config/permissions";
import type { Notification } from "@/features/notifications/services/notifications.service";
import { useLocale } from "@/providers/locale-provider";

export type NotificationRowActionHandlers = {
  onView: (notification: Notification) => void;
};

type NotificationRowActionsProps = {
  notification: Notification;
  handlers: NotificationRowActionHandlers;
};

export function NotificationRowActionsMenu({
  notification,
  handlers,
}: NotificationRowActionsProps) {
  const { t } = useLocale();

  const actions = useMemo<EntityRowAction[]>(
    () => [
      {
        id: "view",
        label: t("notifications.actions.view"),
        icon: Eye,
        permission: permissions.notifications.platformRead,
        onSelect: () => handlers.onView(notification),
      },
    ],
    [handlers, notification, t],
  );

  return <EntityRowActions actions={actions} />;
}
