"use client";

import type { ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { Fingerprint, Monitor, ShieldCheck } from "lucide-react";
import { toast } from "sonner";

import { PageHeader } from "@/components/layout";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/common/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { ChangePasswordForm } from "@/features/account/components/change-password-form";
import { OAuthIdentityManager } from "@/features/account/components/github-identity-manager";
import { NotificationPreferencesForm } from "@/features/account/components/notification-preferences-form";
import { PasskeyManager } from "@/features/account/components/passkey-manager";
import { SessionManager } from "@/features/account/components/session-manager";
import { TotpManager } from "@/features/account/components/totp-manager";
import { ProfileEditForm } from "@/features/account/components/profile-edit-form";
import { ProfilePermissionsList } from "@/features/account/components/profile-permissions-list";
import { isApiError } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";
import { fetchAppPublicConfig } from "@/services/app-config.service";
import { cn } from "@/lib/utils";

type AccountShell = "platform" | "cms" | "tenant";

function homeHref(shell: AccountShell, tenantSlug?: string) {
  if (shell === "platform") return routes.platform.home;
  if (shell === "tenant" && tenantSlug) return routes.tenant.home(tenantSlug);
  return routes.cms.home;
}

export function AccountProfilePage({
  shell,
  tenantSlug,
}: {
  shell: AccountShell;
  tenantSlug?: string;
}) {
  const { t, locale } = useLocale();
  const { user, hydrateProfile } = useAuth();
  const { data: appConfig } = useQuery({
    queryKey: ["app", "config", locale],
    queryFn: () => fetchAppPublicConfig(locale),
  });

  const roles = user?.roles?.length
    ? user.roles
    : user?.isSuperAdmin
      ? ["super_admin"]
      : [];

  const requestVerify = async () => {
    try {
      await authService.requestEmailVerify(user?.email);
      toast.success(t("auth.verify.request_success"));
    } catch (error) {
      if (isApiError(error)) {
        toast.error(error.message || t("common.error_generic"));
      } else {
        toast.error(t("common.error_generic"));
      }
    } finally {
      await hydrateProfile();
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("auth.profile.title")}
        description={t("auth.profile.description")}
        breadcrumbs={[
          {
            label: t("layout.breadcrumb_home"),
            href: homeHref(shell, tenantSlug),
          },
          { label: t("auth.profile") },
        ]}
      />

      <Card className="shadow-none">
        <CardHeader>
          <CardTitle>{t("auth.profile.title")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <dl className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            <Fact
              label={t("auth.profile.name")}
              value={user?.fullName || "—"}
            />
            <Fact label={t("auth.profile.email")} value={user?.email || "—"} />
            <Fact
              label={t("auth.profile.email_verified")}
              value={
                user?.emailVerified ? (
                  <Badge variant="success">{t("auth.profile.verified")}</Badge>
                ) : (
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="warning">
                      {t("auth.profile.unverified")}
                    </Badge>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={requestVerify}
                    >
                      {t("auth.verify.request")}
                    </Button>
                  </div>
                )
              }
            />
            <Fact
              label={t("auth.profile.status")}
              value={
                user?.status ? (
                  <Badge variant="secondary">{user.status}</Badge>
                ) : (
                  "—"
                )
              }
            />
            <Fact
              label={t("auth.profile.roles_title")}
              value={
                roles.length ? (
                  <div className="flex flex-wrap gap-1.5">
                    {roles.map((role) => (
                      <Badge key={role} variant="secondary">
                        {role}
                      </Badge>
                    ))}
                  </div>
                ) : (
                  "—"
                )
              }
            />
            <Fact
              label={t("auth.profile.uuid")}
              value={
                <span className="font-mono text-xs">{user?.uuid ?? "—"}</span>
              }
            />
          </dl>

          <div className="border-border space-y-3 border-t pt-5">
            <div>
              <h3 className="text-sm font-semibold">
                {t("auth.profile.edit_title")}
              </h3>
              <CardHint>{t("auth.profile.edit_hint")}</CardHint>
            </div>
            <ProfileEditForm />
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-2 lg:items-start xl:grid-cols-3">
        <Card className="shadow-none">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Fingerprint
                className="text-muted-foreground size-5"
                aria-hidden
              />
              {t("auth.passkey.title")}
            </CardTitle>
            <CardHint>{t("auth.passkey.description")}</CardHint>
          </CardHeader>
          <CardContent>
            <PasskeyManager />
          </CardContent>
        </Card>

        <Card className="shadow-none">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <ShieldCheck
                className="text-muted-foreground size-5"
                aria-hidden
              />
              {t("auth.totp.title")}
            </CardTitle>
            <CardHint>{t("auth.totp.description")}</CardHint>
          </CardHeader>
          <CardContent>
            <TotpManager />
          </CardContent>
        </Card>

        <Card className="shadow-none">
          <CardHeader>
            <CardTitle>{t("auth.oauth.title")}</CardTitle>
            <CardHint>{t("auth.oauth.profile_hint")}</CardHint>
          </CardHeader>
          <CardContent>
            <OAuthIdentityManager
              providers={[
                {
                  id: "github",
                  enabled: Boolean(appConfig?.auth_methods.github.login),
                },
                {
                  id: "google",
                  enabled: Boolean(appConfig?.auth_methods.google.login),
                },
                {
                  id: "facebook",
                  enabled: Boolean(appConfig?.auth_methods.facebook.login),
                },
                {
                  id: "apple",
                  enabled: Boolean(appConfig?.auth_methods.apple.login),
                },
              ]}
            />
          </CardContent>
        </Card>

        <Card className="shadow-none">
          <CardHeader>
            <CardTitle>{t("auth.password.title")}</CardTitle>
            <CardHint>{t("auth.profile.change_password_body")}</CardHint>
          </CardHeader>
          <CardContent>
            <ChangePasswordForm />
          </CardContent>
        </Card>
      </div>

      <Card className="shadow-none">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Monitor className="text-muted-foreground size-5" aria-hidden />
            {t("auth.sessions.title")}
          </CardTitle>
          <CardHint>{t("auth.sessions.description")}</CardHint>
        </CardHeader>
        <CardContent>
          <SessionManager />
        </CardContent>
      </Card>

      <Card className="shadow-none">
        <CardHeader>
          <CardTitle>{t("auth.preferences.title")}</CardTitle>
          <CardHint>{t("auth.preferences.description")}</CardHint>
        </CardHeader>
        <CardContent>
          <NotificationPreferencesForm />
        </CardContent>
      </Card>

      <Card className="shadow-none">
        <CardHeader>
          <CardTitle>{t("auth.profile.permissions_title")}</CardTitle>
          <CardHint>{t("auth.profile.permissions_hint")}</CardHint>
        </CardHeader>
        <CardContent>
          <ProfilePermissionsList granted={user?.permissions} />
        </CardContent>
      </Card>
    </div>
  );
}

function CardHint({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <p className={cn("text-muted-foreground text-sm", className)}>{children}</p>
  );
}

function Fact({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="min-w-0 space-y-1">
      <dt className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
        {label}
      </dt>
      <dd className="text-sm font-medium break-all">{value}</dd>
    </div>
  );
}
