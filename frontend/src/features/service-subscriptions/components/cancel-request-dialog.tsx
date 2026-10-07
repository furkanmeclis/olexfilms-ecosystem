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
  type ServiceSubscription,
} from "@/features/service-subscriptions/services/service-subscriptions.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Page = { items: ServiceSubscription[] };

/**
 * "Erken iptal talebi" (TEC-311): the reason is required and the
 * cancellation fee of the subscription is shown before sending. On success
 * the cached subscription moves to cancel_requested right away.
 */
export function CancelRequestDialog({
  subscription,
  onClose,
}: {
  subscription: ServiceSubscription | null;
  onClose: () => void;
}) {
  const { t, format } = useLocale();
  const id = useId();
  const queryClient = useQueryClient();
  const [reason, setReason] = useState("");

  const request = useMutation({
    mutationFn: (sub: ServiceSubscription) =>
      serviceSubscriptionsService.requestCancel(sub.uuid, reason.trim()),
    onSuccess: (_req, sub) => {
      const moved = { ...sub, status: "cancel_requested" as const };
      queryClient.setQueryData(serviceSubscriptionKeys.detail(sub.uuid), moved);
      queryClient.setQueriesData<Page>(
        { queryKey: [...serviceSubscriptionKeys.all, "list"] },
        (page) =>
          page
            ? {
                ...page,
                items: page.items.map((row) =>
                  row.uuid === sub.uuid
                    ? { ...row, status: moved.status }
                    : row,
                ),
              }
            : page,
      );
      void queryClient.invalidateQueries({
        queryKey: serviceSubscriptionKeys.all,
      });
      appToast.success(t("catalog.subscriptions.cancel.done"));
      setReason("");
      onClose();
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) && error.status === 409
          ? t("catalog.subscriptions.cancel.errors.conflict")
          : t("catalog.subscriptions.cancel.errors.failed"),
      ),
  });

  const fee = subscription
    ? format.currency(
        amountNumber(subscription.cancellation_fee),
        subscription.currency,
      )
    : "";

  return (
    <Dialog
      open={Boolean(subscription)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent data-testid="cancel-request-dialog">
        <DialogHeader>
          <DialogTitle>{t("catalog.subscriptions.cancel.title")}</DialogTitle>
          <DialogDescription>
            {t("catalog.subscriptions.cancel.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <p
            className="bg-muted rounded-md p-3 text-sm"
            data-testid="cancel-fee-info"
          >
            {t("catalog.subscriptions.cancel.fee_info", { fee })}
          </p>
          <div className="space-y-2">
            <Label htmlFor={`${id}-reason`}>
              {t("catalog.subscriptions.cancel.reason")}
            </Label>
            <Textarea
              id={`${id}-reason`}
              data-testid="cancel-reason"
              value={reason}
              maxLength={5000}
              onChange={(e) => setReason(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            data-testid="cancel-submit"
            disabled={!reason.trim() || request.isPending || !subscription}
            onClick={() => subscription && request.mutate(subscription)}
          >
            {t("catalog.subscriptions.cancel.submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
