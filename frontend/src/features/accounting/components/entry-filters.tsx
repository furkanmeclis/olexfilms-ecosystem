"use client";

import { X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  type SelectOption,
} from "@/features/accounting/components/shared";
import {
  activeFilterCount,
  EMPTY_ENTRY_FILTERS,
  type EntryFilterValues,
} from "@/features/accounting/lib/form";
import {
  ACCOUNTING_DIRECTIONS,
  ENTRY_SOURCE_TYPES,
} from "@/features/accounting/services/accounting.service";
import { useLocale } from "@/providers/locale-provider";

/** Ledger filters: date range, cari, source and direction (TEC-176). */
export function EntryFilters({
  value,
  cariOptions,
  onChange,
}: {
  value: EntryFilterValues;
  cariOptions: SelectOption[];
  onChange: (next: EntryFilterValues) => void;
}) {
  const { t } = useLocale();
  const set = (key: keyof EntryFilterValues, v: string) =>
    onChange({ ...value, [key]: v });
  const count = activeFilterCount(value);

  return (
    <div
      className="grid items-end gap-3 sm:grid-cols-2 lg:grid-cols-[repeat(5,minmax(0,1fr))_auto]"
      data-testid="entry-filters"
    >
      <div className="grid gap-1.5">
        <Label htmlFor="entry-filter-from">
          {t("accounting.filters.date_from")}
        </Label>
        <DatePicker
          id="entry-filter-from"
          value={value.date_from}
          placeholder={t("accounting.filters.any_date")}
          onChange={(v) => set("date_from", v)}
        />
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="entry-filter-to">
          {t("accounting.filters.date_to")}
        </Label>
        <DatePicker
          id="entry-filter-to"
          value={value.date_to}
          placeholder={t("accounting.filters.any_date")}
          onChange={(v) => set("date_to", v)}
        />
      </div>
      <NativeSelect
        id="entry-filter-cari"
        name="cari_uuid"
        label={t("accounting.fields.cari")}
        value={value.cari_uuid}
        placeholder={t("accounting.filters.all_cari")}
        onChange={(v) => set("cari_uuid", v)}
        options={cariOptions}
      />
      <NativeSelect
        id="entry-filter-source"
        name="source_type"
        label={t("accounting.fields.source")}
        value={value.source_type}
        placeholder={t("accounting.filters.all_sources")}
        onChange={(v) => set("source_type", v)}
        options={ENTRY_SOURCE_TYPES.map((s) => ({
          value: s,
          label: t(`accounting.sources.${s}`),
        }))}
      />
      <NativeSelect
        id="entry-filter-direction"
        name="direction"
        label={t("accounting.fields.direction")}
        value={value.direction}
        placeholder={t("accounting.filters.all_directions")}
        onChange={(v) => set("direction", v)}
        options={ACCOUNTING_DIRECTIONS.map((d) => ({
          value: d,
          label: t(`accounting.directions.${d}`),
        }))}
      />
      <Button
        type="button"
        variant="ghost"
        disabled={count === 0}
        onClick={() => onChange(EMPTY_ENTRY_FILTERS)}
        data-testid="entry-filters-clear"
      >
        <X className="size-4" />
        {t("accounting.filters.clear")}
      </Button>
    </div>
  );
}
