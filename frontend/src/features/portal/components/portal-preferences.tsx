"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BellRing } from "lucide-react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { Card, CardContent } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { ConnectedAppsCard } from "@/features/mcp";
import { PortalPage } from "@/features/portal/components/portal-page";
import {
  portalApi,
  type PortalNotificationPreferences,
} from "@/features/portal/lib/portal-client";
import { useLocale } from "@/providers/locale-provider";

export const PORTAL_PREFERENCES_KEY = [
  "portal",
  "notification-preferences",
] as const;

type Toggle = "email_enabled" | "inapp_enabled";

const TOGGLES: readonly { field: Toggle; key: string }[] = [
  { field: "email_enabled", key: "email" },
  { field: "inapp_enabled", key: "inapp" },
];

/**
 * Portal > Notification preferences (TEC-245): the F0-10 preferences of
 * the signed-in user (GET / PUT /v1/portal/notification-preferences). Each
 * switch saves at once; the whole object is sent back so the per-event
 * rules stay as they are. Fleet accounts may change their own settings.
 */
export function PortalPreferences() {
  const { t } = useLocale();
  const qc = useQueryClient();
  const prefs = useQuery({
    queryKey: PORTAL_PREFERENCES_KEY,
    queryFn: () => portalApi.getNotificationPreferences(),
  });
  const save = useMutation({
    mutationFn: (next: PortalNotificationPreferences) =>
      portalApi.updateNotificationPreferences(next),
    onSuccess: (saved) => {
      qc.setQueryData(PORTAL_PREFERENCES_KEY, saved);
      toast.success(t("portal.preferences.saved"));
    },
    onError: () => toast.error(t("portal.preferences.failed")),
  });

  const onToggle = (field: Toggle, value: boolean) => {
    if (!prefs.data) return;
    save.mutate({ ...prefs.data, [field]: value });
  };

  return (
    <PortalPage
      title={t("portal.preferences.title")}
      icon={<BellRing className="size-6" />}
      testId="portal-preferences-page"
    >
      {prefs.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void prefs.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : !prefs.data ? (
        <p className="text-muted-foreground text-sm">
          {t("portal.preferences.loading")}
        </p>
      ) : (
        <Card>
          <CardContent className="divide-y pt-2">
            <p className="text-muted-foreground py-4 text-sm">
              {t("portal.preferences.description")}
            </p>
            {TOGGLES.map(({ field, key }) => {
              const id = `portal-pref-${field}`;
              const checked = save.isPending
                ? (save.variables?.[field] ?? prefs.data[field])
                : prefs.data[field];
              return (
                <div
                  key={field}
                  className="flex items-start justify-between gap-4 py-4"
                >
                  <div className="space-y-1">
                    <Label htmlFor={id}>{t(`portal.preferences.${key}`)}</Label>
                    <p className="text-muted-foreground text-sm">
                      {t(`portal.preferences.${key}_hint`)}
                    </p>
                  </div>
                  <Switch
                    id={id}
                    data-testid={id}
                    checked={checked}
                    disabled={save.isPending}
                    onCheckedChange={(v) => onToggle(field, v)}
                  />
                </div>
              );
            })}
          </CardContent>
        </Card>
      )}
      <div className="mt-6">
        <ConnectedAppsCard realm="portal" />
      </div>
    </PortalPage>
  );
}
