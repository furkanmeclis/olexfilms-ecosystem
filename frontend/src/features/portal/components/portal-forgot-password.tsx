"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

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
import { routes } from "@/config/routes";
import { PortalApiError, portalApi } from "@/features/portal/lib/portal-client";
import { useLocale } from "@/providers/locale-provider";

/** Portal password reset for fleet accounts (code by e-mail, TEC-90). */
export function PortalForgotPassword() {
  const { t } = useLocale();
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [step, setStep] = useState<"email" | "code" | "done">("email");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const message = (err: unknown) =>
    err instanceof PortalApiError && err.status !== 500
      ? err.message
      : t("portal.errors.generic");

  const onSend = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await portalApi.forgotPassword(email);
      setStep("code");
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  };

  const onReset = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await portalApi.resetPassword(email, code, password);
      setStep("done");
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center px-4 py-10">
      <Card>
        <CardHeader>
          <CardTitle>{t("portal.forgot.title")}</CardTitle>
          <CardDescription>{t("portal.forgot.subtitle")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {step === "email" ? (
            <form className="space-y-4" onSubmit={onSend}>
              <div className="space-y-2">
                <Label htmlFor="portal-forgot-email">
                  {t("portal.forgot.email")}
                </Label>
                <Input
                  id="portal-forgot-email"
                  type="email"
                  autoComplete="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                />
              </div>
              <Button type="submit" className="w-full" disabled={busy}>
                {t("portal.forgot.send")}
              </Button>
            </form>
          ) : null}
          {step === "code" ? (
            <form className="space-y-4" onSubmit={onReset}>
              <p className="text-muted-foreground text-sm">
                {t("portal.forgot.sent")}
              </p>
              <div className="space-y-2">
                <Label htmlFor="portal-forgot-code">
                  {t("portal.forgot.code")}
                </Label>
                <Input
                  id="portal-forgot-code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  value={code}
                  onChange={(e) => setCode(e.target.value.trim())}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="portal-forgot-password">
                  {t("portal.forgot.new_password")}
                </Label>
                <Input
                  id="portal-forgot-password"
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                />
              </div>
              <Button type="submit" className="w-full" disabled={busy}>
                {t("portal.forgot.reset")}
              </Button>
            </form>
          ) : null}
          {step === "done" ? (
            <Alert>
              <AlertDescription>{t("portal.forgot.done")}</AlertDescription>
            </Alert>
          ) : null}
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <div className="text-center text-sm">
            <Link
              href={routes.portal.login}
              className="text-primary underline-offset-2 hover:underline"
            >
              {t("portal.forgot.back")}
            </Link>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
