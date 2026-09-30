"use client";

import { Bell, Download, FileUp, ShieldAlert, X } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  canMarkNotificationRead,
  isInboxChannel,
} from "@/features/notifications/constants";
import { useMarkNotificationRead } from "@/features/notifications/hooks/use-notification-mutations";
import type { LiveNotificationItem } from "@/features/notifications/lib/notification-live-store";
import { notificationActions } from "@/features/notifications/lib/notification-action";
import { formatNotificationText } from "@/features/notifications/lib/notification-display";
import { runNotificationAction } from "@/features/notifications/lib/run-notification-action";
import { useOptionalTenant } from "@/features/organizations/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

type NotificationLiveCardProps = {
  item: LiveNotificationItem;
  onDismiss: (id: string) => void;
};

function NotificationCardIcon({
  notification,
}: {
  notification: LiveNotificationItem;
}) {
  const className = "size-4";
  if (notification.priority === "critical") {
    return <ShieldAlert className={className} />;
  }
  if (notification.template_code?.startsWith("exports.")) {
    return <Download className={className} />;
  }
  if (notification.template_code?.startsWith("imports.")) {
    return <FileUp className={className} />;
  }
  return <Bell className={className} />;
}

function accentClass(priority?: string) {
  switch (priority) {
    case "critical":
      return "bg-destructive";
    case "high":
      return "bg-amber-500";
    default:
      return "bg-primary";
  }
}

function iconWrapClass(priority?: string) {
  switch (priority) {
    case "critical":
      return "bg-destructive/12 text-destructive";
    case "high":
      return "bg-amber-500/12 text-amber-700 dark:text-amber-300";
    default:
      return "bg-primary/10 text-primary";
  }
}

export function NotificationLiveCard({
  item,
  onDismiss,
}: NotificationLiveCardProps) {
  const { t } = useLocale();
  const router = useRouter();
  const tenant = useOptionalTenant();
  const markRead = useMarkNotificationRead({ silent: true });
  const actions = notificationActions(item.action_url, {
    tenantSlug: tenant?.slug,
    payload: item.payload,
  });
  const [entered, setEntered] = useState(false);
  const [progress, setProgress] = useState(100);
  const pausedRef = useRef(false);
  const remainingRef = useRef(item.durationMs);
  const rafRef = useRef<number | null>(null);
  const lastFrameRef = useRef<number | null>(null);
  const finite = Number.isFinite(item.durationMs);

  useEffect(() => {
    const frame = requestAnimationFrame(() => setEntered(true));
    return () => cancelAnimationFrame(frame);
  }, []);

  useEffect(() => {
    if (!finite) return;

    remainingRef.current = item.durationMs;
    pausedRef.current = false;
    lastFrameRef.current = null;

    const tick = (now: number) => {
      if (lastFrameRef.current != null && !pausedRef.current) {
        const delta = now - lastFrameRef.current;
        remainingRef.current = Math.max(0, remainingRef.current - delta);
        setProgress((remainingRef.current / item.durationMs) * 100);
        if (remainingRef.current <= 0) {
          onDismiss(item.id);
          return;
        }
      }
      lastFrameRef.current = now;
      rafRef.current = requestAnimationFrame(tick);
    };

    lastFrameRef.current = null;
    rafRef.current = requestAnimationFrame(tick);
    return () => {
      if (rafRef.current != null) cancelAnimationFrame(rafRef.current);
    };
  }, [finite, item.durationMs, item.id, onDismiss]);

  const dismiss = () => onDismiss(item.id);

  const markInboxRead = () => {
    if (
      item.uuid &&
      isInboxChannel(item.channel) &&
      canMarkNotificationRead({
        channel: item.channel ?? "inapp",
        status: item.status ?? "delivered",
      })
    ) {
      markRead.mutate(item.uuid);
    }
  };

  const run = async (index: number) => {
    const action = actions[index];
    if (!action) return;
    try {
      await runNotificationAction({
        action,
        navigate: (href) => router.push(href),
        notification: item,
      });
      markInboxRead();
      dismiss();
    } catch {
      appToast.error(t("notifications.toast.action_failed"));
    }
  };

  return (
    <article
      role="status"
      aria-live="polite"
      onPointerEnter={() => {
        pausedRef.current = true;
      }}
      onPointerLeave={() => {
        pausedRef.current = false;
      }}
      className={cn(
        "bg-popover/95 text-popover-foreground pointer-events-auto relative w-[min(24rem,calc(100vw-2rem))] overflow-hidden rounded-2xl border shadow-2xl ring-1 ring-black/5 backdrop-blur-md transition-all duration-300 ease-out dark:ring-white/10",
        entered
          ? "translate-x-0 scale-100 opacity-100"
          : "translate-x-6 scale-95 opacity-0",
      )}
    >
      <div className="flex gap-3 p-4">
        <div
          className={cn(
            "mt-0.5 flex size-10 shrink-0 items-center justify-center rounded-xl",
            iconWrapClass(item.priority),
          )}
        >
          <NotificationCardIcon notification={item} />
        </div>
        <div className="min-w-0 flex-1 space-y-2.5">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0 space-y-1">
              <p className="text-muted-foreground text-[11px] font-medium tracking-wide uppercase">
                {t("notifications.toast.live_label")}
              </p>
              <p className="text-sm leading-snug font-semibold">
                {formatNotificationText(item.title, t)}
              </p>
              {item.body ? (
                <p className="text-muted-foreground line-clamp-2 text-xs leading-relaxed">
                  {formatNotificationText(item.body, t)}
                </p>
              ) : null}
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              className="text-muted-foreground -me-1 -mt-1"
              aria-label={t("notifications.toast.dismiss")}
              onClick={dismiss}
            >
              <X className="size-3.5" />
            </Button>
          </div>
          {actions.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {actions.map((action, index) => (
                <Button
                  key={`${action.kind}-${action.href}`}
                  type="button"
                  size="sm"
                  variant={index === 0 ? "default" : "outline"}
                  className="h-8 px-3 text-xs"
                  onClick={() => void run(index)}
                >
                  {t(action.labelKey)}
                </Button>
              ))}
            </div>
          ) : null}
        </div>
      </div>
      {finite ? (
        <div className="bg-muted/70 absolute inset-x-0 bottom-0 h-0.5">
          <div
            className={cn("h-full", accentClass(item.priority))}
            style={{ width: `${progress}%` }}
          />
        </div>
      ) : null}
    </article>
  );
}
