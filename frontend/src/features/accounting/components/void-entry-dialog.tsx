"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
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
  accountingService,
  type FinanceEntry,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Reverses an open manual entry (POST /v1/accounting/entries/{uuid}/void):
 * the ledger is append-only, so a mirror row is written. Needs a reason and
 * a recent step-up (platformRequest opens the prompt and retries).
 */
export function VoidEntryDialog({
  orgUuid,
  entry,
  onOpenChange,
}: {
  orgUuid: string;
  entry: FinanceEntry | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | undefined>();

  const save = useMutation({
    mutationFn: (input: { uuid: string; reason: string }) =>
      accountingService.voidEntry(input.uuid, input.reason),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: accountingKeys.all(orgUuid),
      });
      appToast.success(t("accounting.void.done"));
      setReason("");
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      appToast.error(
        isApiError(err) ? err.message : t("accounting.toast.failed"),
      );
    },
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!entry) return;
    const text = reason.trim();
    if (!text) {
      setError(t("accounting.validation.required"));
      return;
    }
    setError(undefined);
    save.mutate({ uuid: entry.uuid, reason: text });
  };

  return (
    <Dialog open={Boolean(entry)} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("accounting.void.title")}</DialogTitle>
          <DialogDescription>
            {t("accounting.void.description")}
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={submit}
          noValidate
          className="space-y-4"
          data-testid="void-entry-form"
        >
          <div className="grid gap-1.5">
            <Label htmlFor="void-entry-reason">
              {t("accounting.void.reason")}
            </Label>
            <Textarea
              id="void-entry-reason"
              name="reason"
              rows={3}
              maxLength={1000}
              value={reason}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? "void-entry-reason-error" : undefined}
              onChange={(e) => setReason(e.target.value)}
            />
            <FieldError id="void-entry-reason-error" message={error} />
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
              type="submit"
              variant="destructive"
              disabled={save.isPending}
              data-testid="void-entry-submit"
            >
              {t("accounting.void.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
