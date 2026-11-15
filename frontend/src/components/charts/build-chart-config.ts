import { categoricalColor } from "@/components/analytics/palette";
import type { ChartConfig } from "@/components/ui/chart";

type SeriesInput = {
  key: string;
  label?: string;
  color?: string;
};

/** Convenience helper — assign categorical palette colors in order (TEC-1211: never cycled). */
export function buildChartConfig(series: SeriesInput[]): ChartConfig {
  return Object.fromEntries(
    series.map((item, index) => [
      item.key,
      {
        label: item.label ?? item.key,
        color: item.color ?? categoricalColor(index),
      },
    ]),
  ) satisfies ChartConfig;
}
