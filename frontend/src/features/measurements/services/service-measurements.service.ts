import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type ServiceMeasurements = Schemas["ServiceMeasurements"];
export type ServiceMeasurementLink = Schemas["ServiceMeasurementLink"];
export type ServiceMeasurementBrief = Schemas["ServiceMeasurementBrief"];
export type ServiceMeasurementDiff = Schemas["ServiceMeasurementDiff"];
export type ServiceMeasurementDiffPart = Schemas["ServiceMeasurementDiffPart"];
export type MeasurementPhase = ServiceMeasurementLink["phase"];

const enc = encodeURIComponent;

/**
 * Before/after measurements of a service (TEC-296, TEC-297): the links,
 * the match suggestions and candidates, the part diff and the "checked"
 * stamp. Every call needs the measurements module and measurements.link.
 */
export const serviceMeasurementsService = {
  list(serviceUuid: string) {
    return platformRequest<ServiceMeasurements>(
      "GET",
      `/v1/services/${enc(serviceUuid)}/measurements`,
    );
  },
  /**
   * Confirms the measurement already linked to the phase, or links an
   * unlinked one manually (in place of an unconfirmed auto link).
   */
  link(serviceUuid: string, measurementUuid: string, phase: MeasurementPhase) {
    return platformRequest<ServiceMeasurements>(
      "POST",
      `/v1/services/${enc(serviceUuid)}/measurements`,
      { body: { measurement_uuid: measurementUuid, phase } },
    );
  },
  diff(serviceUuid: string) {
    return platformRequest<ServiceMeasurementDiff>(
      "GET",
      `/v1/services/${enc(serviceUuid)}/measurements/diff`,
    );
  },
  markChecked(serviceUuid: string, note: string) {
    return platformRequest<void>(
      "POST",
      `/v1/services/${enc(serviceUuid)}/measurements/checked`,
      { body: { note } },
    );
  },
};

export const serviceMeasurementKeys = {
  all: ["service-measurements"] as const,
  links: (serviceUuid: string) =>
    ["service-measurements", "links", serviceUuid] as const,
  diff: (serviceUuid: string) =>
    ["service-measurements", "diff", serviceUuid] as const,
};

/** The link of a phase, if any. */
export function phaseLink(
  data: ServiceMeasurements | undefined,
  phase: MeasurementPhase,
): ServiceMeasurementLink | undefined {
  return data?.links.find((l) => l.phase === phase);
}

/**
 * Measurements that may take a phase: the phase's suggestions first, then
 * the other unlinked measurements of the VIN, newest first (no duplicates).
 */
export function phaseOptions(
  data: ServiceMeasurements | undefined,
  phase: MeasurementPhase,
): ServiceMeasurementBrief[] {
  if (!data) return [];
  const at = (m: ServiceMeasurementBrief) =>
    Date.parse(m.measured_at ?? m.created_at) || 0;
  const suggested = data.suggestions
    .filter((s) => s.phase === phase)
    .map((s) => s.measurement);
  const rest = [
    ...data.suggestions
      .filter((s) => s.phase !== phase)
      .map((s) => s.measurement),
    ...data.candidates,
  ].sort((a, b) => at(b) - at(a));
  const seen = new Set<string>();
  return [...suggested, ...rest].filter((m) => {
    if (seen.has(m.uuid)) return false;
    seen.add(m.uuid);
    return true;
  });
}
