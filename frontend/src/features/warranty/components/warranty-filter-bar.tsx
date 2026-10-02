"use client";

import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  ALL,
  DAYS_LEFT_PRESETS,
  EMPTY_WARRANTY_FILTERS,
  hasWarrantyFilters,
  WARRANTY_STATUSES,
  type WarrantyListFilters,
} from "@/features/warranty/lib/warranty-list";
import { useLocale } from "@/providers/locale-provider";

/**
 * Status, "ends within N days" and search filters shared by the panel and
 * portal lists (TEC-191); `extra` adds panel-only fields (product).
 */
export function WarrantyFilterBar({
  filters,
  onChange,
  extra,
  searchPlaceholder,
}: {
  filters: WarrantyListFilters;
  onChange: (patch: Partial<WarrantyListFilters>) => void;
  extra?: ReactNode;
  searchPlaceholder: string;
}) {
  const { t } = useLocale();
  return (
    <div className="space-y-4" data-testid="warranty-filters">
      <div
        className="flex flex-wrap gap-2"
        role="group"
        aria-label={t("warranty.list.status")}
      >
        {[ALL, ...WARRANTY_STATUSES].map((status) => {
          const active = filters.status === status;
          return (
            <Button
              key={status}
              type="button"
              size="sm"
              variant={active ? "default" : "outline"}
              aria-pressed={active}
              data-status={status}
              onClick={() =>
                onChange({ status: status as WarrantyListFilters["status"] })
              }
            >
              {status === ALL
                ? t("warranty.list.all_statuses")
                : t(`warranty.status.${status}`)}
            </Button>
          );
        })}
      </div>
      <div
        className="flex flex-wrap items-center gap-2"
        role="group"
        aria-label={t("warranty.list.ends_within")}
      >
        <span className="text-muted-foreground text-sm">
          {t("warranty.list.ends_within")}
        </span>
        {[ALL, ...DAYS_LEFT_PRESETS].map((days) => {
          const active = filters.endsWithin === days;
          return (
            <Button
              key={days}
              type="button"
              size="sm"
              variant={active ? "secondary" : "ghost"}
              aria-pressed={active}
              data-ends-within={days}
              onClick={() =>
                onChange({
                  endsWithin: days as WarrantyListFilters["endsWithin"],
                })
              }
            >
              {days === ALL
                ? t("warranty.list.ends_any")
                : t("warranty.list.ends_days", { days })}
            </Button>
          );
        })}
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="warranty-search">{t("warranty.list.search")}</Label>
          <Input
            id="warranty-search"
            type="search"
            value={filters.q}
            maxLength={100}
            placeholder={searchPlaceholder}
            onChange={(e) => onChange({ q: e.target.value })}
          />
        </div>
        {extra}
      </div>
      {hasWarrantyFilters(filters) ? (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          data-testid="clear-filters"
          onClick={() => onChange(EMPTY_WARRANTY_FILTERS)}
        >
          {t("warranty.list.clear_filters")}
        </Button>
      ) : null}
    </div>
  );
}
