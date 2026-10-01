"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { signIn, signOut, useSession } from "next-auth/react";
import { signIn as signInPasskey } from "next-auth/webauthn";
import { useQuery } from "@tanstack/react-query";
import { Fingerprint, ShieldCheck } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

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
  createLoginSchema,
  type LoginFormValues,
} from "@/features/auth/schemas";
import { parseTenantSlugFromPath } from "@/lib/routing/tenant";
import {
  CREDENTIAL_ERROR_CODES,
  resolveCredentialErrorCode,
  type CredentialSignInResult,
} from "@/lib/auth/credentials-errors";
import { defaultHomeForUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { fetchAppPublicConfig } from "@/services/app-config.service";
import { z } from "zod";

import { AuthCard } from "./auth-card";

function resolveNext(raw: string | null, fallback: string) {
  // Only same-origin paths. Browsers treat a backslash like "/", so a value
  // starting "/<backslash>evil.com" would become a protocol-relative redirect.
  if (
    !raw ||
    !raw.startsWith("/") ||
    raw.startsWith("//") ||
    /[\\\u0000-\u001f]/.test(raw)
  ) {
    return fallback;
  }
  return raw;
}

type OAuthId = "github" | "google" | "facebook" | "apple";

type SavedCredentials = {
  email: string;
  password: string;
};

function createMfaSchema(t: (key: string) => string) {
  return z.object({
    totp_code: z.string().min(1, t("auth.totp.validation.required")),
  });
}

export function LoginForm() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const searchParams = useSearchParams();
  const { markAuthenticated, hydrateProfile } = useAuth();
  const { status: sessionStatus, update: updateSession } = useSession();
  const [formError, setFormError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [passkeyPending, setPasskeyPending] = useState(false);
  const [oauthPending, setOauthPending] = useState<OAuthId | null>(null);
  const [mfaStep, setMfaStep] = useState(false);
  const [savedCredentials, setSavedCredentials] =
    useState<SavedCredentials | null>(null);
  const schema = useMemo(() => createLoginSchema(t), [t]);
  const mfaSchema = useMemo(() => createMfaSchema(t), [t]);

  const { data: appConfig } = useQuery({
    queryKey: ["app", "config", locale],
    queryFn: () => fetchAppPublicConfig(locale),
  });

  const methods = appConfig?.auth_methods;
  const showPasskey = methods?.passkey.login !== false;
  const showPassword = methods?.password.login !== false;
  const showRegisterLink =
    Boolean(appConfig?.registration_enabled) &&
    (methods?.password.register ||
      methods?.github.register ||
      methods?.google.register ||
      methods?.facebook.register ||
      methods?.apple.register);

  const oauthError = searchParams.get("error");
  const oauthFormError =
    oauthError === "GitHubNotLinked" ||
    oauthError === "OAuthNotLinked" ||
    oauthError === "OAuthAccountNotLinked" ||
    oauthError === "AccountNotLinked"
      ? t("auth.oauth.not_linked")
      : oauthError === "Configuration" ||
          oauthError === "AccessDenied" ||
          oauthError === "AdapterError" ||
          oauthError === "CallbackRouteError"
        ? t("auth.oauth.sign_in_error")
        : null;
  const displayError = formError ?? oauthFormError;

  const finishLogin = async () => {
    await updateSession();
    markAuthenticated();
    const user = await hydrateProfile();
    if (!user) {
      setFormError(t("auth.login.error"));
      return false;
    }
    const home = defaultHomeForUser(user);
    if (home === routes.errors.forbidden) {
      setFormError(t("errors.role_mismatch"));
      return false;
    }
    const next = resolveNext(searchParams.get("next"), home);
    router.replace(next);
    return true;
  };

  const resolveCredentialsError = (code: string | null) => {
    if (!code) return t("auth.login.error");
    if (code.includes("RATE_LIMITED")) {
      return t("auth.login.rate_limited");
    }
    if (code.includes("NO_TENANT_MEMBERSHIP")) {
      return t("auth.login.no_tenant_membership");
    }
    if (code === CREDENTIAL_ERROR_CODES.NO_PANEL_ACCESS) {
      return t("auth.login.no_panel_access");
    }
    if (code === CREDENTIAL_ERROR_CODES.MFA_REQUIRED) {
      return t("auth.totp.login_required");
    }
    if (code === CREDENTIAL_ERROR_CODES.INVALID_MFA_CODE) {
      return t("auth.totp.login_invalid");
    }
    if (code === CREDENTIAL_ERROR_CODES.MFA_NOT_ENROLLED) {
      return t("auth.totp.login_not_enrolled");
    }
    return t("auth.login.error");
  };

  const signInWithCredentials = async (
    email: string,
    password: string,
    totpCode?: string,
  ) => {
    const organizationSlug =
      parseTenantSlugFromPath(searchParams.get("next")) ?? undefined;
    const result = (await signIn("credentials", {
      email,
      password,
      totp_code: totpCode ?? "",
      ...(organizationSlug ? { organization_slug: organizationSlug } : {}),
      redirect: false,
    })) as CredentialSignInResult | undefined;
    const errorCode = result ? resolveCredentialErrorCode(result) : null;
    if (errorCode) {
      if (errorCode === CREDENTIAL_ERROR_CODES.MFA_REQUIRED) {
        setSavedCredentials({ email, password });
        setMfaStep(true);
        setFormError(null);
        return "mfa";
      }
      setFormError(resolveCredentialsError(errorCode));
      return false;
    }
    if (!result?.ok) {
      setFormError(t("auth.login.error"));
      return false;
    }
    const ok = await finishLogin();
    return ok;
  };

  const onSubmit = async (values: LoginFormValues) => {
    setFormError(null);
    setPending(true);
    try {
      const outcome = await signInWithCredentials(
        values.email,
        values.password,
      );
      if (outcome !== true && outcome !== "mfa") setPending(false);
      else if (outcome === true) setPending(false);
    } catch {
      setFormError(t("auth.login.error"));
      setPending(false);
    }
  };

  const onMfaSubmit = async (values: z.infer<typeof mfaSchema>) => {
    if (!savedCredentials) return;
    setFormError(null);
    setPending(true);
    try {
      const outcome = await signInWithCredentials(
        savedCredentials.email,
        savedCredentials.password,
        values.totp_code.trim(),
      );
      if (outcome === true) {
        setMfaStep(false);
        setSavedCredentials(null);
      }
      setPending(false);
    } catch {
      setFormError(t("auth.totp.login_invalid"));
      setPending(false);
    }
  };

  const cancelMfa = () => {
    setMfaStep(false);
    setSavedCredentials(null);
    setFormError(null);
  };

  const onPasskeySignIn = async () => {
    setFormError(null);
    setPasskeyPending(true);
    try {
      if (sessionStatus === "authenticated") {
        await signOut({ redirect: false });
      }
      const result = await signInPasskey("passkey", {
        action: "authenticate",
        redirect: false,
      });
      if (!result || result.error) {
        if (result?.error?.includes("NO_TENANT_MEMBERSHIP")) {
          setFormError(t("auth.login.no_tenant_membership"));
        } else {
          setFormError(t("auth.passkey.error"));
        }
        setPasskeyPending(false);
        return;
      }
      const ok = await finishLogin();
      if (!ok) setPasskeyPending(false);
    } catch {
      setFormError(t("auth.passkey.error"));
      setPasskeyPending(false);
    }
  };

  const onOAuthSignIn = async (provider: OAuthId) => {
    setFormError(null);
    setOauthPending(provider);
    try {
      if (sessionStatus === "authenticated") {
        await signOut({ redirect: false });
      }
      await signIn(provider, {
        redirect: true,
        callbackUrl: resolveNext(
          searchParams.get("next"),
          routes.platform.home,
        ),
      });
    } catch {
      setFormError(t("auth.oauth.sign_in_error"));
      setOauthPending(null);
    }
  };

  const authPending = pending || passkeyPending || oauthPending !== null;

  const oauthButtons: {
    id: OAuthId;
    enabled: boolean;
    icon: ReactNode;
    label: string;
    pendingLabel: string;
  }[] = [
    {
      id: "github",
      enabled: Boolean(methods?.github.login),
      icon: <GitHubIcon className="size-4" />,
      label: t("auth.github.sign_in"),
      pendingLabel: t("auth.github.signing_in"),
    },
    {
      id: "google",
      enabled: Boolean(methods?.google.login),
      icon: <GoogleIcon className="size-4" />,
      label: t("auth.google.sign_in"),
      pendingLabel: t("auth.google.signing_in"),
    },
    {
      id: "facebook",
      enabled: Boolean(methods?.facebook.login),
      icon: <FacebookIcon className="size-4" />,
      label: t("auth.facebook.sign_in"),
      pendingLabel: t("auth.facebook.signing_in"),
    },
    {
      id: "apple",
      enabled: Boolean(methods?.apple.login),
      icon: <AppleIcon className="size-4" />,
      label: t("auth.apple.sign_in"),
      pendingLabel: t("auth.apple.signing_in"),
    },
  ];

  if (mfaStep) {
    return (
      <AuthCard
        title={t("auth.totp.login_title")}
        description={t("auth.totp.login_description")}
      >
        <AppForm
          schema={mfaSchema}
          defaultValues={{ totp_code: "" }}
          onSubmit={onMfaSubmit}
        >
          <FieldGroup>
            {displayError ? (
              <Field data-invalid={true}>
                <FieldError>{displayError}</FieldError>
              </Field>
            ) : null}

            <AppInput
              name="totp_code"
              label={t("auth.totp.confirm_code")}
              autoComplete="one-time-code"
              inputMode="numeric"
              placeholder={t("auth.totp.code_placeholder")}
            />

            <Field className="flex flex-col gap-2 sm:flex-row">
              <Button
                type="submit"
                disabled={authPending}
                className="sm:flex-1"
              >
                <ShieldCheck aria-hidden />
                {pending
                  ? t("auth.login.submitting")
                  : t("auth.totp.verify_login")}
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={authPending}
                onClick={cancelMfa}
              >
                {t("common.back")}
              </Button>
            </Field>
          </FieldGroup>
        </AppForm>
      </AuthCard>
    );
  }

  return (
    <AuthCard
      title={t("auth.login.title")}
      description={t("auth.login.description")}
    >
      <div className="space-y-4">
        {showPasskey ? (
          <Button
            type="button"
            variant="outline"
            className="w-full"
            disabled={authPending}
            onClick={onPasskeySignIn}
          >
            <Fingerprint aria-hidden />
            {passkeyPending
              ? t("auth.passkey.signing_in")
              : t("auth.passkey.sign_in")}
          </Button>
        ) : null}

        {oauthButtons
          .filter((button) => button.enabled)
          .map((button) => (
            <Button
              key={button.id}
              type="button"
              variant="outline"
              className="w-full"
              disabled={authPending}
              onClick={() => onOAuthSignIn(button.id)}
            >
              {button.icon}
              {oauthPending === button.id ? button.pendingLabel : button.label}
            </Button>
          ))}

        {showPassword ? (
          <>
            {(showPasskey || oauthButtons.some((button) => button.enabled)) && (
              <div className="flex items-center gap-3">
                <Separator className="flex-1" />
                <span className="text-muted-foreground text-xs uppercase">
                  {t("auth.passkey.or_password")}
                </span>
                <Separator className="flex-1" />
              </div>
            )}

            <AppForm
              schema={schema}
              defaultValues={{ email: "", password: "" }}
              onSubmit={onSubmit}
            >
              <FieldGroup>
                {displayError ? (
                  <Field data-invalid={true}>
                    <FieldError>{displayError}</FieldError>
                  </Field>
                ) : null}

                <AppInput
                  name="email"
                  label={t("auth.login.email")}
                  type="email"
                  autoComplete="email"
                  placeholder={t("auth.placeholders.email")}
                />
                <AppPassword
                  name="password"
                  label={t("auth.login.password")}
                  autoComplete="current-password"
                  placeholder={t("auth.placeholders.password")}
                  labelAction={
                    <Link
                      href={routes.guest.forgotPassword}
                      className="ml-auto text-sm underline-offset-4 hover:underline"
                    >
                      {t("auth.login.forgot_link")}
                    </Link>
                  }
                />

                <Field>
                  <Button type="submit" disabled={authPending}>
                    {pending
                      ? t("auth.login.submitting")
                      : t("auth.login.submit")}
                  </Button>
                </Field>
              </FieldGroup>
            </AppForm>
          </>
        ) : displayError ? (
          <Field data-invalid={true}>
            <FieldError>{displayError}</FieldError>
          </Field>
        ) : null}

        {showRegisterLink ? (
          <p className="text-muted-foreground text-center text-sm">
            {t("auth.login.no_account")}{" "}
            <Link
              href={routes.guest.register}
              className="underline-offset-4 hover:underline"
            >
              {t("auth.login.register_link")}
            </Link>
          </p>
        ) : null}
      </div>
    </AuthCard>
  );
}
