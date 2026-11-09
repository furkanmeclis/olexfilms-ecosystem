import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type ReportEnvelope = Schemas["ReportEnvelope"];
export type ReportPoint = Schemas["ReportPoint"];

/** TEC-1072: one period drives every card and chart of the center home. */
export type CenterPeriod = "today" | "7d" | "30d" | "90d" | "12m";

export const CENTER_PERIODS: readonly CenterPeriod[] = [
  "today",
  "7d",
  "30d",
  "90d",
  "12m",
];
export const CENTER_DEFAULT_PERIOD: CenterPeriod = "30d";

/** Number of top products and recent services shown. */
export const CENTER_TOP_LIMIT = 10;
export const CENTER_RECENT_LIMIT = 8;

/**
 * Start of period at now in the browser's time zone (today: local
 * midnight; 12m: first day of the month eleven months back, like the
 * reports module), as the created_from bound of the recent services list.
 */
export function centerPeriodStart(period: CenterPeriod, now: Date): Date {
  const d = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  switch (period) {
    case "today":
      return d;
    case "7d":
      d.setDate(d.getDate() - 6);
      return d;
    case "30d":
      d.setDate(d.getDate() - 29);
      return d;
    case "90d":
      d.setDate(d.getDate() - 89);
      return d;
    case "12m":
      return new Date(now.getFullYear(), now.getMonth() - 11, 1);
  }
}

/** Card value of the overview envelope (null when the card is absent). */
export function overviewCard(
  env: ReportEnvelope | undefined,
  key: string,
): number | null {
  const point = env?.series
    .find((s) => s.key === "cards")
    ?.points.find((p) => p.key === key);
  return point ? point.value : null;
}

/**
 * Center home statistics through the reports module (TEC-495): the
 * overview cards (network, services, warranties) and the most used
 * products ranking, in the brand scope of the center.
 */
export const centerOverviewService = {
  overview(period: CenterPeriod) {
    return platformRequest<ReportEnvelope>("GET", "/v1/reports/overview", {
      query: { period },
    });
  },
  topProducts(period: CenterPeriod) {
    return platformRequest<ReportEnvelope>(
      "GET",
      "/v1/reports/services.top_products",
      { query: { period, limit: CENTER_TOP_LIMIT } },
    );
  },
};
