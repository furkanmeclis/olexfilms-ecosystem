"use client";

import { useMemo, useState } from "react";
import { toast } from "sonner";

import { AppCombobox, AppForm, AppInput, AppSelect } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { LOCALE_NAMES, SUPPORTED_LOCALES } from "@/config/i18n";
import {
  PROFILE_INHERIT,
  createProfileSchema,
  type ProfileFormValues,
} from "@/features/auth/schemas";
import { useTimeZoneOptions } from "@/hooks/use-time-zone-options";
import { isApiError } from "@/lib/api";
import { mapMeToAuthUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";

export function ProfileEditForm() {
  const { t, timeZone } = useLocale();
  const { user, setUser } = useAuth();
  const timeZoneOptions = useTimeZoneOptions(user?.ownTimeZone);
  const localeOptions = useMemo(
    () => [
      { value: PROFILE_INHERIT, label: t("auth.profile.inherit") },
      ...SUPPORTED_LOCALES.map((value) => ({
        value,
        label: LOCALE_NAMES[value],
      })),
    ],
    [t],
  );
  const [pending, setPending] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const schema = useMemo(() => createProfileSchema(t), [t]);

  if (!user) return null;

  const onSubmit = async (values: ProfileFormValues) => {
    setFormError(null);
    setPending(true);
    try {
      const me = await authService.updateProfile({
        name: values.name,
        surname: values.surname,
        // Empty string clears the field: inherit from the organization.
        locale: values.locale === PROFILE_INHERIT ? "" : values.locale,
        timezone: values.timezone,
      });
      setUser(mapMeToAuthUser(me));
      toast.success(t("auth.profile.save_success"));
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

  return (
    <AppForm
      schema={schema}
      defaultValues={{
        name: user.name,
        surname: user.surname,
        locale: user.ownLocale ?? PROFILE_INHERIT,
        timezone: user.ownTimeZone ?? "",
      }}
      onSubmit={onSubmit}
      className="space-y-4"
    >
      {formError ? (
        <Alert variant="destructive">
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      ) : null}

      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput
          name="name"
          label={t("auth.profile.first_name")}
          autoComplete="given-name"
        />
        <AppInput
          name="surname"
          label={t("auth.profile.last_name")}
          autoComplete="family-name"
        />
        <AppSelect
          name="locale"
          label={t("auth.profile.locale")}
          description={t("auth.profile.locale_hint")}
          options={localeOptions}
        />
        <AppCombobox
          name="timezone"
          label={t("auth.profile.timezone")}
          description={t("auth.profile.timezone_hint", { zone: timeZone })}
          options={timeZoneOptions}
          placeholder={t("auth.profile.inherit")}
          searchPlaceholder={t("auth.profile.timezone_search")}
          emptyText={t("auth.profile.timezone_empty")}
          clearable
        />
      </div>

      <Button type="submit" disabled={pending}>
        {pending ? t("auth.profile.saving") : t("auth.profile.save")}
      </Button>
    </AppForm>
  );
}
