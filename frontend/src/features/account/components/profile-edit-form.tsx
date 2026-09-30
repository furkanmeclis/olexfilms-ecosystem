"use client";

import { useMemo, useState } from "react";
import { toast } from "sonner";

import { AppForm, AppInput } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  createProfileSchema,
  type ProfileFormValues,
} from "@/features/auth/schemas";
import { isApiError } from "@/lib/api";
import { mapMeToAuthUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";

export function ProfileEditForm() {
  const { t } = useLocale();
  const { user, setUser } = useAuth();
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
      </div>

      <Button type="submit" disabled={pending}>
        {pending ? t("auth.profile.saving") : t("auth.profile.save")}
      </Button>
    </AppForm>
  );
}
