"use client";

import { useRouter } from "next/navigation";
import { signIn } from "next-auth/react";
import { useMemo, useState } from "react";

import {
  AppForm,
  AppInput,
  AppPassword,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { FieldError, FieldGroup } from "@/components/ui/field";
import { routes } from "@/config/routes";
import { AuthCard } from "@/features/auth/components/auth-card";
import {
  createOrganizationRegisterSchema,
  type OrganizationRegisterFormValues,
} from "@/features/auth/schemas";
import { organizationsService } from "@/features/organizations/services/organizations.service";
import { isApiError } from "@/lib/api";
import {
  CREDENTIAL_ERROR_CODES,
  resolveCredentialErrorCode,
} from "@/lib/auth/credentials-errors";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

export function OrganizationRegisterForm() {
  const { t } = useLocale();
  const router = useRouter();
  const { markAuthenticated, hydrateProfile } = useAuth();
  const [formError, setFormError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const schema = useMemo(() => createOrganizationRegisterSchema(t), [t]);

  const onSubmit = async (values: OrganizationRegisterFormValues) => {
    setFormError(null);
    setPending(true);
    try {
      const result = await organizationsService.register({
        name: values.name,
        surname: values.surname,
        email: values.email,
        password: values.password,
        organization_name: values.organization_name,
        city: values.city,
        district: values.district,
        phone: values.phone,
        address: values.address,
      });
      const signInResult = await signIn("credentials", {
        email: values.email,
        password: values.password,
        organization_slug: result.organization.slug,
        redirect: false,
      });
      if (signInResult?.error) {
        const code = resolveCredentialErrorCode(signInResult);
        if (code === CREDENTIAL_ERROR_CODES.NO_TENANT_MEMBERSHIP) {
          setFormError(t("auth.login.no_tenant_membership"));
        } else if (
          code === CREDENTIAL_ERROR_CODES.ORGANIZATION_ACCESS_EXPIRED
        ) {
          setFormError(t("organizations.access.expired_title"));
        } else {
          setFormError(t("register.error_sign_in"));
        }
        setPending(false);
        return;
      }
      markAuthenticated();
      await hydrateProfile();
      router.replace(routes.tenant.home(result.organization.slug));
    } catch (error) {
      if (isApiError(error)) {
        setFormError(error.message || t("register.error"));
      } else {
        setFormError(t("register.error"));
      }
      setPending(false);
    }
  };

  return (
    <AuthCard
      title={t("register.title")}
      description={t("register.description")}
    >
      <AppForm
        schema={schema}
        defaultValues={{
          name: "",
          surname: "",
          email: "",
          password: "",
          organization_name: "",
          city: "",
          district: "",
          phone: "",
          address: "",
        }}
        onSubmit={onSubmit}
      >
        <FieldGroup>
          {formError ? <FieldError>{formError}</FieldError> : null}
          <div className="grid gap-4 sm:grid-cols-2">
            <AppInput name="name" label={t("register.fields.name")} />
            <AppInput name="surname" label={t("register.fields.surname")} />
          </div>
          <AppInput
            name="organization_name"
            label={t("register.fields.organization_name")}
          />
          <div className="grid gap-4 sm:grid-cols-2">
            <AppInput name="city" label={t("register.fields.city")} />
            <AppInput name="district" label={t("register.fields.district")} />
          </div>
          <AppInput
            name="phone"
            label={t("register.fields.phone")}
            type="tel"
          />
          <AppInput
            name="email"
            label={t("register.fields.email")}
            type="email"
          />
          <AppTextarea
            name="address"
            label={t("register.fields.address")}
            rows={3}
          />
          <AppPassword name="password" label={t("register.fields.password")} />
          <Button type="submit" className="w-full" disabled={pending}>
            {pending ? t("register.submitting") : t("register.submit")}
          </Button>
        </FieldGroup>
      </AppForm>
    </AuthCard>
  );
}
