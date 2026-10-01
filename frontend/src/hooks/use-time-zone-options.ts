"use client";

import { useMemo } from "react";

import { supportedTimeZones, timeZoneOffsetLabel } from "@/lib/i18n/format";

export type TimeZoneOption = {
  value: string;
  label: string;
  description?: string;
};

/**
 * IANA zones from Intl.supportedValuesOf("timeZone") with their current
 * UTC offset, for the profile and organization pickers. A saved value the
 * engine does not list (legacy alias) stays selectable.
 */
export function useTimeZoneOptions(current?: string | null): TimeZoneOption[] {
  return useMemo(() => {
    const zones = supportedTimeZones();
    if (current && !zones.includes(current)) zones.unshift(current);
    const now = new Date();
    return zones.map((zone) => ({
      value: zone,
      label: zone.replaceAll("_", " "),
      description: timeZoneOffsetLabel(zone, now),
    }));
  }, [current]);
}
