"use client";

import { useRouter } from "next/navigation";
import { signIn } from "next-auth/react";
import { useMemo, useState } from "react";

import { AppForm, AppInput, AppPassword } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { FieldError, FieldGroup } from "@/components/ui/field";
import { routes } from "@/config/routes";
import { AuthCard } from "@/features/auth/components/auth-card";
import { PublicAuthShell } from "@/features/auth/components/public-auth-shell";
import {
  createTenantLoginSchema,
  type TenantLoginFormValues,
} from "@/features/auth/schemas";
import { useTenant } from "@/features/organizations/providers/tenant-provider";
import {
  CREDENTIAL_ERROR_CODES,
  resolveCredentialErrorCode,
} from "@/lib/auth/credentials-errors";
import { isApiError } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

export function TenantLoginForm() {
  const { t } = useLocale();
  const router = useRouter();
  const { slug, organization } = useTenant();
  const { markAuthenticated, hydrateProfile } = useAuth();
  const [formError, setFormError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const schema = useMemo(() => createTenantLoginSchema(t), [t]);

  const onSubmit = async (values: TenantLoginFormValues) => {
    setFormError(null);
    setPending(true);
    try {
      const result = await signIn("credentials", {
        email: values.email,
        password: values.password,
        organization_slug: slug,
        redirect: false,
      });
      if (result?.error) {
        const code = resolveCredentialErrorCode(result);
        if (code === CREDENTIAL_ERROR_CODES.NO_TENANT_MEMBERSHIP) {
          setFormError(t("auth.login.no_tenant_membership"));
        } else if (
          code === CREDENTIAL_ERROR_CODES.ORGANIZATION_ACCESS_EXPIRED
        ) {
          setFormError(t("organizations.access.expired_title"));
        } else {
          setFormError(t("auth.login.error"));
        }
        setPending(false);
        return;
      }
      markAuthenticated();
      await hydrateProfile();
      router.replace(routes.tenant.home(slug));
    } catch (error) {
      if (isApiError(error)) {
        setFormError(error.message || t("auth.login.error"));
      } else {
        setFormError(t("auth.login.error"));
      }
      setPending(false);
    }
  };

  return (
    <PublicAuthShell>
      <AuthCard
        title={organization?.name ?? t("organizations.login.title")}
        description={t("organizations.login.description")}
      >
        <AppForm
          schema={schema}
          defaultValues={{ email: "", password: "" }}
          onSubmit={onSubmit}
        >
          <FieldGroup>
            {formError ? <FieldError>{formError}</FieldError> : null}
            <AppInput
              name="email"
              label={t("auth.fields.email")}
              type="email"
            />
            <AppPassword name="password" label={t("auth.fields.password")} />
            <Button type="submit" className="w-full" disabled={pending}>
              {pending ? t("auth.login.submitting") : t("auth.login.submit")}
            </Button>
          </FieldGroup>
        </AppForm>
      </AuthCard>
    </PublicAuthShell>
  );
}
