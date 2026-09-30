"use client";

import type { ReactNode } from "react";

import { AppDrawer } from "@/components/dialogs/app-drawer";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { useLocale } from "@/providers/locale-provider";

export type EntityFilterOption = {
  value: string;
  label: string;
  labelKey?: string;
};

export type EntityFilterDef = {
  key: string;
  labelKey: string;
  /**
   * When false, control is rendered disabled (API param not available yet).
   * Defaults to true.
   */
  supported?: boolean;
  variant: "text" | "select" | "faceted" | "date" | "date-range";
  options?: EntityFilterOption[];
  placeholderKey?: string;
};

export type EntityFilterValues = Record<string, string | string[] | undefined>;

type EntityFiltersProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title?: string;
  description?: string;
  filters: EntityFilterDef[];
  values: EntityFilterValues;
  onChange: (key: string, value: string | string[] | undefined) => void;
  onReset: () => void;
  onApply?: () => void;
};

function FilterControl({
  def,
  value,
  onChange,
  t,
}: {
  def: EntityFilterDef;
  value: string | string[] | undefined;
  onChange: (value: string | string[] | undefined) => void;
  t: (key: string) => string;
}) {
  const disabled = def.supported === false;
  const label = t(def.labelKey);

  if (def.variant === "select" || def.variant === "faceted") {
    const ANY = "__any__";
    const raw = Array.isArray(value) ? value[0] : value;
    return (
      <div className="space-y-2">
        <Label htmlFor={`entity-filter-${def.key}`}>{label}</Label>
        <Select
          value={raw ?? ANY}
          onValueChange={(next) => onChange(next === ANY ? undefined : next)}
          disabled={disabled}
        >
          <SelectTrigger id={`entity-filter-${def.key}`}>
            <SelectValue placeholder={t("entity.filter_any")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ANY}>{t("entity.filter_any")}</SelectItem>
            {(def.options ?? []).map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.labelKey ? t(opt.labelKey) : opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {disabled ? (
          <p className="text-muted-foreground text-xs">
            {t("entity.filter_unsupported")}
          </p>
        ) : null}
      </div>
    );
  }

  if (def.variant === "date" || def.variant === "date-range") {
    const raw = typeof value === "string" ? value : "";
    return (
      <div className="space-y-2">
        <Label htmlFor={`entity-filter-${def.key}`}>{label}</Label>
        <DatePicker
          id={`entity-filter-${def.key}`}
          value={raw}
          disabled={disabled}
          onChange={(next) => onChange(next || undefined)}
        />
        {disabled ? (
          <p className="text-muted-foreground text-xs">
            {t("entity.filter_unsupported")}
          </p>
        ) : null}
      </div>
    );
  }

  const raw = typeof value === "string" ? value : "";
  return (
    <div className="space-y-2">
      <Label htmlFor={`entity-filter-${def.key}`}>{label}</Label>
      <Input
        id={`entity-filter-${def.key}`}
        value={raw}
        disabled={disabled}
        placeholder={def.placeholderKey ? t(def.placeholderKey) : undefined}
        onChange={(e) => onChange(e.target.value || undefined)}
      />
      {disabled ? (
        <p className="text-muted-foreground text-xs">
          {t("entity.filter_unsupported")}
        </p>
      ) : null}
    </div>
  );
}

/**
 * Reusable advanced filters drawer for entity list pages.
 */
export function EntityFilters({
  open,
  onOpenChange,
  title,
  description,
  filters,
  values,
  onChange,
  onReset,
  onApply,
}: EntityFiltersProps) {
  const { t } = useLocale();

  const footer: ReactNode = (
    <div className="flex items-center justify-end gap-2">
      <Button type="button" variant="outline" onClick={onReset}>
        {t("entity.reset_filters")}
      </Button>
      <Button
        type="button"
        onClick={() => {
          onApply?.();
          onOpenChange(false);
        }}
      >
        {t("entity.apply_filters")}
      </Button>
    </div>
  );

  return (
    <AppDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={title ?? t("entity.filters")}
      description={description ?? t("entity.filters_description")}
      size="md"
      footer={footer}
    >
      <div className="space-y-4">
        {filters.map((def) => (
          <FilterControl
            key={def.key}
            def={def}
            value={values[def.key]}
            onChange={(next) => onChange(def.key, next)}
            t={t}
          />
        ))}
      </div>
    </AppDrawer>
  );
}

export function countActiveFilters(values: EntityFilterValues): number {
  return Object.values(values).filter((v) => {
    if (v == null || v === "") return false;
    if (Array.isArray(v)) return v.length > 0;
    return true;
  }).length;
}
