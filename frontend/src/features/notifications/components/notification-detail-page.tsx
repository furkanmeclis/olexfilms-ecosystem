"use client";

import { CheckCheck } from "lucide-react";
import { useEffect, useRef, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  canMarkNotificationRead,
  priorityTone,
  statusTone,
} from "@/features/notifications/constants";
import { useMarkNotificationRead } from "@/features/notifications/hooks/use-notification-mutations";
import { useNotificationRealtimeInvalidate } from "@/features/notifications/hooks/use-notification-realtime";
import { useNotification } from "@/features/notifications/hooks/use-notifications-query";
import { formatNotificationText } from "@/features/notifications/lib/notification-display";
import type { Notification } from "@/features/notifications/services/notifications.service";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type NotificationDetailPageProps = {
  uuid: string;
};

function DetailField({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="space-y-1">
      <dt className="text-muted-foreground text-xs font-medium">{label}</dt>
      <dd className="text-sm break-words">{value ?? "—"}</dd>
    </div>
  );
}

function localized(t: (key: string) => string, prefix: string, value: string) {
  const key = `${prefix}.${value}`;
  const label = t(key);
  return label === key ? value : label;
}

function NotificationDetailActions({
  notification,
}: {
  notification: Notification;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const markRead = useMarkNotificationRead();

  if (
    !can(permissions.notifications.read) ||
    !canMarkNotificationRead(notification)
  ) {
    return null;
  }

  return (
    <EntityActions>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={markRead.isPending}
        onClick={() => markRead.mutate(notification.uuid)}
      >
        <CheckCheck className="size-4" />
        {t("notifications.actions.mark_read")}
      </Button>
    </EntityActions>
  );
}

/** Own inbox notification detail (`GET /v1/notifications/{uuid}`). */
export function NotificationDetailPage({ uuid }: NotificationDetailPageProps) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const autoMarkedIdRef = useRef<string | null>(null);
  const markReadSilent = useMarkNotificationRead({ silent: true });
  const markReadMutateRef = useRef(markReadSilent.mutate);

  useNotificationRealtimeInvalidate();

  const notificationQuery = useNotification(uuid);
  const notification = notificationQuery.data;
  const title = notification?.title ?? t("notifications.detail_title");
  const canViewPlatform = can(permissions.notifications.platformRead);

  useEffect(() => {
    markReadMutateRef.current = markReadSilent.mutate;
  }, [markReadSilent.mutate]);

  useEffect(() => {
    autoMarkedIdRef.current = null;
  }, [uuid]);

  useEffect(() => {
    if (!notification) return;
    if (!canMarkNotificationRead(notification)) return;
    if (autoMarkedIdRef.current === notification.uuid) return;
    autoMarkedIdRef.current = notification.uuid;
    markReadMutateRef.current(notification.uuid);
  }, [notification]);

  return (
    <EntityPage
      title={title}
      description={t("notifications.detail_description")}
      permission={permissions.notifications.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("notifications.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        ...(canViewPlatform
          ? [
              {
                label: t("notifications.title"),
                href: routes.platform.notifications.root,
              },
            ]
          : [{ label: t("notifications.inbox.title") }]),
        { label: title },
      ]}
      actions={
        notification ? (
          <NotificationDetailActions notification={notification} />
        ) : null
      }
    >
      {notificationQuery.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : null}
      {notificationQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("notifications.detail_not_found")}
          onRetry={() => void notificationQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {notification ? (
        <div className="space-y-6">
          <EntityHeader
            title={notification.title}
            badges={
              <>
                <StatusChip
                  label={localized(
                    t,
                    "notifications.status",
                    notification.status,
                  )}
                  tone={statusTone(notification.status)}
                />
                <StatusChip
                  label={localized(
                    t,
                    "notifications.priority",
                    notification.priority,
                  )}
                  tone={priorityTone(notification.priority)}
                />
              </>
            }
            subtitle={localized(
              t,
              "notifications.channel",
              notification.channel,
            )}
          >
            <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <DetailField
                label={t("notifications.fields.template_code")}
                value={
                  notification.template_code ? (
                    <span className="font-mono text-xs">
                      {notification.template_code}
                    </span>
                  ) : (
                    "—"
                  )
                }
              />
              <DetailField
                label={t("notifications.fields.user")}
                value={
                  notification.user
                    ? `${notification.user.name} ${notification.user.surname}`.trim() +
                      ` · ${notification.user.email}`
                    : "—"
                }
              />
              <DetailField
                label={t("notifications.fields.recipient")}
                value={notification.recipient ?? "—"}
              />
              <DetailField
                label={t("notifications.fields.created_at")}
                value={datetime(notification.created_at, undefined, locale)}
              />
            </dl>
          </EntityHeader>

          <div className="grid gap-6 lg:grid-cols-2">
            <EntitySectionCard title={t("notifications.detail.overview")}>
              <EntityDetail
                sections={[
                  {
                    id: "overview",
                    fields: [
                      {
                        key: "uuid",
                        label: t("notifications.fields.uuid"),
                        value: (
                          <span className="font-mono text-xs">
                            {notification.uuid}
                          </span>
                        ),
                      },
                      {
                        key: "source_event",
                        label: t("notifications.fields.source_event"),
                        value: notification.source_event ?? "—",
                      },
                      {
                        key: "action_url",
                        label: t("notifications.fields.action_url"),
                        value: notification.action_url ?? "—",
                      },
                      {
                        key: "sent_at",
                        label: t("notifications.fields.sent_at"),
                        value: notification.sent_at
                          ? datetime(notification.sent_at, undefined, locale)
                          : "—",
                      },
                      {
                        key: "read_at",
                        label: t("notifications.fields.read_at"),
                        value: notification.read_at
                          ? datetime(notification.read_at, undefined, locale)
                          : "—",
                      },
                    ],
                  },
                ]}
              />
            </EntitySectionCard>

            <EntitySectionCard title={t("notifications.detail.body")}>
              <div className="space-y-3">
                <div>
                  <p className="text-muted-foreground mb-1 text-xs font-medium">
                    {t("notifications.fields.title")}
                  </p>
                  <p className="text-sm">{notification.title}</p>
                </div>
                <div>
                  <p className="text-muted-foreground mb-1 text-xs font-medium">
                    {t("notifications.fields.body")}
                  </p>
                  <pre className="bg-muted/40 max-h-80 overflow-auto rounded-md border p-3 text-xs whitespace-pre-wrap">
                    {formatNotificationText(notification.body, t)}
                  </pre>
                </div>
              </div>
            </EntitySectionCard>
          </div>
        </div>
      ) : null}
    </EntityPage>
  );
}
