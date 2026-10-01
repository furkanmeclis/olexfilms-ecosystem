"use client";

import { useEffect, useState } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Markdown } from "@/features/portal/lib/markdown";
import {
  portalApi,
  type PendingLegalText,
} from "@/features/portal/lib/portal-client";
import { useLocale } from "@/providers/locale-provider";

/**
 * K22: after sign-in the AI guidelines are asked once per text version. The
 * box starts unchecked; "Continue" records the box state (accept or
 * decline). Declining never locks the portal, only the AI features.
 */
export function AIConsentDialog() {
  const { t, locale } = useLocale();
  const [pending, setPending] = useState<PendingLegalText | null>(null);
  const [checked, setChecked] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    portalApi
      .pendingConsents(locale)
      .then((res) => {
        if (cancelled) return;
        const text = res.items.find((i) => i.kind === "ai_guidelines");
        setPending(text ?? null);
        setChecked(false);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [locale]);

  const onContinue = async () => {
    if (!pending) return;
    setBusy(true);
    setError(false);
    try {
      await portalApi.decideConsent(pending, checked);
      setPending(null);
    } catch {
      setError(true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={pending !== null}>
      <DialogContent
        data-testid="ai-consent-dialog"
        className="[&>button:last-child]:hidden"
        onEscapeKeyDown={(e) => e.preventDefault()}
        onPointerDownOutside={(e) => e.preventDefault()}
        onInteractOutside={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{t("portal.consent.title")}</DialogTitle>
          <DialogDescription>
            {t("portal.consent.decline_note")}
          </DialogDescription>
        </DialogHeader>
        {pending ? (
          <div className="bg-muted/40 max-h-72 overflow-y-auto rounded-md p-4">
            <Markdown source={pending.body} />
          </div>
        ) : null}
        <div className="flex items-start gap-3">
          <Checkbox
            id="ai-consent-accept"
            checked={checked}
            onCheckedChange={(v) => setChecked(v === true)}
          />
          <Label htmlFor="ai-consent-accept" className="leading-snug">
            {t("portal.consent.checkbox")}
          </Label>
        </div>
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{t("portal.consent.failed")}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button onClick={() => void onContinue()} disabled={busy}>
            {t("portal.consent.continue")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
