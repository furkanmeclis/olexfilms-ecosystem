"use client";

import { useState } from "react";

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
import { useLocale } from "@/providers/locale-provider";

import type { LegalText } from "../lib/types";

/**
 * K22: the AI guidelines (Markdown) must be accepted before chatting. The
 * box starts unchecked and "Accept" stays disabled until it is checked;
 * closing leaves the chat input disabled. Mount it with a key per opening
 * so the box starts unchecked every time.
 */
export function AssistantConsentDialog({
  text,
  open,
  onOpenChange,
  onAccept,
}: {
  text: LegalText | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onAccept: () => Promise<void>;
}) {
  const { t } = useLocale();
  const [checked, setChecked] = useState(false);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);

  const accept = async () => {
    setBusy(true);
    setFailed(false);
    try {
      await onAccept();
    } catch {
      setFailed(true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open && text !== null} onOpenChange={onOpenChange}>
      <DialogContent data-testid="ai-assistant-consent">
        <DialogHeader>
          <DialogTitle>{t("ai.consent.title")}</DialogTitle>
          <DialogDescription>{t("ai.consent.description")}</DialogDescription>
        </DialogHeader>
        {text ? (
          <div className="bg-muted/40 max-h-72 overflow-y-auto rounded-md p-4">
            <Markdown source={text.body} />
          </div>
        ) : null}
        <div className="flex items-start gap-3">
          <Checkbox
            id="ai-assistant-consent-accept"
            checked={checked}
            onCheckedChange={(v) => setChecked(v === true)}
          />
          <Label htmlFor="ai-assistant-consent-accept" className="leading-snug">
            {t("ai.consent.checkbox")}
          </Label>
        </div>
        {failed ? (
          <Alert variant="destructive">
            <AlertDescription>{t("ai.consent.failed")}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("ai.consent.later")}
          </Button>
          <Button
            data-testid="ai-assistant-consent-accept"
            onClick={() => void accept()}
            disabled={!checked || busy}
          >
            {t("ai.consent.accept")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
