"use client";

import { Bell } from "lucide-react";
import { forwardRef, type ComponentPropsWithoutRef } from "react";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

type NotificationInboxTriggerProps = ComponentPropsWithoutRef<"button"> & {
  unreadCount: number;
};

export const NotificationInboxTrigger = forwardRef<
  HTMLButtonElement,
  NotificationInboxTriggerProps
>(function NotificationInboxTrigger({ unreadCount, className, ...props }, ref) {
  const { t } = useLocale();
  const label =
    unreadCount > 0
      ? t("notifications.inbox.trigger_unread", { count: unreadCount })
      : t("notifications.inbox.trigger");

  return (
    <Button
      ref={ref}
      type="button"
      variant="ghost"
      size="icon"
      className={cn("relative", className)}
      aria-label={label}
      {...props}
    >
      <Bell className="size-4" />
      {unreadCount > 0 ? (
        <span className="absolute -end-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center">
          <span className="bg-destructive/40 absolute inset-0 animate-ping rounded-full" />
          <span className="bg-destructive relative flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-[10px] leading-none font-semibold text-white">
            {unreadCount > 99 ? "99+" : unreadCount}
          </span>
        </span>
      ) : null}
    </Button>
  );
});
