"use client";

import { Check } from "lucide-react";
import { createContext, useContext, type ReactNode } from "react";
import { createPortal } from "react-dom";

import { canOpenStep, type WizardStep } from "@/features/services/lib/wizard";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * TEC-865: the service wizard keeps its steps and the current step's
 * actions in one floating bar at the bottom of the content area, so the
 * dealer never scrolls to reach İleri / Kaydet. Each step still owns its
 * buttons (handlers, disabled and loading states); `WizardActions` portals
 * them into the bar's slot. Without a bar (a step rendered on its own) the
 * actions stay inline at the end of the step.
 */
const WizardActionsSlot = createContext<HTMLElement | null>(null);

export const WizardActionsProvider = WizardActionsSlot.Provider;

export function WizardActions({ children }: { children: ReactNode }) {
  const slot = useContext(WizardActionsSlot);
  if (slot) return createPortal(children, slot);
  return <div className="flex flex-wrap justify-between gap-2">{children}</div>;
}

export function WizardBottomBar({
  steps,
  current,
  hasService,
  contractLocked,
  photosLocked,
  onSelect,
  actionsRef,
}: {
  steps: readonly WizardStep[];
  current: WizardStep;
  hasService: boolean;
  contractLocked: boolean;
  photosLocked: boolean;
  onSelect: (step: WizardStep) => void;
  actionsRef: (el: HTMLDivElement | null) => void;
}) {
  const { t } = useLocale();
  const currentIndex = Math.max(steps.indexOf(current), 0);
  return (
    <div
      data-testid="wizard-bar"
      className={cn(
        "border-border bg-card sticky z-20 flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border px-3 py-2.5 shadow-lg sm:px-4",
        "bottom-[max(0.5rem,env(safe-area-inset-bottom))] sm:bottom-3",
        "supports-[backdrop-filter]:bg-card/85 supports-[backdrop-filter]:backdrop-blur-md",
      )}
    >
      <nav
        aria-label={t("services.wizard.bar.label")}
        className="flex min-w-0 items-center gap-3"
      >
        <p
          className="min-w-0 truncate text-sm"
          data-testid="wizard-progress"
          aria-live="polite"
        >
          <span className="font-semibold">
            {t("services.wizard.bar.progress", {
              current: currentIndex + 1,
              total: steps.length,
            })}
          </span>
          <span className="text-muted-foreground hidden sm:inline">
            {" · "}
            {t(`services.wizard.steps.${current}`)}
          </span>
        </p>
        <ol
          className="hidden items-center gap-1 md:flex"
          data-testid="wizard-stepper"
        >
          {steps.map((step, i) => {
            const active = step === current;
            const done = hasService && i < currentIndex;
            const open = canOpenStep(
              step,
              hasService,
              contractLocked,
              steps,
              photosLocked,
            );
            // TEC-500: a step closed by missing intake photos says why.
            const photoBlocked =
              hasService &&
              !open &&
              !canOpenStep(step, hasService, false, steps, photosLocked);
            const label = t(`services.wizard.steps.${step}`);
            return (
              <li key={step} className="flex items-center gap-1">
                {i > 0 ? (
                  <span
                    aria-hidden
                    className={cn(
                      "h-px w-3 lg:w-5",
                      done || active ? "bg-primary" : "bg-border",
                    )}
                  />
                ) : null}
                <button
                  type="button"
                  disabled={!open}
                  aria-label={`${i + 1}. ${label}`}
                  title={
                    photoBlocked
                      ? t("photo_standard.intake.next_blocked")
                      : label
                  }
                  data-blocked={photoBlocked ? "photos" : undefined}
                  aria-current={active ? "step" : undefined}
                  data-step={step}
                  onClick={() => onSelect(step)}
                  className={cn(
                    "focus-visible:ring-ring flex size-8 shrink-0 items-center justify-center rounded-full border text-xs font-semibold transition-colors focus-visible:ring-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50",
                    active
                      ? "border-primary bg-primary text-primary-foreground"
                      : done
                        ? "border-primary text-primary hover:bg-primary/10"
                        : "hover:bg-accent",
                  )}
                >
                  {done ? <Check className="size-4" /> : i + 1}
                </button>
              </li>
            );
          })}
        </ol>
      </nav>
      <div
        ref={actionsRef}
        data-testid="wizard-actions"
        className="ms-auto flex flex-wrap items-center justify-end gap-2"
      />
    </div>
  );
}
