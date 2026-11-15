"use client";

import { useMemo } from "react";

import { useLocale } from "@/providers/locale-provider";

import type { DurationLabels } from "./format";

export function useDurationLabels(): DurationLabels {
  const { t } = useLocale();
  return useMemo(
    () => ({
      day: t("chart.duration.day"),
      hour: t("chart.duration.hour"),
      minute: t("chart.duration.minute"),
      second: t("chart.duration.second"),
    }),
    [t],
  );
}
