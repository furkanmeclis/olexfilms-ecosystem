import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type StatsPeriod = Schemas["StatsPeriod"];
export type StatsGroup = Schemas["StatsGroup"];
export type TopVehicle = Schemas["TopVehicle"];
export type TopVehicles = Schemas["TopVehicles"];

export const STATS_PERIODS: readonly StatsPeriod[] = [
  "30d",
  "90d",
  "12m",
  "all",
];
export const STATS_GROUPS: readonly StatsGroup[] = ["model", "brand"];

/**
 * TEC-151 center dashboard statistics: top-10 car brands / models by
 * completed services of the domain brand (GET /v1/stats/top-vehicle-models,
 * services.read with a brand-wide grant, or super_admin).
 */
export const vehicleStatsService = {
  topVehicleModels(params: { period: StatsPeriod; group: StatsGroup }) {
    return platformRequest<TopVehicles>("GET", "/v1/stats/top-vehicle-models", {
      query: params,
    });
  },
};
