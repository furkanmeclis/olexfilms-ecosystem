"use client";

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
import type { StockTransferStatus } from "@/features/transfers/services/transfers.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * Optional reason of a reject / cancel move (detail page and list row
 * actions, TEC-374).
 */
export function ReasonDialog({
  target,
  onClose,
  onConfirm,
  pending,
}: {
  target: StockTransferStatus | null;
  onClose: () => void;
  onConfirm: (reason: string) => void;
  pending: boolean;
}) {
  const { t } = useLocale();
  const [reason, setReason] = useState("");
  return (
    <Dialog
      open={target !== null}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {target ? t(`transfers.action.${target}`) : ""}
          </DialogTitle>
          <DialogDescription>
            {t("transfers.detail.reason_hint")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor="transfer-reason">
            {t("transfers.detail.reason")}
          </Label>
          <Textarea
            id="transfer-reason"
            rows={3}
            maxLength={500}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("transfers.form.cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            data-testid="transfer-reason-confirm"
            disabled={pending}
            onClick={() => onConfirm(reason.trim())}
          >
            {target ? t(`transfers.action.${target}`) : ""}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
