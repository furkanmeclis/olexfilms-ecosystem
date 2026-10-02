"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { MessageSquareWarning } from "lucide-react";
import { useState, type FormEvent } from "react";

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
import { FieldError } from "@/features/accounting/components/shared";
import { accountingKeys } from "@/features/accounting/hooks/use-accounting-access";
import {
  DISPUTE_TEXT_MAX,
  disputeReasonSchema,
} from "@/features/accounting/lib/disputes";
import { fieldErrors } from "@/features/accounting/lib/form";
import {
  accountingService,
  type AccountingDisputeInput,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Reason form of a new dispute (K24: the child explains, the parent fixes). */
export function DisputeForm({
  entryUuid,
  summary,
  pending,
  onCancel,
  onSubmit,
}: {
  entryUuid: string;
  /** One line describing the disputed row (date, category, amount). */
  summary?: string;
  pending?: boolean;
  onCancel: () => void;
  onSubmit: (input: AccountingDisputeInput) => Promise<unknown>;
}) {
  const { t } = useLocale();
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | undefined>();

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const parsed = disputeReasonSchema(t).safeParse({ reason });
    if (!parsed.success) {
      setError(fieldErrors(parsed.error).reason);
      return;
    }
    setError(undefined);
    try {
      await onSubmit({ entry_uuid: entryUuid, reason: parsed.data.reason });
    } catch (err) {
      if (isApiError(err) && err.isValidation) {
        setError(err.fieldErrors().reason ?? err.message);
      }
    }
  };

  return (
    <form
      onSubmit={submit}
      noValidate
      className="space-y-4"
      data-testid="dispute-form"
    >
      {summary ? (
        <p
          className="bg-muted rounded-md p-2 text-sm"
          data-testid="dispute-summary"
        >
          {summary}
        </p>
      ) : null}
      <div className="grid gap-1.5">
        <Label htmlFor="dispute-reason">
          {t("accounting.disputes.fields.reason")}
        </Label>
        <Textarea
          id="dispute-reason"
          name="reason"
          rows={4}
          maxLength={DISPUTE_TEXT_MAX}
          value={reason}
          placeholder={t("accounting.disputes.reason_hint")}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? "dispute-reason-error" : undefined}
          onChange={(e) => setReason(e.target.value)}
        />
        <FieldError id="dispute-reason-error" message={error} />
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button type="submit" disabled={pending} data-testid="dispute-submit">
          {t("accounting.disputes.submit")}
        </Button>
      </DialogFooter>
    </form>
  );
}

/**
 * "Dispute" button of a ledger or statement row. The caller renders it only
 * for a disputable row (lib/disputes isEntryDisputable /
 * isStatementLineDisputable) and only with accounting.dispute.
 */
export function DisputeButton({
  orgUuid,
  entryUuid,
  summary,
}: {
  orgUuid: string;
  entryUuid: string;
  summary?: string;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);

  const save = useMutation({
    mutationFn: (input: AccountingDisputeInput) =>
      accountingService.openDispute(input),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: accountingKeys.all(orgUuid),
      });
      appToast.success(t("accounting.disputes.opened"));
      setOpen(false);
    },
    onError: (error: unknown) => {
      appToast.error(
        isApiError(error) ? error.message : t("accounting.toast.failed"),
      );
    },
  });

  return (
    <>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        data-testid="dispute-open"
        data-entry={entryUuid}
        onClick={() => setOpen(true)}
      >
        <MessageSquareWarning className="size-4" />
        {t("accounting.disputes.open_action")}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("accounting.disputes.open_title")}</DialogTitle>
            <DialogDescription>
              {t("accounting.disputes.open_description")}
            </DialogDescription>
          </DialogHeader>
          {open ? (
            <DisputeForm
              entryUuid={entryUuid}
              summary={summary}
              pending={save.isPending}
              onCancel={() => setOpen(false)}
              onSubmit={(input) => save.mutateAsync(input)}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  );
}
