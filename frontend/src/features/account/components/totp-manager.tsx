"use client";

import QRCode from "qrcode";
import { ShieldCheck } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";

type SetupState = {
  secret: string;
  otpauthUrl: string;
};

export function TotpManager() {
  const { t } = useLocale();
  const [setup, setSetup] = useState<SetupState | null>(null);
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);
  const [confirmCode, setConfirmCode] = useState("");
  const [disableCode, setDisableCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [disableOpen, setDisableOpen] = useState(false);
  const [pending, setPending] = useState(false);

  const statusQuery = useQuery({
    queryKey: ["account", "totp"],
    queryFn: async () => {
      try {
        return await authService.getTOTPStatus();
      } catch (error) {
        toast.error(
          isApiError(error) ? error.message : t("auth.totp.error.load"),
        );
        throw error;
      }
    },
  });
  const enabled = statusQuery.data?.enabled ?? false;
  const loading = statusQuery.isPending;

  const beginSetup = async () => {
    setPending(true);
    try {
      const result = await authService.setupTOTP();
      const url = await QRCode.toDataURL(result.otpauth_url, {
        width: 200,
        margin: 1,
      });
      setSetup({ secret: result.secret, otpauthUrl: result.otpauth_url });
      setQrDataUrl(url);
      setConfirmCode("");
      setRecoveryCodes(null);
    } catch (error) {
      toast.error(
        isApiError(error) ? error.message : t("auth.totp.error.setup"),
      );
    } finally {
      setPending(false);
    }
  };

  const confirmSetup = async () => {
    setPending(true);
    try {
      const result = await authService.confirmTOTP(confirmCode.trim());
      setRecoveryCodes(result.recovery_codes);
      setSetup(null);
      setQrDataUrl(null);
      await statusQuery.refetch();
      toast.success(t("auth.totp.confirm_success"));
    } catch (error) {
      toast.error(
        isApiError(error) ? error.message : t("auth.totp.error.confirm"),
      );
    } finally {
      setPending(false);
    }
  };

  const disableTotp = async () => {
    setPending(true);
    try {
      await authService.disableTOTP(disableCode.trim());
      setSetup(null);
      setQrDataUrl(null);
      setDisableOpen(false);
      setDisableCode("");
      await statusQuery.refetch();
      toast.success(t("auth.totp.disable_success"));
    } catch (error) {
      toast.error(
        isApiError(error) ? error.message : t("auth.totp.error.disable"),
      );
    } finally {
      setPending(false);
    }
  };

  if (loading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-5 w-40" />
        <Skeleton className="h-10 w-full" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={enabled ? "success" : "secondary"}>
          {enabled ? t("auth.totp.status_on") : t("auth.totp.status_off")}
        </Badge>
      </div>

      {recoveryCodes ? (
        <Alert>
          <ShieldCheck aria-hidden />
          <AlertDescription className="space-y-2">
            <p className="font-medium">{t("auth.totp.recovery_title")}</p>
            <p className="text-sm">{t("auth.totp.recovery_hint")}</p>
            <ul className="grid gap-1 font-mono text-xs sm:grid-cols-2">
              {recoveryCodes.map((code) => (
                <li key={code}>{code}</li>
              ))}
            </ul>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setRecoveryCodes(null)}
            >
              {t("auth.totp.recovery_done")}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}

      {setup ? (
        <div className="space-y-4 rounded-lg border p-4">
          <p className="text-sm">{t("auth.totp.setup_scan_hint")}</p>
          {qrDataUrl ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={qrDataUrl}
              alt={t("auth.totp.qr_alt")}
              className="mx-auto size-[200px] rounded-md border bg-white p-2"
            />
          ) : (
            <Skeleton className="mx-auto size-[200px]" />
          )}
          <div className="space-y-1">
            <Label htmlFor="totp-secret">{t("auth.totp.manual_secret")}</Label>
            <Input
              id="totp-secret"
              readOnly
              value={setup.secret}
              className="font-mono text-xs"
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="totp-confirm">{t("auth.totp.confirm_code")}</Label>
            <Input
              id="totp-confirm"
              inputMode="numeric"
              autoComplete="one-time-code"
              placeholder={t("auth.totp.code_placeholder")}
              value={confirmCode}
              onChange={(event) => setConfirmCode(event.target.value)}
            />
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              disabled={pending || confirmCode.trim().length < 6}
              onClick={() => void confirmSetup()}
            >
              {pending ? t("auth.totp.confirming") : t("auth.totp.confirm")}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={pending}
              onClick={() => setSetup(null)}
            >
              {t("common.cancel")}
            </Button>
          </div>
        </div>
      ) : enabled ? (
        <Button
          type="button"
          variant="outline"
          onClick={() => {
            setDisableCode("");
            setDisableOpen(true);
          }}
        >
          {t("auth.totp.disable")}
        </Button>
      ) : (
        <Button
          type="button"
          disabled={pending}
          onClick={() => void beginSetup()}
        >
          {pending ? t("auth.totp.setting_up") : t("auth.totp.enable")}
        </Button>
      )}

      <Dialog open={disableOpen} onOpenChange={setDisableOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("auth.totp.disable_title")}</DialogTitle>
            <DialogDescription>
              {t("auth.totp.disable_description")}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="totp-disable-code">
              {t("auth.totp.confirm_code")}
            </Label>
            <Input
              id="totp-disable-code"
              inputMode="numeric"
              autoComplete="one-time-code"
              placeholder={t("auth.totp.code_placeholder")}
              value={disableCode}
              onChange={(event) => setDisableCode(event.target.value)}
            />
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setDisableOpen(false)}
              disabled={pending}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={pending || disableCode.trim().length < 6}
              onClick={() => void disableTotp()}
            >
              {pending ? t("auth.totp.disabling") : t("auth.totp.disable")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
