"use client";

import { useMemo } from "react";

import {
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  FormActions,
} from "@/components/forms";
import { EntityDrawer } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import {
  purgeRuleFormSchema,
  type PurgeRuleFormValues,
} from "@/features/logs/schemas/purge-rule-form";
import type { PurgeRule } from "@/features/logs/services/logs.service";
import { useLocale } from "@/providers/locale-provider";

const LEVELS = ["debug", "warn", "error"] as const;

const INTERVALS = [
  { value: "5", labelKey: "logs.rules.intervals.5" },
  { value: "15", labelKey: "logs.rules.intervals.15" },
  { value: "30", labelKey: "logs.rules.intervals.30" },
  { value: "60", labelKey: "logs.rules.intervals.60" },
  { value: "180", labelKey: "logs.rules.intervals.180" },
  { value: "360", labelKey: "logs.rules.intervals.360" },
  { value: "720", labelKey: "logs.rules.intervals.720" },
  { value: "1440", labelKey: "logs.rules.intervals.1440" },
  { value: "10080", labelKey: "logs.rules.intervals.10080" },
] as const;

type PurgeRuleFormDrawerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  rule?: PurgeRule | null;
  isSubmitting?: boolean;
  onSubmit: (values: PurgeRuleFormValues) => Promise<void> | void;
};

function LevelsField({
  value,
  onChange,
}: {
  value: string[];
  onChange: (next: string[]) => void;
}) {
  const { t } = useLocale();

  return (
    <div className="space-y-2">
      <Label>{t("logs.rules.fields.levels")}</Label>
      <div className="flex flex-wrap gap-3">
        {LEVELS.map((level) => {
          const checked = value.includes(level);
          return (
            <label key={level} className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={checked}
                onCheckedChange={(next) => {
                  if (next) onChange([...value, level]);
                  else onChange(value.filter((item) => item !== level));
                }}
              />
              {t(`logs.levels.${level}`)}
            </label>
          );
        })}
      </div>
    </div>
  );
}

export function PurgeRuleFormDrawer({
  open,
  onOpenChange,
  rule,
  isSubmitting,
  onSubmit,
}: PurgeRuleFormDrawerProps) {
  const { t } = useLocale();
  const schema = useMemo(() => purgeRuleFormSchema(t), [t]);
  const mode = rule ? "edit" : "create";

  const defaultValues = useMemo<PurgeRuleFormValues>(
    () => ({
      name: rule?.name ?? "",
      enabled: rule?.enabled ?? true,
      levels: rule?.levels?.length ? rule.levels : ["debug"],
      source: rule?.source ?? "",
      message_contains: rule?.message_contains ?? "",
      older_than_hours: rule?.older_than_hours ?? 168,
      interval_minutes: String(rule?.interval_minutes ?? 1440),
    }),
    [rule],
  );

  const intervalOptions = INTERVALS.map((item) => ({
    value: item.value,
    label: t(item.labelKey),
  }));

  return (
    <EntityDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={
        mode === "create"
          ? t("logs.rules.create_title")
          : t("logs.rules.edit_title")
      }
      description={t("logs.rules.form_description")}
      size="lg"
    >
      <AppForm
        schema={schema}
        defaultValues={defaultValues}
        onSubmit={async (values) => {
          await onSubmit(values);
          onOpenChange(false);
        }}
      >
        {(form) => (
          <div className="space-y-4">
            <AppInput name="name" label={t("logs.rules.fields.name")} />
            <AppSwitch name="enabled" label={t("logs.rules.fields.enabled")} />
            <LevelsField
              value={form.watch("levels")}
              onChange={(next) =>
                form.setValue("levels", next, { shouldDirty: true })
              }
            />
            <AppInput
              name="source"
              label={t("logs.rules.fields.source")}
              description={t("logs.rules.fields.source_hint")}
            />
            <AppInput
              name="message_contains"
              label={t("logs.rules.fields.message_contains")}
            />
            <AppInput
              name="older_than_hours"
              label={t("logs.rules.fields.older_than_hours")}
              type="number"
            />
            <AppSelect
              name="interval_minutes"
              label={t("logs.rules.fields.interval_minutes")}
              options={intervalOptions}
            />
            <FormActions>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={isSubmitting}>
                {t("common.save")}
              </Button>
            </FormActions>
          </div>
        )}
      </AppForm>
    </EntityDrawer>
  );
}
