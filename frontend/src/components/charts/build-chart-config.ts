import type { ChartConfig } from "@/components/ui/chart";

const PALETTE = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
] as const;

type SeriesInput = {
  key: string;
  label?: string;
  color?: string;
};

/** Convenience helper — assign chart palette colors in order */
export function buildChartConfig(series: SeriesInput[]): ChartConfig {
  return Object.fromEntries(
    series.map((item, index) => [
      item.key,
      {
        label: item.label ?? item.key,
        color: item.color ?? PALETTE[index % PALETTE.length],
      },
    ]),
  ) satisfies ChartConfig;
}
