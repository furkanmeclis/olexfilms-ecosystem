"use client";

import { useMemo } from "react";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { recentPeriods } from "@/features/ai-admin/lib/quota";
import { useLocale } from "@/providers/locale-provider";

/** Month (YYYY-MM, UTC) picker of the quota period; last 12 months. */
export function PeriodSelect({
  value,
  onChange,
}: {
  value: string;
  onChange: (period: string) => void;
}) {
  const { t, locale } = useLocale();
  const periods = useMemo(() => recentPeriods(12), []);
  const label = useMemo(() => {
    const fmt = new Intl.DateTimeFormat(locale, {
      year: "numeric",
      month: "long",
      timeZone: "UTC",
    });
    return (period: string) => fmt.format(new Date(`${period}-01T00:00:00Z`));
  }, [locale]);

  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger
        className="h-8 w-44"
        aria-label={t("ai_admin.period")}
        data-testid="ai-period-select"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {periods.map((period) => (
          <SelectItem key={period} value={period}>
            {label(period)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
