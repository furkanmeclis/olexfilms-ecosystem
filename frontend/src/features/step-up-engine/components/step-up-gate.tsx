"use client";

import { LockKeyhole } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useStepUp } from "@/features/step-up-engine/hooks/use-step-up";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type StepUpGateProps = {
  children: ReactNode;
  purpose?: string;
  className?: string;
};

export function StepUpGate({ children, purpose, className }: StepUpGateProps) {
  const { t } = useLocale();
  const { status, ensure, isEnsuring } = useStepUp();
  const unlocked = status?.valid === true;

  if (unlocked) {
    return <div className={className}>{children}</div>;
  }

  return (
    <div
      className={cn(
        "bg-muted/40 flex flex-col items-center gap-4 rounded-lg border p-5 text-center sm:flex-row sm:items-center sm:text-start",
        className,
      )}
    >
      <div className="bg-background flex size-10 shrink-0 items-center justify-center rounded-full border shadow-sm">
        <LockKeyhole className="text-muted-foreground size-4" aria-hidden />
      </div>
      <div className="min-w-0 flex-1 space-y-1">
        <p className="text-sm font-medium">{t("stepup.gate.title")}</p>
        <p className="text-muted-foreground text-xs leading-relaxed">
          {purpose
            ? t(`stepup.gate.purpose.${purpose}`)
            : t("stepup.gate.description")}
        </p>
      </div>
      <Button
        type="button"
        size="sm"
        className="shrink-0"
        disabled={isEnsuring}
        onClick={() => void ensure()}
      >
        {isEnsuring ? t("stepup.gate.unlocking") : t("stepup.gate.unlock")}
      </Button>
    </div>
  );
}
