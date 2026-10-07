"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { Loading } from "@/components/common/loading";
import {
  PLACES,
  composePlaceView,
  toInlineSvg,
} from "@/features/measurements/lib/part-map";
import {
  LEGEND,
  interpretationColor,
  type MapReading,
} from "@/features/measurements/lib/thresholds";
import {
  measurementKeys,
  measurementsService,
} from "@/features/measurements/services/measurements.service";
import { useLocale } from "@/providers/locale-provider";

/** Color legend of the interpretation levels (thresholds.ts). */
export function InterpretationLegend() {
  const { t } = useLocale();
  return (
    <ul
      className="flex flex-wrap gap-x-4 gap-y-2 text-xs"
      aria-label={t("measurements.map.legend")}
      data-testid="interpretation-legend"
    >
      {LEGEND.map(({ level, key }) => (
        <li key={key} className="flex items-center gap-2">
          <span
            className="inline-block size-3 shrink-0 rounded-full border"
            style={{ backgroundColor: interpretationColor(level) }}
            aria-hidden
          />
          {t(`measurements.interpretations.${key}`)}
        </li>
      ))}
    </ul>
  );
}

/**
 * NexPTG part map of a measurement: the four views of the body type with
 * each part in its mean level color and the numbered points, colored in
 * the browser from the raw body type SVGs. The vehicle shape is never
 * mirrored in RTL (`dir="ltr"`).
 */
export function PartMap({
  bodyType,
  readings,
}: {
  bodyType: string | null;
  readings: readonly MapReading[];
}) {
  const { t } = useLocale();
  const map = useQuery({
    queryKey: measurementKeys.partMap(bodyType ?? ""),
    queryFn: () => measurementsService.partMap(bodyType ?? ""),
    enabled: Boolean(bodyType),
    staleTime: Infinity,
    retry: false,
  });

  const views = useMemo(() => {
    if (!map.data) return [];
    return PLACES.flatMap((place) => {
      const composed = composePlaceView(map.data, place, readings);
      const svg = composed ? toInlineSvg(composed) : null;
      return svg ? [{ place, svg }] : [];
    });
  }, [map.data, readings]);

  if (bodyType && map.isLoading) return <Loading />;
  if (!bodyType || map.isError || views.length === 0) {
    return (
      <p className="text-muted-foreground text-sm" data-testid="part-map-empty">
        {t("measurements.map.unavailable")}
      </p>
    );
  }
  return (
    <div className="space-y-4" data-testid="part-map">
      <div className="grid gap-4 md:grid-cols-2" dir="ltr">
        {views.map(({ place, svg }) => (
          <figure key={place} className="rounded-md border p-2">
            {/* Static body type SVG from the API; overlays are numbers and
                fixed colors only (part-map.ts escapes them). */}
            <div
              className="[&_svg]:h-auto [&_svg]:w-full"
              data-place={place}
              dangerouslySetInnerHTML={{ __html: svg }}
            />
            <figcaption
              className="text-muted-foreground mt-1 text-center text-xs"
              dir="auto"
            >
              {t(`measurements.places.${place}`)}
            </figcaption>
          </figure>
        ))}
      </div>
      <InterpretationLegend />
    </div>
  );
}
