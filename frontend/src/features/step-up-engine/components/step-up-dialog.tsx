"use client";

import { startAuthentication } from "@simplewebauthn/browser";
import { Fingerprint, LockKeyhole, ShieldCheck } from "lucide-react";
import { useState } from "react";
import { z } from "zod";

import { AppForm, AppInput, AppPassword } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldError, FieldGroup } from "@/components/ui/field";
import { Separator } from "@/components/ui/separator";
import { stepUpService } from "@/features/step-up-engine/services/stepup.service";
import type {
  StepUpMethod,
  StepUpStatus,
} from "@/features/step-up-engine/types";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

type StepUpDialogProps = {
  open: boolean;
  status: StepUpStatus | null;
  onOpenChange: (open: boolean) => void;
  onVerified: () => void | Promise<void>;
};

function createPasswordSchema(t: (key: string) => string) {
  return z.object({
    password: z.string().min(1, t("stepup.validation.password_required")),
  });
}

function createTotpSchema(t: (key: string) => string) {
  return z.object({
    code: z.string().min(1, t("stepup.validation.totp_required")),
  });
}

export function StepUpDialog({
  open,
  status,
  onOpenChange,
  onVerified,
}: StepUpDialogProps) {
  const { t } = useLocale();
  const [formError, setFormError] = useState<string | null>(null);
  const [passwordPending, setPasswordPending] = useState(false);
  const [passkeyPending, setPasskeyPending] = useState(false);
  const [totpPending, setTotpPending] = useState(false);

  const methods = status?.methods ?? [];
  const passwordEnabled = methods.includes("password");
  const passkeyEnabled = methods.includes("passkey");
  const totpEnabled = methods.includes("totp");
  const passwordSchema = createPasswordSchema(t);
  const totpSchema = createTotpSchema(t);

  const enabledCount =
    Number(passwordEnabled) + Number(passkeyEnabled) + Number(totpEnabled);

  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (!open) {
      setFormError(null);
      setPasswordPending(false);
      setPasskeyPending(false);
      setTotpPending(false);
    }
  }

  const onPasswordSubmit = async (values: z.infer<typeof passwordSchema>) => {
    setFormError(null);
    setPasswordPending(true);
    try {
      await stepUpService.verifyPassword(values.password);
      await onVerified();
    } catch (error) {
      setFormError(
        isApiError(error)
          ? error.message
          : t("stepup.error.verification_failed"),
      );
    } finally {
      setPasswordPending(false);
    }
  };

  const onTotpSubmit = async (values: z.infer<typeof totpSchema>) => {
    setFormError(null);
    setTotpPending(true);
    try {
      await stepUpService.verifyTotp(values.code.trim());
      await onVerified();
    } catch (error) {
      setFormError(
        isApiError(error) ? error.message : t("stepup.error.totp_failed"),
      );
    } finally {
      setTotpPending(false);
    }
  };

  const onPasskey = async () => {
    setFormError(null);
    setPasskeyPending(true);
    try {
      const options = await stepUpService.passkeyOptions();
      const assertion = await startAuthentication(options);
      await stepUpService.passkeyVerify(assertion);
      await onVerified();
    } catch (error) {
      setFormError(
        isApiError(error) ? error.message : t("stepup.error.passkey_failed"),
      );
    } finally {
      setPasskeyPending(false);
    }
  };

  const anyPending = passwordPending || passkeyPending || totpPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <LockKeyhole className="size-5" aria-hidden />
            {t("stepup.dialog.title")}
          </DialogTitle>
          <DialogDescription>
            {t("stepup.dialog.description")}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {passkeyEnabled ? (
            <div className="space-y-2">
              <Button
                type="button"
                variant="outline"
                className="w-full"
                disabled={anyPending}
                onClick={onPasskey}
              >
                <Fingerprint aria-hidden />
                {passkeyPending
                  ? t("stepup.dialog.passkey_pending")
                  : t("stepup.dialog.passkey")}
              </Button>
            </div>
          ) : null}

          {totpEnabled ? (
            <AppForm
              schema={totpSchema}
              defaultValues={{ code: "" }}
              onSubmit={onTotpSubmit}
            >
              <FieldGroup>
                <AppInput
                  name="code"
                  label={t("stepup.dialog.totp")}
                  autoComplete="one-time-code"
                  inputMode="numeric"
                  placeholder={t("stepup.dialog.totp_placeholder")}
                  disabled={anyPending}
                />
                <DialogFooter className="px-0 pt-2 sm:justify-end">
                  <Button type="submit" disabled={anyPending}>
                    <ShieldCheck aria-hidden />
                    {totpPending
                      ? t("stepup.dialog.verify_pending")
                      : t("stepup.dialog.verify")}
                  </Button>
                </DialogFooter>
              </FieldGroup>
            </AppForm>
          ) : null}

          {enabledCount > 1 &&
          (passkeyEnabled || totpEnabled) &&
          passwordEnabled ? (
            <div className="flex items-center gap-3">
              <Separator className="flex-1" />
              <span className="text-muted-foreground text-xs uppercase">
                {t("stepup.dialog.or_password")}
              </span>
              <Separator className="flex-1" />
            </div>
          ) : null}

          {passwordEnabled ? (
            <AppForm
              schema={passwordSchema}
              defaultValues={{ password: "" }}
              onSubmit={onPasswordSubmit}
            >
              <FieldGroup>
                <AppPassword
                  name="password"
                  label={t("stepup.dialog.password")}
                  autoComplete="current-password"
                  placeholder={t("stepup.dialog.password_placeholder")}
                />
                <DialogFooter className="px-0 pt-2 sm:justify-end">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => onOpenChange(false)}
                    disabled={anyPending}
                  >
                    {t("common.cancel")}
                  </Button>
                  <Button type="submit" disabled={anyPending}>
                    {passwordPending
                      ? t("stepup.dialog.verify_pending")
                      : t("stepup.dialog.verify")}
                  </Button>
                </DialogFooter>
              </FieldGroup>
            </AppForm>
          ) : null}

          {formError ? (
            <Field data-invalid>
              <FieldError>{formError}</FieldError>
            </Field>
          ) : null}

          {!passwordEnabled && !passkeyEnabled && !totpEnabled ? (
            <p className="text-muted-foreground text-sm">
              {t("stepup.dialog.no_methods")}
            </p>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  );
}

export type { StepUpMethod };
