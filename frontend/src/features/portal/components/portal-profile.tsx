"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BellRing,
  ChevronRight,
  FileSignature,
  KeyRound,
  LogOut,
  Monitor,
  ShieldCheck,
  UserRound,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { LOCALE_NAMES, SUPPORTED_LOCALES, type AppLocale } from "@/config/i18n";
import { routes } from "@/config/routes";
import { ChangePasswordForm } from "@/features/account/components/change-password-form";
import { SessionManager } from "@/features/account/components/session-manager";
import { TotpManager } from "@/features/account/components/totp-manager";
import { PortalPage } from "@/features/portal/components/portal-page";
import {
  portalAccountApi,
  portalApi,
  portalSignOut,
  portalTotpApi,
} from "@/features/portal/lib/portal-client";
import {
  isPortalReadOnly,
  PORTAL_ME_KEY,
} from "@/features/portal/lib/portal-vehicles";
import { useLocale } from "@/providers/locale-provider";

function isAppLocale(value: string | null | undefined): value is AppLocale {
  return Boolean(
    value && (SUPPORTED_LOCALES as readonly string[]).includes(value),
  );
}

/**
 * Portal > My profile (TEC-771): the customer's / fleet user's account in
 * one place, like the panel profile: name and language (PATCH
 * /v1/auth/profile), signed-in devices, password and 2FA for fleet
 * accounts (customers sign in with a phone code), links to notification
 * preferences and contracts, and sign out.
 */
export function PortalProfile() {
  const { t } = useLocale();
  const router = useRouter();
  const me = useQuery({
    queryKey: PORTAL_ME_KEY,
    queryFn: () => portalApi.me(),
    staleTime: 5 * 60_000,
  });
  const fleet = isPortalReadOnly(me.data?.roles);

  const signOut = async () => {
    // Portal only: the panel session (if any) stays signed in.
    await portalSignOut();
    router.replace(routes.portal.login);
    router.refresh();
  };

  return (
    <PortalPage
      title={t("portal.profile.title")}
      icon={<UserRound className="size-6" />}
      testId="portal-profile-page"
      actions={
        <Button
          variant="outline"
          size="sm"
          onClick={() => void signOut()}
          data-testid="portal-profile-logout"
        >
          <LogOut className="size-4 rtl:rotate-180" />
          {t("portal.home.logout")}
        </Button>
      }
    >
      {me.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void me.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : !me.data ? (
        <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
      ) : (
        <ProfileForm
          key={me.data.user.name ?? ""}
          name={me.data.user.name ?? ""}
          surname={me.data.user.surname ?? ""}
          email={me.data.user.email ?? null}
          locale={me.data.user.locale ?? null}
        />
      )}

      <Section
        icon={<BellRing className="size-5" />}
        title={t("portal.profile.links_title")}
      >
        <ul className="-mx-2">
          <LinkRow
            href={routes.portal.preferences}
            icon={<BellRing className="size-4" />}
            label={t("portal.nav.preferences")}
            hint={t("portal.profile.preferences_hint")}
          />
          {!fleet ? (
            <LinkRow
              href={routes.portal.contracts}
              icon={<FileSignature className="size-4" />}
              label={t("portal.nav.contracts")}
              hint={t("portal.profile.contracts_hint")}
            />
          ) : null}
        </ul>
      </Section>

      {fleet ? (
        <>
          <Section
            icon={<KeyRound className="size-5" />}
            title={t("auth.password.title")}
            hint={t("auth.profile.change_password_body")}
          >
            <ChangePasswordForm
              change={portalAccountApi.changePassword}
              afterChange={signOut}
            />
          </Section>
          <Section
            icon={<ShieldCheck className="size-5" />}
            title={t("auth.totp.title")}
            hint={t("auth.totp.description")}
            testId="portal-totp-card"
          >
            <TotpManager api={portalTotpApi} queryKey={["portal", "totp"]} />
          </Section>
        </>
      ) : (
        <p className="text-muted-foreground px-1 text-sm">
          {t("portal.profile.phone_login_hint")}
        </p>
      )}

      <Section
        icon={<Monitor className="size-5" />}
        title={t("auth.sessions.title")}
      >
        <SessionManager
          api={portalAccountApi}
          queryKey={["portal", "sessions"]}
        />
      </Section>
    </PortalPage>
  );
}

