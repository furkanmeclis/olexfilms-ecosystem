"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { AppForm, AppInput } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
} from "@/components/ui/field";
import { routes } from "@/config/routes";
import {
  createForgotPasswordSchema,
  type ForgotPasswordFormValues,
} from "@/features/auth/schemas";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";

import { AuthCard } from "./auth-card";

export function ForgotPasswordForm() {
  const { t } = useLocale();
  const [sent, setSent] = useState(false);
  const [pending, setPending] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const schema = useMemo(() => createForgotPasswordSchema(t), [t]);

  const onSubmit = async (values: ForgotPasswordFormValues) => {
    setFormError(null);
    setPending(true);
    try {
      await authService.forgotPassword({ email: values.email });
      setSent(true);
      toast.success(t("auth.forgot.success"));
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

  if (sent) {
    return (
      <AuthCard
        title={t("auth.forgot.title")}
        description={t("auth.forgot.success_body")}
      >
        <FieldGroup>
          <FieldDescription>{t("auth.forgot.mailhog_hint")}</FieldDescription>
          <Field>
            <Button asChild>
              <Link href={routes.guest.resetPassword}>
                {t("auth.forgot.go_reset")}
              </Link>
            </Button>
            <FieldDescription className="text-center">
              <Link href={routes.guest.login}>{t("auth.back_to_login")}</Link>
            </FieldDescription>
          </Field>
        </FieldGroup>
      </AuthCard>
    );
  }

  return (
    <AuthCard
      title={t("auth.forgot.title")}
      description={t("auth.forgot.description")}
    >
      <AppForm
        schema={schema}
        defaultValues={{ email: "" }}
        onSubmit={onSubmit}
      >
        <FieldGroup>
          {formError ? (
            <Field data-invalid={true}>
              <FieldError>{formError}</FieldError>
            </Field>
          ) : null}

          <AppInput
            name="email"
            label={t("auth.fields.email")}
            type="email"
            autoComplete="email"
            placeholder={t("auth.placeholders.email")}
          />

          <Field>
            <Button type="submit" disabled={pending}>
              {pending ? t("auth.forgot.submitting") : t("auth.forgot.submit")}
            </Button>
            <FieldDescription className="text-center">
              <Link href={routes.guest.login}>{t("auth.back_to_login")}</Link>
            </FieldDescription>
          </Field>
        </FieldGroup>
      </AppForm>
    </AuthCard>
  );
}
