"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { signIn, signOut, useSession } from "next-auth/react";
import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import { AppleIcon } from "@/components/icons/apple-icon";
import { FacebookIcon } from "@/components/icons/facebook-icon";
import { GitHubIcon } from "@/components/icons/github-icon";
import { GoogleIcon } from "@/components/icons/google-icon";
import { AppForm, AppInput, AppPassword } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Field, FieldError, FieldGroup } from "@/components/ui/field";
import { Separator } from "@/components/ui/separator";
import { routes } from "@/config/routes";
import {
  createRegisterSchema,
  type RegisterFormValues,
} from "@/features/auth/schemas";
import { isApiError } from "@/lib/api";
import { defaultHomeForUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";
import { fetchAppPublicConfig } from "@/services/app-config.service";

import { AuthCard } from "./auth-card";

type OAuthId = "github" | "google" | "facebook" | "apple";

export function RegisterForm() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const { markAuthenticated, hydrateProfile } = useAuth();
  const { status: sessionStatus } = useSession();
  const [formError, setFormError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [oauthPending, setOauthPending] = useState<OAuthId | null>(null);
  const schema = useMemo(() => createRegisterSchema(t), [t]);

  const { data: appConfig, isLoading } = useQuery({
    queryKey: ["app", "config", locale],
    queryFn: () => fetchAppPublicConfig(locale),
  });

  const methods = appConfig?.auth_methods;
  const passwordRegister = Boolean(methods?.password.register);
  const oauthRegister = [
    {
      id: "github" as const,
      enabled: Boolean(methods?.github.register),
      icon: <GitHubIcon className="size-4" />,
      label: t("auth.github.sign_up"),
      pendingLabel: t("auth.github.signing_in"),
    },
    {
      id: "google" as const,
      enabled: Boolean(methods?.google.register),
      icon: <GoogleIcon className="size-4" />,
      label: t("auth.google.sign_up"),
      pendingLabel: t("auth.google.signing_in"),
    },
    {
      id: "facebook" as const,
      enabled: Boolean(methods?.facebook.register),
      icon: <FacebookIcon className="size-4" />,
      label: t("auth.facebook.sign_up"),
      pendingLabel: t("auth.facebook.signing_in"),
    },
    {
      id: "apple" as const,
      enabled: Boolean(methods?.apple.register),
      icon: <AppleIcon className="size-4" />,
      label: t("auth.apple.sign_up"),
      pendingLabel: t("auth.apple.signing_in"),
    },
  ];

  if (!isLoading && !appConfig?.registration_enabled) {
    return (
      <AuthCard
        title={t("auth.register.disabled_title")}
        description={t("auth.register.disabled_description")}
      >
        <Button asChild className="w-full">
          <Link href={routes.guest.login}>{t("auth.back_to_login")}</Link>
        </Button>
      </AuthCard>
    );
  }

  const onSubmit = async (values: RegisterFormValues) => {
    setFormError(null);
    setPending(true);
    try {
      await authService.register({
        email: values.email,
        password: values.password,
        name: values.name,
        surname: values.surname,
      });
      const result = await signIn("credentials", {
        email: values.email,
        password: values.password,
        redirect: false,
      });
      if (result?.error) {
        router.replace(routes.guest.login);
        return;
      }
      markAuthenticated();
      const user = await hydrateProfile();
      router.replace(user ? defaultHomeForUser(user) : routes.guest.login);
    } catch (error) {
      if (isApiError(error)) {
        setFormError(error.message || t("auth.register.error"));
      } else {
        setFormError(t("auth.register.error"));
      }
      setPending(false);
    }
  };

  const onOAuth = async (provider: OAuthId) => {
    setFormError(null);
    setOauthPending(provider);
    try {
      if (sessionStatus === "authenticated") {
        await signOut({ redirect: false });
      }
      await signIn(provider, {
        redirect: true,
        callbackUrl: routes.platform.home,
      });
    } catch {
      setFormError(t("auth.oauth.sign_in_error"));
      setOauthPending(null);
    }
  };

  const authPending = pending || oauthPending !== null;

  return (
    <AuthCard
      title={t("auth.register.title")}
      description={t("auth.register.description")}
    >
      <div className="space-y-4">
        {oauthRegister
          .filter((button) => button.enabled)
          .map((button) => (
            <Button
              key={button.id}
              type="button"
              variant="outline"
              className="w-full"
              disabled={authPending}
              onClick={() => onOAuth(button.id)}
            >
              {button.icon}
              {oauthPending === button.id ? button.pendingLabel : button.label}
            </Button>
          ))}

        {passwordRegister ? (
          <>
            {oauthRegister.some((button) => button.enabled) ? (
              <div className="flex items-center gap-3">
                <Separator className="flex-1" />
                <span className="text-muted-foreground text-xs uppercase">
                  {t("auth.register.or_email")}
                </span>
                <Separator className="flex-1" />
              </div>
            ) : null}

            <AppForm
              schema={schema}
              defaultValues={{
                email: "",
                name: "",
                surname: "",
                password: "",
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
                  name="name"
                  label={t("auth.register.name")}
                  autoComplete="given-name"
                  placeholder={t("auth.placeholders.name")}
                />
                <AppInput
                  name="surname"
                  label={t("auth.register.surname")}
                  autoComplete="family-name"
                  placeholder={t("auth.placeholders.surname")}
                />
                <AppInput
                  name="email"
                  label={t("auth.register.email")}
                  type="email"
                  autoComplete="email"
                  placeholder={t("auth.placeholders.email")}
                />
                <AppPassword
                  name="password"
                  label={t("auth.register.password")}
                  autoComplete="new-password"
                  placeholder={t("auth.placeholders.new_password")}
                />
                <Field>
                  <Button type="submit" disabled={authPending}>
                    {pending
                      ? t("auth.register.submitting")
                      : t("auth.register.submit")}
                  </Button>
                </Field>
              </FieldGroup>
            </AppForm>
          </>
        ) : formError ? (
          <Field data-invalid={true}>
            <FieldError>{formError}</FieldError>
          </Field>
        ) : null}

        <p className="text-muted-foreground text-center text-sm">
          {t("auth.register.have_account")}{" "}
          <Link
            href={routes.guest.login}
            className="underline-offset-4 hover:underline"
          >
            {t("auth.register.login_link")}
          </Link>
        </p>
      </div>
    </AuthCard>
  );
}