function ProfileForm({
  name: initialName,
  surname: initialSurname,
  email,
  locale: ownLocale,
}: {
  name: string;
  surname: string;
  email: string | null;
  locale: string | null;
}) {
  const { t, locale: current, setLocale } = useLocale();
  const qc = useQueryClient();
  const [name, setName] = useState(initialName);
  const [surname, setSurname] = useState(initialSurname);
  const [lang, setLang] = useState<AppLocale>(
    isAppLocale(ownLocale) ? ownLocale : current,
  );
  const save = useMutation({
    mutationFn: () =>
      portalApi.updateProfile({
        name: name.trim(),
        surname: surname.trim(),
        locale: lang,
      }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: PORTAL_ME_KEY });
      if (lang !== current) await setLocale(lang);
      toast.success(t("portal.profile.saved"));
    },
    onError: () => toast.error(t("portal.profile.failed")),
  });

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!name.trim()) return;
    save.mutate();
  };

  return (
    <Card>
      <CardContent>
        <form
          onSubmit={onSubmit}
          className="space-y-4"
          data-testid="portal-profile-form"
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="portal-profile-name">
                {t("auth.profile.first_name")}
              </Label>
              <Input
                id="portal-profile-name"
                value={name}
                autoComplete="given-name"
                required
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="portal-profile-surname">
                {t("auth.profile.last_name")}
              </Label>
              <Input
                id="portal-profile-surname"
                value={surname}
                autoComplete="family-name"
                onChange={(e) => setSurname(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="portal-profile-locale">
                {t("auth.profile.locale")}
              </Label>
              <Select
                value={lang}
                onValueChange={(value) => {
                  if (isAppLocale(value)) setLang(value);
                }}
              >
                <SelectTrigger id="portal-profile-locale" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SUPPORTED_LOCALES.map((value) => (
                    <SelectItem key={value} value={value}>
                      {LOCALE_NAMES[value]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {email ? (
              <div className="space-y-2">
                <Label htmlFor="portal-profile-email">
                  {t("auth.profile.email")}
                </Label>
                <Input id="portal-profile-email" value={email} readOnly />
              </div>
            ) : null}
          </div>
          <Button type="submit" disabled={save.isPending || !name.trim()}>
            {save.isPending ? t("auth.profile.saving") : t("auth.profile.save")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

function Section({
  icon,
  title,
  hint,
  testId,
  children,
}: {
  icon: ReactNode;
  title: string;
  hint?: string;
  testId?: string;
  children: ReactNode;
}) {
  return (
    <Card data-testid={testId}>
      <CardContent className="space-y-3">
        <div className="flex items-center gap-2 font-medium">
          <span className="text-muted-foreground" aria-hidden>
            {icon}
          </span>
          {title}
        </div>
        {hint ? <p className="text-muted-foreground text-sm">{hint}</p> : null}
        {children}
      </CardContent>
    </Card>
  );
}

function LinkRow({
  href,
  icon,
  label,
  hint,
}: {
  href: string;
  icon: ReactNode;
  label: string;
  hint: string;
}) {
  return (
    <li>
      <Link
        href={href}
        className="hover:bg-muted flex min-h-12 items-center gap-3 rounded-md px-2 py-2"
      >
        <span className="text-muted-foreground">{icon}</span>
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium">{label}</span>
          <span className="text-muted-foreground block text-xs">{hint}</span>
        </span>
        <ChevronRight className="text-muted-foreground size-4 rtl:rotate-180" />
      </Link>
    </li>
  );
}
