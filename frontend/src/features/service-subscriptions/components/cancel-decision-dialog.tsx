"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";

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
import { amountNumber } from "@/features/service-subscriptions/lib/subscriptions";
import {
  serviceSubscriptionKeys,
  serviceSubscriptionsService,
  type CancelQueueItem,
} from "@/features/service-subscriptions/services/service-subscriptions.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export type CancelDecision = "approve" | "reject";

/**
 * Center decision on an early cancellation request (TEC-311): approve with
 * an optional note, reject with a required reason (sent as decision note).
 */
export function CancelDecisionDialog({
  request,
  decision,
  onClose,
}: {
  request: CancelQueueItem | null;
  decision: CancelDecision | null;
  onClose: () => void;
}) {
  const { t, format } = useLocale();
  const id = useId();
  const queryClient = useQueryClient();
  const [note, setNote] = useState("");
  const reject = decision === "reject";

  const decide = useMutation({
    mutationFn: (req: CancelQueueItem) =>
      reject
        ? serviceSubscriptionsService.rejectCancel(req.uuid, note)
        : serviceSubscriptionsService.approveCancel(req.uuid, note),
    onSuccess: () => {
      appToast.success(
        t(
          reject
            ? "catalog.subscriptions.queue.rejected"
            : "catalog.subscriptions.queue.approved",
        ),
      );
      void queryClient.invalidateQueries({
        queryKey: serviceSubscriptionKeys.all,
      });
      setNote("");
      onClose();
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) && error.status === 409
          ? t("catalog.subscriptions.queue.errors.conflict")
          : t("catalog.subscriptions.queue.errors.failed"),
      ),
  });

  const ready = Boolean(request) && (!reject || note.trim().length > 0);

  return (
    <Dialog
      open={Boolean(request && decision)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent data-testid="cancel-decision-dialog">
        <DialogHeader>
          <DialogTitle>
            {t(
              reject
                ? "catalog.subscriptions.queue.reject_title"
                : "catalog.subscriptions.queue.approve_title",
            )}
          </DialogTitle>
          <DialogDescription>
            {request
              ? t("catalog.subscriptions.queue.decision_summary", {
                  org: request.organization_name,
                  item: request.item_name,
                  fee: format.currency(
                    amountNumber(request.cancellation_fee),
                    request.currency,
                  ),
                })
              : null}
          </DialogDescription>
        </DialogHeader>
        {request ? (
          <p className="bg-muted rounded-md p-3 text-sm whitespace-pre-wrap">
            {request.reason}
          </p>
        ) : null}
        <div className="space-y-2">
          <Label htmlFor={`${id}-note`}>
            {t(
              reject
                ? "catalog.subscriptions.queue.reject_reason"
                : "catalog.subscriptions.queue.approve_note",
            )}
          </Label>
          <Textarea
            id={`${id}-note`}
            data-testid="decision-note"
            value={note}
            maxLength={5000}
            onChange={(e) => setNote(e.target.value)}
          />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            variant={reject ? "destructive" : "default"}
            data-testid="decision-submit"
            disabled={!ready || decide.isPending}
            onClick={() => request && decide.mutate(request)}
          >
            {t(
              reject
                ? "catalog.subscriptions.queue.reject"
                : "catalog.subscriptions.queue.approve",
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
