"use client";

import { Bell, BellOff, CheckCheck } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { NotificationDetailDrawer } from "@/features/notifications/components/notification-detail-drawer";
import { NotificationInboxTrigger } from "@/features/notifications/components/notification-inbox-trigger";
import {
  canMarkNotificationRead,
  isUnreadNotification,
} from "@/features/notifications/constants";
import {
  useMarkAllNotificationsRead,
  useMarkNotificationRead,
} from "@/features/notifications/hooks/use-notification-mutations";
import { useNotificationRealtimeInvalidate } from "@/features/notifications/hooks/use-notification-realtime";
import {
  useNotificationsInbox,
  useNotificationsUnreadCount,
} from "@/features/notifications/hooks/use-notifications-query";
import { notificationActions } from "@/features/notifications/lib/notification-action";
import { formatNotificationText } from "@/features/notifications/lib/notification-display";
import { runNotificationAction } from "@/features/notifications/lib/run-notification-action";
import type { Notification } from "@/features/notifications/services/notifications.service";
import { useOptionalTenant } from "@/features/organizations/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export function NotificationInbox() {
  const { t, format } = useLocale();
  const router = useRouter();
  const { can } = usePermission();
  const tenant = useOptionalTenant();
  const enabled = can(permissions.notifications.read);

  useNotificationRealtimeInvalidate(enabled);

  const inboxQuery = useNotificationsInbox(enabled);
  const unreadQuery = useNotificationsUnreadCount(enabled);
  const markRead = useMarkNotificationRead({ silent: true });
  const markAllRead = useMarkAllNotificationsRead();
  const [selected, setSelected] = useState<Notification | null>(null);

  const items = useMemo(
    () => inboxQuery.data?.items ?? [],
    [inboxQuery.data?.items],
  );
  const unreadCount = unreadQuery.data?.count ?? 0;
  const canViewPlatform = can(permissions.notifications.platformRead);

  if (!enabled) return null;

  const openItem = (notification: Notification) => {
    setSelected(notification);
    if (canMarkNotificationRead(notification)) {
      markRead.mutate(notification.uuid);
    }
  };

  const runAction = async (
    notification: Notification,
    href: string,
    kind: "download" | "route" | "external",
  ) => {
    try {
      await runNotificationAction({
        action: { kind, href, labelKey: "notifications.actions.open" },
        navigate: (next) => router.push(next),
        notification,
      });
      if (canMarkNotificationRead(notification)) {
        markRead.mutate(notification.uuid);
      }
    } catch {
      appToast.error(t("notifications.toast.action_failed"));
    }
  };

  return (
    <PermissionGuard permission={permissions.notifications.read}>
      <Popover>
        <PopoverTrigger asChild>
          <NotificationInboxTrigger unreadCount={unreadCount} />
        </PopoverTrigger>
        <PopoverContent
          align="end"
          sideOffset={8}
          className="w-[min(24rem,calc(100vw-1.5rem))] overflow-hidden rounded-xl p-0 shadow-lg"
        >
          <div className="flex items-start justify-between gap-3 px-4 py-3.5">
            <div className="flex min-w-0 items-start gap-2.5">
              <span className="bg-primary/10 text-primary mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg">
                <Bell className="size-4" />
              </span>
              <div className="min-w-0">
                <p className="text-sm font-semibold">
                  {t("notifications.inbox.title")}
                </p>
                <p className="text-muted-foreground text-xs">
                  {t("notifications.inbox.subtitle")}
                </p>
              </div>
            </div>
            {unreadCount > 0 ? (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="text-muted-foreground hover:text-foreground h-7 shrink-0 gap-1 px-2 text-xs"
                disabled={markAllRead.isPending}
                onClick={() => markAllRead.mutate()}
              >
                <CheckCheck className="size-3.5" />
                {t("notifications.inbox.mark_all_read")}
              </Button>
            ) : null}
          </div>
          <Separator />
          <ScrollArea className="h-80">
            {inboxQuery.isLoading ? (
              <InboxSkeleton />
            ) : inboxQuery.isError ? (
              <div className="flex flex-col items-center gap-3 px-6 py-10 text-center">
                <p className="text-sm">{t("notifications.inbox.error")}</p>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    void inboxQuery.refetch();
                    void unreadQuery.refetch();
                  }}
                >
                  {t("common.retry")}
                </Button>
              </div>
            ) : items.length === 0 ? (
              <div className="flex flex-col items-center gap-2 px-6 py-12 text-center">
                <span className="bg-muted text-muted-foreground flex size-11 items-center justify-center rounded-full">
                  <BellOff className="size-4" />
                </span>
                <p className="text-sm font-medium">
                  {t("notifications.inbox.empty")}
                </p>
                <p className="text-muted-foreground text-xs">
                  {t("notifications.inbox.empty_hint")}
                </p>
              </div>
            ) : (
              <ul className="p-1.5">
                {items.map((item) => {
                  const unread = isUnreadNotification(item);
                  const actions = notificationActions(item.action_url, {
                    tenantSlug: tenant?.slug,
                    payload: item.payload,
                  });
                  return (
                    <li key={item.uuid}>
                      <div
                        className={cn(
                          "hover:bg-muted/60 relative rounded-lg transition-colors",
                          unread && "bg-primary/[0.04]",
                        )}
                      >
                        {unread ? (
                          <span
                            aria-hidden
                            className="bg-primary absolute start-2 top-4 size-1.5 rounded-full"
                          />
                        ) : null}
                        <button
                          type="button"
                          className="flex w-full flex-col gap-1 px-3.5 py-2.5 ps-5 text-start"
                          onClick={() => openItem(item)}
                        >
                          <div className="flex items-start justify-between gap-3">
                            <span
                              className={cn(
                                "line-clamp-2 text-sm leading-snug",
                                unread
                                  ? "font-semibold"
                                  : "text-foreground/80 font-medium",
                              )}
                            >
                              {item.title}
                            </span>
                            <span className="text-muted-foreground shrink-0 text-[11px]">
                              {format.relative(item.created_at)}
                            </span>
                          </div>
                          {item.body ? (
                            <p className="text-muted-foreground line-clamp-2 text-xs leading-relaxed">
                              {formatNotificationText(item.body, t)}
                            </p>
                          ) : null}
                        </button>
                        {actions.length > 0 ? (
                          <div className="flex flex-wrap gap-1.5 px-5 pb-2.5">
                            {actions.slice(0, 2).map((action, index) => (
                              <Button
                                key={`${action.kind}-${action.href}`}
                                type="button"
                                size="sm"
                                variant={index === 0 ? "default" : "outline"}
                                className="h-7 px-2.5 text-xs"
                                onClick={() =>
                                  void runAction(item, action.href, action.kind)
                                }
                              >
                                {t(action.labelKey)}
                              </Button>
                            ))}
                          </div>
                        ) : null}
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </ScrollArea>
          {canViewPlatform ? (
            <>
              <Separator />
              <div className="p-2">
                <Button
                  asChild
                  variant="ghost"
                  className="text-muted-foreground hover:text-foreground w-full justify-center text-xs"
                >
                  <Link href={routes.platform.notifications.root}>
                    {t("notifications.inbox.view_all")}
                  </Link>
                </Button>
              </div>
            </>
          ) : null}
        </PopoverContent>
      </Popover>

      <NotificationDetailDrawer
        notification={selected}
        open={Boolean(selected)}
        onOpenChange={(next) => {
          if (!next) setSelected(null);
        }}
      />
    </PermissionGuard>
  );
}

function InboxSkeleton() {
  return (
    <div className="space-y-2 p-3">
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index} className="space-y-2 rounded-lg px-2 py-2">
          <div className="bg-muted h-3.5 w-2/3 animate-pulse rounded" />
          <div className="bg-muted h-3 w-full animate-pulse rounded" />
          <div className="bg-muted h-3 w-1/3 animate-pulse rounded" />
        </div>
      ))}
    </div>
  );
}
