"use client";

import { useMemo, useState } from "react";
import { toast } from "sonner";

import { AppForm, AppPassword } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  createChangePasswordSchema,
  type ChangePasswordFormValues,
} from "@/features/auth/schemas";
import { isApiError } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";

export function ChangePasswordForm() {
  const { t } = useLocale();
  const { logout } = useAuth();
  const [pending, setPending] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const schema = useMemo(() => createChangePasswordSchema(t), [t]);

  const onSubmit = async (values: ChangePasswordFormValues) => {
    setFormError(null);
    setPending(true);
    try {
      await authService.changePassword({
        current_password: values.current_password,
        new_password: values.new_password,
      });
      toast.success(t("auth.password.success"));
      try {
        await logout();
      } catch {
        // Cookies already cleared by BFF on password change
      }
      window.location.replace(routes.guest.login);
    } catch (error) {
      if (isApiError(error)) {
        setFormError(error.message || t("auth.password.error"));
      } else {
        setFormError(t("auth.password.error"));
      }
      setPending(false);
    }
  };

  return (
    <AppForm
      schema={schema}
      defaultValues={{
        current_password: "",
        new_password: "",
        confirm_password: "",
      }}
      onSubmit={onSubmit}
      className="space-y-4"
    >
      {formError ? (
        <Alert variant="destructive">
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      ) : null}

      <AppPassword
        name="current_password"
        label={t("auth.fields.current_password")}
        autoComplete="current-password"
        placeholder={t("auth.placeholders.password")}
      />
      <AppPassword
        name="new_password"
        label={t("auth.fields.new_password")}
        autoComplete="new-password"
        placeholder={t("auth.placeholders.new_password")}
      />
      <AppPassword
        name="confirm_password"
        label={t("auth.fields.confirm_password")}
        autoComplete="new-password"
        placeholder={t("auth.placeholders.confirm_password")}
      />

      <Button type="submit" disabled={pending}>
        {pending ? t("auth.password.submitting") : t("auth.password.submit")}
      </Button>
    </AppForm>
  );
}
