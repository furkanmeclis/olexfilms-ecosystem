import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type HomeOverview = Schemas["HomeOverview"];
export type HomeOverviewPeriod = Schemas["StatsPeriod"];
export type OverviewVehicle = Schemas["OverviewVehicle"];
export type OverviewProduct = Schemas["OverviewProduct"];
export type OverviewRecentService = Schemas["OverviewRecentService"];

export const HOME_OVERVIEW_PERIODS: readonly HomeOverviewPeriod[] = [
  "30d",
  "90d",
  "12m",
  "all",
];

/** The dealer home opens on the last 90 days. */
export const HOME_OVERVIEW_DEFAULT_PERIOD: HomeOverviewPeriod = "90d";

/**
 * TEC-868 dealer home "Analiz": most serviced car models, most used
 * products and the latest completed services of the active organization
 * (GET /v1/stats/overview, services.read).
 */
export const homeOverviewService = {
  get(period: HomeOverviewPeriod) {
    return platformRequest<HomeOverview>("GET", "/v1/stats/overview", {
      query: { period },
    });
  },
};
