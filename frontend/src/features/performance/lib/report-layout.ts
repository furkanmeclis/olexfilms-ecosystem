import type { ReportLayoutInput } from "@/features/performance/services/performance.service";

/** A dashboard widget while the layout is edited. */
export type EditableWidget = {
  id: string;
  report: string;
  period: string | null;
  granularity: string | null;
};

type WidgetPeriod = ReportLayoutInput["widgets"][number]["period"];
type WidgetGranularity = ReportLayoutInput["widgets"][number]["granularity"];

/**
 * PUT /v1/reports/layout body: the full list in display order (the server
 * derives sort_order from the array order).
 */
export function layoutInput(
  widgets: readonly EditableWidget[],
): ReportLayoutInput {
  return {
    version: 1,
    widgets: widgets.map((w) => ({
      id: w.id,
      report: w.report,
      period: (w.period ?? null) as WidgetPeriod,
      granularity: (w.granularity ?? null) as WidgetGranularity,
    })),
  };
}
