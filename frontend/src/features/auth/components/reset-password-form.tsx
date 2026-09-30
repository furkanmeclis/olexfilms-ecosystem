"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { AppForm, AppInput, AppPassword } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
} from "@/components/ui/field";
import { routes } from "@/config/routes";
import {
  createResetPasswordSchema,
  type ResetPasswordFormValues,
} from "@/features/auth/schemas";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";

import { AuthCard } from "./auth-card";

export function ResetPasswordForm() {
  const { t } = useLocale();
  const router = useRouter();
  const searchParams = useSearchParams();
  const [pending, setPending] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const schema = useMemo(() => createResetPasswordSchema(t), [t]);

  const onSubmit = async (values: ResetPasswordFormValues) => {
    setFormError(null);
    setPending(true);
    try {
      await authService.resetPassword({
        email: values.email,
        code: values.code,
        password: values.password,
      });
      toast.success(t("auth.reset.success"));
      router.replace(routes.guest.login);
    } catch (error) {
      if (isApiError(error)) {
        setFormError(error.message || t("auth.reset.error"));
      } else {
        setFormError(t("auth.reset.error"));
      }
    } finally {
      setPending(false);
    }
  };

  return (
    <AuthCard
      title={t("auth.reset.title")}
      description={t("auth.reset.description")}
    >
      <AppForm
        schema={schema}
        defaultValues={{
          email: searchParams.get("email") ?? "",
          code: searchParams.get("code") ?? "",
          password: "",
          confirm_password: "",
        }}
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
          <AppInput
            name="code"
            label={t("auth.fields.code")}
            autoComplete="one-time-code"
            placeholder={t("auth.placeholders.code")}
          />
          <AppPassword
            name="password"
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

          <Field>
            <Button type="submit" disabled={pending}>
              {pending ? t("auth.reset.submitting") : t("auth.reset.submit")}
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
