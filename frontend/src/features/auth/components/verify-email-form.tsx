"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
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
  createVerifyEmailSchema,
  type VerifyEmailFormValues,
} from "@/features/auth/schemas";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";

import { AuthCard } from "./auth-card";

export function VerifyEmailForm() {
  const { t } = useLocale();
  const router = useRouter();
  const searchParams = useSearchParams();
  const [pending, setPending] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const schema = useMemo(() => createVerifyEmailSchema(t), [t]);

  const onSubmit = async (values: VerifyEmailFormValues) => {
    setFormError(null);
    setPending(true);
    try {
      await authService.verifyEmail({
        email: values.email,
        code: values.code,
      });
      toast.success(t("auth.verify.success"));
      router.replace(routes.guest.login);
    } catch (error) {
      if (isApiError(error)) {
        setFormError(error.message || t("auth.verify.error"));
      } else {
        setFormError(t("auth.verify.error"));
      }
    } finally {
      setPending(false);
    }
  };

  return (
    <AuthCard
      title={t("auth.verify.title")}
      description={t("auth.verify.description")}
    >
      <AppForm
        schema={schema}
        defaultValues={{
          email: searchParams.get("email") ?? "",
          code: searchParams.get("code") ?? "",
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

          <Field>
            <Button type="submit" disabled={pending}>
              {pending ? t("auth.verify.submitting") : t("auth.verify.submit")}
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
