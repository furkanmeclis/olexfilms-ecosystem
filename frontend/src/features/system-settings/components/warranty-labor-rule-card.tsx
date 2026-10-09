"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Save } from "lucide-react";
import { useState } from "react";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  hasLaborErrors,
  LABOR_RULES,
  laborChanges,
  toLaborDraft,
  validateLaborDraft,
  type LaborDraft,
  type LaborRule,
  type LaborSettings,
} from "@/features/system-settings/lib/labor-rule";
import { systemSettingsService } from "@/features/system-settings/services/system-settings.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * System settings hub card for the warranty claim labor rule (TEC-337
 * keys): dealer / center / shared, the labor amount in the organization
 * currency and, for `shared`, the share percentage (0–100).
 */
export function WarrantyLaborRuleCard({
  labor,
  canWrite,
  queryKey,
}: {
  labor: LaborSettings;
  canWrite: boolean;
  queryKey: readonly unknown[];
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<LaborDraft>(() => toLaborDraft(labor));
  const [serverError, setServerError] = useState<string | null>(null);
  const errors = validateLaborDraft(draft, labor);
  const changes = laborChanges(draft, labor);
  const invalid = hasLaborErrors(errors);

  const save = useMutation({
    mutationFn: async () => {
      for (const change of changes) {
        await systemSettingsService.put(change.key, change.value);
      }
    },
    onSuccess: () => {
      setServerError(null);
      appToast.success(t("settings.system.toast.saved"));
    },
    onError: (err) => {
      if (isApiError(err) && err.isValidation) {
        setServerError(err.fieldErrors().value ?? err.message);
        return;
      }
      appToast.error(
        isApiError(err) ? err.message : t("settings.system.toast.failed"),
      );
    },
    onSettled: () => void queryClient.invalidateQueries({ queryKey }),
  });

  const patch = (p: Partial<LaborDraft>) => {
    setServerError(null);
    setDraft((d) => ({ ...d, ...p }));
  };
  const disabled = !canWrite || save.isPending;

  return (
    <Card data-testid="warranty-labor-rule">
      <CardHeader>
        <CardTitle>{t("settings.system.labor.title")}</CardTitle>
        <CardDescription>
          {t("settings.system.labor.description")}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-4 md:grid-cols-3"
          onSubmit={(event) => {
            event.preventDefault();
            if (!invalid && changes.length) save.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="labor-rule">
              {t("settings.system.labor.rule")}
            </Label>
            <Select
              value={draft.rule}
              disabled={disabled}
              onValueChange={(value) => patch({ rule: value as LaborRule })}
            >
              <SelectTrigger id="labor-rule" data-testid="labor-rule-select">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {LABOR_RULES.map((rule) => (
                  <SelectItem key={rule} value={rule}>
                    {t(`settings.system.labor.rules.${rule}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-muted-foreground text-xs">
              {t(`settings.system.labor.rules.${draft.rule}_hint`)}
            </p>
          </div>

          {labor.amount ? (
            <div className="space-y-1.5">
              <Label htmlFor="labor-amount">
                {t("settings.system.labor.amount")}
              </Label>
              <Input
                id="labor-amount"
                data-testid="labor-amount"
                inputMode="decimal"
                value={draft.amount}
                disabled={disabled}
                aria-invalid={errors.amount ? true : undefined}
                onChange={(e) => patch({ amount: e.target.value })}
              />
              {errors.amount ? (
                <p role="alert" className="text-destructive text-sm">
                  {t(`settings.system.labor.errors.amount_${errors.amount}`)}
                </p>
              ) : (
                <p className="text-muted-foreground text-xs">
                  {t("settings.system.labor.amount_hint")}
                </p>
              )}
            </div>
          ) : null}

          {draft.rule === "shared" && labor.percent ? (
            <div className="space-y-1.5">
              <Label htmlFor="labor-percent">
                {t("settings.system.labor.percent")}
              </Label>
              <Input
                id="labor-percent"
                data-testid="labor-percent"
                type="number"
                inputMode="numeric"
                min={0}
                max={100}
                value={draft.percent}
                disabled={disabled}
                aria-invalid={errors.percent ? true : undefined}
                onChange={(e) => patch({ percent: e.target.value })}
              />
              {errors.percent ? (
                <p
                  role="alert"
                  className="text-destructive text-sm"
                  data-testid="labor-percent-error"
                >
                  {t(`settings.system.labor.errors.percent_${errors.percent}`)}
                </p>
              ) : (
                <p className="text-muted-foreground text-xs">
                  {t("settings.system.labor.percent_hint")}
                </p>
              )}
            </div>
          ) : null}

          {serverError ? (
            <p role="alert" className="text-destructive text-sm md:col-span-3">
              {serverError}
            </p>
          ) : null}
          {canWrite ? (
            <div className="flex justify-end md:col-span-3">
              <Button
                type="submit"
                size="sm"
                data-testid="labor-save"
                disabled={save.isPending || invalid || changes.length === 0}
              >
                <Save className="size-4" />
                {t("settings.system.save")}
              </Button>
            </div>
          ) : null}
        </form>
      </CardContent>
    </Card>
  );
}
