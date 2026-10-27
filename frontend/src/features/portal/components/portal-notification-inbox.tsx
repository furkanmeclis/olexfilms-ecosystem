"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, BellOff, CheckCheck } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { NotificationInboxTrigger } from "@/features/notifications/components/notification-inbox-trigger";
import { notificationActions } from "@/features/notifications/lib/notification-action";
import {
  portalApi,
  type PortalNotification,
} from "@/features/portal/lib/portal-client";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export const PORTAL_NOTIFICATIONS_KEY = ["portal", "notifications"] as const;
const LIMIT = 20;

/**
 * TEC-1034: the portal session's in-app inbox (TEC-1021 endpoints) in the
 * portal header. A row marks itself read and opens its action link: portal
 * paths (appointments, a service's review card, warranties, home) as routes,
 * the public quote link in a new tab.
 */
export function PortalNotificationInbox() {
  const { t, format } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);

  const list = useQuery({
    queryKey: [...PORTAL_NOTIFICATIONS_KEY, "list"],
    queryFn: () => portalApi.listNotifications({ limit: LIMIT, offset: 0 }),
    enabled: open,
    refetchOnWindowFocus: true,
  });
  const unread = useQuery({
    queryKey: [...PORTAL_NOTIFICATIONS_KEY, "unread-count"],
    queryFn: () => portalApi.notificationsUnreadCount(),
    refetchOnWindowFocus: true,
    refetchInterval: 60_000,
    retry: false,
  });
  const refresh = () =>
    void qc.invalidateQueries({ queryKey: PORTAL_NOTIFICATIONS_KEY });
  const markRead = useMutation({
    mutationFn: (uuid: string) => portalApi.markNotificationRead(uuid),
    onSuccess: refresh,
  });
  const markAll = useMutation({
    mutationFn: () => portalApi.markAllNotificationsRead(),
    onSuccess: refresh,
  });

  const items = list.data?.items ?? [];
  const unreadCount = unread.data?.count ?? 0;

  const openItem = (n: PortalNotification) => {
    if (!n.read_at) markRead.mutate(n.uuid);
    const action = notificationActions(n.action_url)[0];
    if (!action) return;
    if (action.kind === "route") {
      setOpen(false);
      router.push(action.href);
    } else {
      window.open(action.href, "_blank", "noopener,noreferrer");
    }
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <NotificationInboxTrigger
          unreadCount={unreadCount}
          data-testid="portal-notifications-trigger"
        />
      </PopoverTrigger>
      <PopoverContent
        align="end"
        sideOffset={8}
        className="w-[min(24rem,calc(100vw-1.5rem))] overflow-hidden rounded-xl p-0 shadow-lg"
        data-testid="portal-notifications"
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
              disabled={markAll.isPending}
              onClick={() => markAll.mutate()}
            >
              <CheckCheck className="size-3.5" />
              {t("notifications.inbox.mark_all_read")}
            </Button>
          ) : null}
        </div>
        <Separator />
        <ScrollArea className="h-80">
          {list.isLoading ? (
            <div className="space-y-2 p-3">
              {Array.from({ length: 4 }).map((_, i) => (
                <div key={i} className="space-y-2 rounded-lg px-2 py-2">
                  <div className="bg-muted h-3.5 w-2/3 animate-pulse rounded" />
                  <div className="bg-muted h-3 w-full animate-pulse rounded" />
                </div>
              ))}
            </div>
          ) : list.isError ? (
            <div className="flex flex-col items-center gap-3 px-6 py-10 text-center">
              <p className="text-sm">{t("notifications.inbox.error")}</p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void list.refetch()}
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
            </div>
          ) : (
            <ul className="p-1.5">
              {items.map((item) => {
                const isUnread = !item.read_at;
                return (
                  <li key={item.uuid}>
                    <button
                      type="button"
                      data-testid="portal-notification-row"
                      className={cn(
                        "hover:bg-muted/60 relative flex w-full flex-col gap-1 rounded-lg px-3.5 py-2.5 ps-5 text-start transition-colors",
                        isUnread && "bg-primary/[0.04]",
                      )}
                      onClick={() => openItem(item)}
                    >
                      {isUnread ? (
                        <span
                          aria-hidden
                          className="bg-primary absolute start-2 top-4 size-1.5 rounded-full"
                        />
                      ) : null}
                      <div className="flex items-start justify-between gap-3">
                        <span
                          className={cn(
                            "line-clamp-2 text-sm leading-snug",
                            isUnread
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
                        <p className="text-muted-foreground line-clamp-3 text-xs leading-relaxed">
                          {item.body}
                        </p>
                      ) : null}
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </ScrollArea>
      </PopoverContent>
    </Popover>
  );
}
