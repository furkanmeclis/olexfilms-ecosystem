"use client";

import { useMutation } from "@tanstack/react-query";
import { UserX } from "lucide-react";
import { useState } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  PortalApiError,
  portalAccountApi,
} from "@/features/portal/lib/portal-client";
import { useLocale } from "@/providers/locale-provider";

/** Message key of a failed code request or deletion. */
export function deleteAccountErrorKey(error: unknown): string {
  const code = error instanceof PortalApiError ? error.code : null;
  switch (code) {
    case "INVALID_OTP_CODE":
      return "portal.errors.invalid_code";
    case "OTP_LOCKED":
      return "portal.errors.otp_locked";
    case "RATE_LIMITED":
      return "portal.errors.rate_limited";
    case "OTP_DELIVERY_FAILED":
      return "portal.errors.delivery_failed";
    case "ACCOUNT_HAS_NO_PHONE":
      return "portal.delete.no_phone";
    case "CUSTOMER_HAS_PANEL_ACCOUNT":
      return "portal.delete.panel_account";
    default:
      return "portal.delete.failed";
  }
}

/**
 * TEC-1257 (KVKK, App Store 5.1.1(v)): the customer deletes their own
 * account. A code goes to the account's phone; the code confirms the
 * anonymization (services and warranties stay, without the person). Then
 * the portal session ends (onDeleted).
 */
export function PortalDeleteAccount({
  onDeleted,
}: {
  onDeleted: () => Promise<void> | void;
}) {
  const { t, locale } = useLocale();
  const [sent, setSent] = useState(false);
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);

  const request = useMutation({
    mutationFn: () => portalAccountApi.requestDeleteCode(locale),
    onSuccess: () => {
      setSent(true);
      setError(null);
    },
    onError: (e) => setError(deleteAccountErrorKey(e)),
  });
  const remove = useMutation({
    mutationFn: () => portalAccountApi.deleteAccount(code.trim()),
    onSuccess: async () => {
      setError(null);
      await onDeleted();
    },
    onError: (e) => setError(deleteAccountErrorKey(e)),
  });

  return (
    <Card data-testid="portal-delete-account" className="border-destructive/40">
      <CardContent className="space-y-3">
        <div className="text-destructive flex items-center gap-2 font-medium">
          <UserX className="size-5" aria-hidden />
          {t("portal.delete.title")}
        </div>
        <p className="text-muted-foreground text-sm">
          {t("portal.delete.hint")}
        </p>
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{t(error)}</AlertDescription>
          </Alert>
        ) : null}
        {!sent ? (
          <Button
            variant="destructive"
            disabled={request.isPending}
            onClick={() => request.mutate()}
            data-testid="portal-delete-send"
          >
            {t("portal.delete.send_code")}
          </Button>
        ) : (
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (code.trim()) remove.mutate();
            }}
          >
            <p className="text-sm">{t("portal.delete.code_sent")}</p>
            <div className="max-w-xs space-y-2">
              <Label htmlFor="portal-delete-code">
                {t("portal.delete.code_label")}
              </Label>
              <Input
                id="portal-delete-code"
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={6}
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
              />
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                type="submit"
                variant="destructive"
                disabled={remove.isPending || !code.trim()}
                data-testid="portal-delete-confirm"
              >
                {t("portal.delete.confirm")}
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={request.isPending}
                onClick={() => request.mutate()}
              >
                {t("portal.phone.resend")}
              </Button>
            </div>
          </form>
        )}
      </CardContent>
    </Card>
  );
}
