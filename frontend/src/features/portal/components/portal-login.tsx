"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useState, type FormEvent } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { routes } from "@/config/routes";
import {
  PortalApiError,
  portalApi,
  portalSignIn,
} from "@/features/portal/lib/portal-client";
import { useLocale } from "@/providers/locale-provider";

/** Countries offered by the phone step (K29: default TR, E.164 on the server). */
const COUNTRIES = [
  "TR",
  "AZ",
  "BG",
  "DE",
  "GR",
  "UA",
  "RU",
  "FR",
  "ES",
  "IT",
  "CN",
  "GB",
  "AE",
] as const;

const ERROR_KEYS: Record<string, string> = {
  INVALID_PHONE: "portal.errors.invalid_phone",
  RATE_LIMITED: "portal.errors.rate_limited",
  OTP_DELIVERY_FAILED: "portal.errors.delivery_failed",
  RATE_LIMITER_UNAVAILABLE: "portal.errors.delivery_failed",
  INVALID_OTP_CODE: "portal.errors.invalid_code",
  OTP_LOCKED: "portal.errors.otp_locked",
  NO_PORTAL_ACCESS: "portal.errors.no_portal_access",
  MFA_REQUIRED: "portal.errors.mfa_required",
  INVALID_MFA_CODE: "portal.errors.invalid_mfa",
};

/**
 * Only same-site portal paths are followed after sign-in, plus the MCP
 * OAuth consent screen (TEC-403) a customer signs in for.
 */
export function safePortalNext(next: string | null): string {
  if (next?.startsWith("/oauth/consent?") && !next.includes("//")) {
    return next;
  }
  if (!next || !next.startsWith("/portal") || next.startsWith("//")) {
    return routes.portal.home;
  }
  if (next.startsWith(routes.portal.login)) return routes.portal.home;
  return next;
}

function useCountdown(until: number | null) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!until) return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [until]);
  if (!until) return 0;
  return Math.max(0, Math.ceil((until - now) / 1000));
}

export function PortalLogin() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const searchParams = useSearchParams();
  const next = safePortalNext(searchParams.get("next"));
  const [tab, setTab] = useState("phone");

  const onSignedIn = () => {
    router.replace(next);
    router.refresh();
  };

  return (
    <div className="mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center px-4 py-10">
      <Card>
        <CardHeader>
          <CardTitle>{t("portal.login.title")}</CardTitle>
          <CardDescription>{t("portal.login.subtitle")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Tabs value={tab} onValueChange={setTab}>
            <TabsList className="mb-4 grid w-full grid-cols-2">
              <TabsTrigger value="phone">
                {t("portal.login.tab_phone")}
              </TabsTrigger>
              <TabsTrigger value="fleet">
                {t("portal.login.tab_fleet")}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="phone">
              <PhoneOTPForm locale={locale} onSignedIn={onSignedIn} />
            </TabsContent>
            <TabsContent value="fleet">
              <FleetForm onSignedIn={onSignedIn} />
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>
    </div>
  );
}

