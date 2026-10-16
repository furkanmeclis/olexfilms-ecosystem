"use client";

import { Clock, Info } from "lucide-react";

import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ImportStatusCards } from "@/features/io/components/import-status-cards";
import type { ImportConfirmCopy } from "@/features/io/lib/import-summary";
import type { ImportPreviewSummary } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";

type ImportConfirmStepProps = {
  summary: ImportPreviewSummary;
  copy: ImportConfirmCopy;
  importable: number;
  skipped: number;
  acknowledged: boolean;
  onAcknowledgedChange: (value: boolean) => void;
};

/**
 * Step 4: what the import will do in one sentence, the skipped rows
 * acknowledgement and the undo window.
 */
export function ImportConfirmStep({
  summary,
  copy,
  importable,
  skipped,
  acknowledged,
  onAcknowledgedChange,
}: ImportConfirmStepProps) {
  const { t } = useLocale();

  return (
    <div className="space-y-6">
      <div className="bg-primary/5 border-primary/30 rounded-lg border p-5">
        <p
          className="text-lg leading-relaxed font-medium"
          data-testid="import-confirm-sentence"
        >
          {copy.sentence}
        </p>
      </div>

      <ImportStatusCards summary={summary} />

      {importable === 0 ? (
        <p
          className="text-destructive flex items-start gap-2 text-sm"
          role="alert"
        >
          <Info className="mt-0.5 size-4 shrink-0" />
          {t("imports.confirm_step.nothing")}
        </p>
      ) : null}

      {skipped > 0 && importable > 0 ? (
        <div className="flex items-start gap-3 rounded-lg border p-4">
          <Checkbox
            id="import-ack-skipped"
            checked={acknowledged}
            onCheckedChange={(v) => onAcknowledgedChange(v === true)}
            data-testid="import-ack-skipped"
          />
          <Label
            htmlFor="import-ack-skipped"
            className="leading-snug font-normal"
          >
            {t("imports.confirm_step.ack_skipped")}
          </Label>
        </div>
      ) : null}

      <p className="text-muted-foreground flex items-start gap-2 text-sm">
        <Clock className="mt-0.5 size-4 shrink-0" />
        {t("imports.confirm_step.undo_window")}
      </p>
    </div>
  );
}
