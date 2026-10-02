import { permissions, type PermissionScope } from "@/config/permissions";
import type {
  StatsGroup,
  TopVehicle,
} from "@/features/vehicle-catalog/services/vehicle-stats.service";

import { logoVersion } from "./images";

/** One bar of the top-10 chart. */
export type TopVehicleBar = {
  key: string;
  label: string;
  count: number;
  brandUuid: string;
  brandName: string;
  logoVersion: string | null;
};

/** API rows → chart rows (label: "Brand Model" for group=model). */
export function toTopVehicleBars(
  items: readonly TopVehicle[],
  group: StatsGroup,
): TopVehicleBar[] {
  return items.map((item) => {
    const model = group === "model" ? item.car_model : null;
    return {
      key: model?.uuid ?? item.car_brand.uuid,
      label: model
        ? `${item.car_brand.name} ${model.name}`
        : item.car_brand.name,
      count: item.service_count,
      brandUuid: item.car_brand.uuid,
      brandName: item.car_brand.name,
      logoVersion: item.car_brand.has_logo
        ? logoVersion(item.car_brand.logo_url)
        : null,
    };
  });
}

/** Shortens a tick label so it fits the category axis. */
export function truncateLabel(label: string, max = 18): string {
  const chars = Array.from(label);
  return chars.length > max ? `${chars.slice(0, max - 1).join("")}…` : label;
}

/**
 * Mirrors the API gate of /v1/stats/top-vehicle-models: a brand-wide
 * services.read grant in the center organization, or scope all
 * (super_admin, brand of the selected organization).
 */
export function canSeeTopVehicles(
  scopeFor: (permission: string) => PermissionScope | undefined,
  orgType: string | null | undefined,
): boolean {
  const scope = scopeFor(permissions.services.read);
  return scope === "all" || (scope === "brand" && orgType === "center");
}