function PhoneOTPForm({
  locale,
  onSignedIn,
}: {
  locale: string;
  onSignedIn: () => void;
}) {
  const { t } = useLocale();
  const [country, setCountry] = useState<string>("TR");
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [step, setStep] = useState<"phone" | "code">("phone");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [resendAt, setResendAt] = useState<number | null>(null);
  const seconds = useCountdown(resendAt);

  const countryNames = useMemo(() => {
    try {
      return new Intl.DisplayNames([locale], { type: "region" });
    } catch {
      return null;
    }
  }, [locale]);

  const sendCode = async () => {
    setBusy(true);
    setError(null);
    try {
      const res = await portalApi.requestOTP(phone, country, locale);
      setResendAt(Date.parse(res.resend_at) || null);
      setStep("code");
    } catch (err) {
      if (err instanceof PortalApiError) {
        const retry = err.details.find((d) => d.field === "resend_at");
        if (retry?.message) {
          setResendAt(Date.parse(retry.message) || null);
          // A code may already be on its way: let the user type it.
          if (err.code === "RATE_LIMITED" && retry.code === "cooldown") {
            setStep("code");
          }
        }
        setError(t(ERROR_KEYS[err.code ?? ""] ?? "portal.errors.generic"));
      } else {
        setError(t("portal.errors.generic"));
      }
    } finally {
      setBusy(false);
    }
  };

  const onPhoneSubmit = (e: FormEvent) => {
    e.preventDefault();
    void sendCode();
  };

  const onCodeSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const failure = await portalSignIn("phone-otp", {
      phone,
      code,
      country,
      locale,
    }).catch(() => "unavailable");
    setBusy(false);
    if (failure === null) {
      onSignedIn();
      return;
    }
    setError(t(ERROR_KEYS[failure] ?? "portal.errors.invalid_code"));
  };

  if (step === "code") {
    return (
      <form className="space-y-4" onSubmit={onCodeSubmit}>
        <p className="text-muted-foreground text-sm">
          {t("portal.phone.code_hint", { phone })}
        </p>
        <div className="space-y-2">
          <Label htmlFor="portal-otp">{t("portal.phone.code_label")}</Label>
          <Input
            id="portal-otp"
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="[0-9]{6}"
            maxLength={6}
            value={code}
            onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
            autoFocus
            required
          />
        </div>
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <Button
          type="submit"
          className="w-full"
          disabled={busy || code.length !== 6}
        >
          {busy ? t("portal.phone.verifying") : t("portal.phone.verify")}
        </Button>
        <div className="flex items-center justify-between gap-2 text-sm">
          <button
            type="button"
            className="text-primary underline-offset-2 hover:underline"
            onClick={() => {
              setStep("phone");
              setCode("");
              setError(null);
            }}
          >
            {t("portal.phone.change_number")}
          </button>
          <button
            type="button"
            className="text-primary disabled:text-muted-foreground underline-offset-2 hover:underline disabled:no-underline"
            disabled={busy || seconds > 0}
            onClick={() => void sendCode()}
          >
            {seconds > 0
              ? t("portal.phone.resend_in", { seconds })
              : t("portal.phone.resend")}
          </button>
        </div>
      </form>
    );
  }

  return (
    <form className="space-y-4" onSubmit={onPhoneSubmit}>
      <div className="grid grid-cols-[7rem_1fr] gap-2">
        <div className="space-y-2">
          <Label htmlFor="portal-country">{t("portal.phone.country")}</Label>
          <select
            id="portal-country"
            className="border-input bg-background h-9 w-full rounded-md border px-2 text-sm"
            value={country}
            onChange={(e) => setCountry(e.target.value)}
          >
            {COUNTRIES.map((c) => (
              <option key={c} value={c}>
                {countryNames?.of(c) ?? c}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="portal-phone">{t("portal.phone.label")}</Label>
          <Input
            id="portal-phone"
            type="tel"
            inputMode="tel"
            autoComplete="tel"
            dir="ltr"
            placeholder={t("portal.phone.placeholder")}
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            required
          />
        </div>
      </div>
      {/* K11: information only, no consent checkbox. */}
      <p className="text-muted-foreground text-xs leading-relaxed">
        {t("portal.phone.kvkk_notice")}
      </p>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      <Button type="submit" className="w-full" disabled={busy || !phone.trim()}>
        {busy ? t("portal.phone.sending") : t("portal.phone.send")}
      </Button>
    </form>
  );
}

function FleetForm({ onSignedIn }: { onSignedIn: () => void }) {
  const { t } = useLocale();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [totp, setTotp] = useState("");
  const [needsTotp, setNeedsTotp] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const failure = await portalSignIn("fleet-password", {
      email,
      password,
      ...(totp ? { totp_code: totp } : {}),
    }).catch(() => "unavailable");
    setBusy(false);
    if (failure === null) {
      onSignedIn();
      return;
    }
    if (failure === "MFA_REQUIRED") setNeedsTotp(true);
    setError(t(ERROR_KEYS[failure] ?? "portal.errors.invalid_credentials"));
  };

  return (
    <form className="space-y-4" onSubmit={onSubmit}>
      <div className="space-y-2">
        <Label htmlFor="portal-email">{t("portal.fleet.email")}</Label>
        <Input
          id="portal-email"
          type="email"
          autoComplete="username"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="portal-password">{t("portal.fleet.password")}</Label>
        <Input
          id="portal-password"
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
        />
      </div>
      {needsTotp ? (
        <div className="space-y-2">
          <Label htmlFor="portal-totp">{t("portal.fleet.totp")}</Label>
          <Input
            id="portal-totp"
            inputMode="numeric"
            autoComplete="one-time-code"
            value={totp}
            onChange={(e) => setTotp(e.target.value.trim())}
          />
        </div>
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      <Button type="submit" className="w-full" disabled={busy}>
        {t("portal.fleet.submit")}
      </Button>
      <div className="text-center text-sm">
        <Link
          href={routes.portal.forgotPassword}
          className="text-primary underline-offset-2 hover:underline"
        >
          {t("portal.fleet.forgot")}
        </Link>
      </div>
    </form>
  );
}
