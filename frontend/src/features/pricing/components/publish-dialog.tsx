"use client";

import { useMutation } from "@tanstack/react-query";
import { LockKeyhole } from "lucide-react";
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
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useStepUp } from "@/features/step-up-engine";
import {
  effectiveFromError,
  isoDay,
  publishRows,
  type BasketRow,
} from "@/features/pricing/lib/recommended";
import {
  recommendedService,
  type RecommendedPublishResult,
} from "@/features/pricing/services/recommended.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type PublishDialogProps = {
  open: boolean;
  rows: readonly BasketRow[];
  onOpenChange: (open: boolean) => void;
  onPublished: (result: RecommendedPublishResult) => void;
  /** Today's ISO day (tests pin it). */
  today?: string;
};

/**
 * "Publish" of the recommended price basket (TEC-507): effective day (today
 * or later), note, then the step-up dialog before the batch is written.
 */
export function PublishDialog(props: PublishDialogProps) {
  // Remount per opening so the form starts from today again.
  return props.open ? <PublishDialogBody {...props} /> : null;
}

function PublishDialogBody({
  rows,
  onOpenChange,
  onPublished,
  today = isoDay(new Date()),
}: PublishDialogProps) {
  const { t } = useLocale();
  const { ensure } = useStepUp();
  const [effectiveFrom, setEffectiveFrom] = useState(today);
  const [note, setNote] = useState("");
  const error = effectiveFromError(effectiveFrom, today);

  const publish = useMutation({
    mutationFn: async () => {
      // Step-up first: publishing is a pricing.recommended.write action.
      if (!(await ensure())) return null;
      return recommendedService.publish({
        effective_from: effectiveFrom,
        note: note.trim() || null,
        rows: publishRows(rows),
      });
    },
    onSuccess: (result) => {
      if (!result) return;
      appToast.success(
        t("catalog.recommended.publish.done", {
          applied: result.applied_count,
          scheduled: result.scheduled_count,
        }),
      );
      onPublished(result);
    },
    onError: (err) =>
      appToast.error(
        isApiError(err) ? err.message : t("catalog.recommended.publish.failed"),
      ),
  });

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent data-testid="publish-dialog">
        <DialogHeader>
          <DialogTitle>{t("catalog.recommended.publish.title")}</DialogTitle>
          <DialogDescription>
            {t("catalog.recommended.publish.description", {
              count: rows.length,
            })}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault();
            if (error || rows.length === 0 || publish.isPending) return;
            publish.mutate();
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="recommended-effective-from">
              {t("catalog.recommended.publish.effective_from")}
            </Label>
            <DatePicker
              id="recommended-effective-from"
              value={effectiveFrom}
              minDate={today}
              aria-invalid={error ? true : undefined}
              onChange={setEffectiveFrom}
            />
            {error ? (
              <p
                className="text-destructive text-xs"
                data-testid="effective-from-error"
              >
                {t(`catalog.recommended.publish.errors.${error}`)}
              </p>
            ) : (
              <p className="text-muted-foreground text-xs">
                {effectiveFrom === today
                  ? t("catalog.recommended.publish.applies_now")
                  : t("catalog.recommended.publish.applies_later")}
              </p>
            )}
          </div>
          <div className="space-y-2">
            <Label htmlFor="recommended-note">
              {t("catalog.recommended.publish.note")}
            </Label>
            <Textarea
              id="recommended-note"
              name="note"
              maxLength={500}
              value={note}
              onChange={(event) => setNote(event.target.value)}
            />
          </div>
          <p className="text-muted-foreground flex items-center gap-2 text-xs">
            <LockKeyhole className="size-3.5 shrink-0" aria-hidden />
            {t("catalog.recommended.publish.step_up_hint")}
          </p>
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
              data-testid="publish-submit"
              disabled={
                Boolean(error) || rows.length === 0 || publish.isPending
              }
            >
              {t("catalog.recommended.publish.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
