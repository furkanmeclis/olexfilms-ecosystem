"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  VOID_REASON_MAX,
  validVoidReason,
} from "@/features/einvoice/lib/einvoice";
import {
  einvoiceKeys,
  einvoiceService,
  type Einvoice,
} from "@/features/einvoice/services/einvoice.service";
import { useStepUp } from "@/features/step-up-engine";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Void with a reason (list row action and detail, TEC-504). A mark only:
 * the number and files stay, the source becomes billable again. Needs a
 * recent step-up, asked for before the request.
 */
export function VoidDialog({
  invoice,
  open,
  onOpenChange,
}: {
  invoice: Pick<Einvoice, "uuid" | "number">;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const { ensure } = useStepUp();
  const [reason, setReason] = useState("");
  const mutation = useMutation({
    mutationFn: async () => {
      if (!(await ensure())) return null;
      return einvoiceService.void(invoice.uuid, reason.trim());
    },
    onSuccess: (updated) => {
      if (!updated) return;
      qc.setQueryData(einvoiceKeys.detail(invoice.uuid), updated);
      void qc.invalidateQueries({ queryKey: einvoiceKeys.all });
      appToast.success(t("einvoice.void.success"));
      setReason("");
      onOpenChange(false);
    },
  });
  const ok = validVoidReason(reason);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("einvoice.void.title")}</DialogTitle>
          <DialogDescription>
            {t("einvoice.void.description", {
              number: invoice.number ?? t("einvoice.draft_number"),
            })}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor="einvoice-void-reason">
            {t("einvoice.void.reason")}
          </Label>
          <Textarea
            id="einvoice-void-reason"
            data-testid="einvoice-void-reason"
            value={reason}
            maxLength={VOID_REASON_MAX}
            rows={4}
            placeholder={t("einvoice.void.reason_placeholder")}
            onChange={(e) => setReason(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            {t("einvoice.void.reason_hint")}
          </p>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            data-testid="einvoice-void-confirm"
            disabled={!ok || mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {t("einvoice.void.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
