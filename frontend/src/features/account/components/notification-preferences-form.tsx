"use client";

import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { AppForm, AppSwitch } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  createNotificationPreferencesSchema,
  type NotificationPreferencesFormValues,
} from "@/features/auth/schemas";
import { isApiError } from "@/lib/api";
import {
  getWebPushSupport,
  registerWebPush,
  unregisterWebPush,
  type WebPushSupport,
} from "@/lib/web-push/register";
import { useLocale } from "@/providers/locale-provider";
import {
  notificationPreferencesService,
  type NotificationPreferences,
} from "@/services/notification-preferences.service";

const defaults: NotificationPreferences = {
  email_enabled: true,
  inapp_enabled: true,
  realtime_enabled: true,
  push_enabled: false,
};

export function NotificationPreferencesForm() {
  const { t } = useLocale();
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [values, setValues] = useState<NotificationPreferences>(defaults);
  const [pushSupport, setPushSupport] = useState<WebPushSupport | null>(null);
  const schema = useMemo(() => createNotificationPreferencesSchema(), []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [prefs, support] = await Promise.all([
          notificationPreferencesService.get(),
          getWebPushSupport(),
        ]);
        if (!cancelled) {
          setValues(prefs);
          setPushSupport(support);
        }
      } catch (error) {
        if (!cancelled && isApiError(error)) {
          setFormError(error.message || t("common.error_generic"));
        } else if (!cancelled) {
          setFormError(t("common.error_generic"));
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [t]);

  const pushHint = useMemo(() => {
    if (pushSupport?.supported === false) {
      if (pushSupport.reason === "vapid") {
        return t("auth.preferences.push_unsupported_vapid");
      }
      return t("auth.preferences.push_unsupported_browser");
    }
    return t("auth.preferences.push_hint");
  }, [pushSupport, t]);

  const onSubmit = async (next: NotificationPreferencesFormValues) => {
    setFormError(null);
    setPending(true);

    let payload: NotificationPreferencesFormValues = { ...next };

    try {
      if (next.push_enabled) {
        if (!pushSupport?.supported) {
          payload = { ...next, push_enabled: false };
          toast.error(
            pushSupport?.reason === "vapid"
              ? t("auth.preferences.push_unsupported_vapid")
              : t("auth.preferences.push_unsupported_browser"),
          );
        } else {
          const sub = await registerWebPush();
          if (!sub) {
            payload = { ...next, push_enabled: false };
            const message =
              typeof Notification !== "undefined" &&
              Notification.permission === "denied"
                ? t("auth.preferences.push_permission_denied")
                : t("auth.preferences.push_register_failed");
            toast.error(message);
          }
        }
      } else {
        await unregisterWebPush();
      }

      const saved = await notificationPreferencesService.update(payload);
      setValues(saved);
      toast.success(t("auth.preferences.save_success"));
    } catch (error) {
      if (isApiError(error)) {
        setFormError(error.message || t("common.error_generic"));
      } else {
        setFormError(t("common.error_generic"));
      }
    } finally {
      setPending(false);
    }
  };

  if (loading) {
    return (
      <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
    );
  }

  return (
    <AppForm
      key={JSON.stringify(values)}
      schema={schema}
      defaultValues={values}
      onSubmit={onSubmit}
      className="space-y-4"
    >
      {formError ? (
        <Alert variant="destructive">
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      ) : null}

      <AppSwitch
        name="email_enabled"
        label={t("auth.preferences.email")}
        description={t("auth.preferences.email_hint")}
      />
      <AppSwitch
        name="inapp_enabled"
        label={t("auth.preferences.inapp")}
        description={t("auth.preferences.inapp_hint")}
      />
      <AppSwitch
        name="push_enabled"
        label={t("auth.preferences.push")}
        description={pushHint}
        disabled={pushSupport?.supported === false}
      />

      <Button type="submit" disabled={pending}>
        {pending ? t("auth.preferences.saving") : t("auth.preferences.save")}
      </Button>
    </AppForm>
  );
}
