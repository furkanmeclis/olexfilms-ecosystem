"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import { Loading } from "@/components/common/loading";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  PLACES,
  composePlaceView,
  toInlineSvg,
  type PartMapData,
} from "@/features/measurements/lib/part-map";
import {
  convertThickness,
  partPoints,
  unitDigits,
  type PointValue,
  type ThicknessUnit,
} from "@/features/measurements/lib/point-readings";
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

const UNITS: ThicknessUnit[] = ["um", "mil"];

function PointChip({
  point,
  unit,
}: {
  point: PointValue;
  unit: ThicknessUnit;
}) {
  const { t, format } = useLocale();
  const color = interpretationColor(point.level);
  return (
    <li
      className="flex items-center gap-1.5 rounded-full border px-1.5 py-0.5 text-xs"
      data-testid="point-value"
    >
      <span
        className="inline-flex size-5 shrink-0 items-center justify-center rounded-full text-[10px] font-bold text-gray-900"
        style={{ backgroundColor: color }}
        aria-hidden
      >
        {point.position}
      </span>
      <span className="sr-only">
        {t("measurements.points.point", { n: point.position })}
      </span>
      <span className="font-medium tabular-nums">
        {point.valueUm === null
          ? "—"
          : format.number(convertThickness(point.valueUm, unit), {
              maximumFractionDigits: unitDigits(unit),
              minimumFractionDigits: unitDigits(unit),
            })}
      </span>
      <span className="text-muted-foreground">
        {t(`measurements.units.${unit}`)}
      </span>
    </li>
  );
}

/**
 * The µm value of every numbered point, per part (TEC-1010): outside and
 * inside readings separately, the number in its level color like the map.
 */
export function PointReadings({
  map,
  readings,
  unit,
}: {
  map: PartMapData | null | undefined;
  readings: readonly MapReading[];
  unit: ThicknessUnit;
}) {
  const { t } = useLocale();
  const parts = useMemo(() => partPoints(map, readings), [map, readings]);
  if (parts.length === 0) return null;
  const partLabel = (part: string) => {
    const key = `measurements.parts.${part}`;
    const label = t(key);
    return label === key ? part : label;
  };
  return (
    <div className="grid gap-3 sm:grid-cols-2" data-testid="point-readings">
      {parts.map((p) => (
        <div key={p.part} className="space-y-1.5 rounded-md border p-3">
          <p className="text-sm font-medium">{partLabel(p.part)}</p>
          {(
            [
              ["outside", p.outside],
              ["inside", p.inside],
            ] as const
          ).map(([side, list]) =>
            list.length > 0 ? (
              <div key={side} className="space-y-1">
                {p.inside.length > 0 ? (
                  <p className="text-muted-foreground text-xs">
                    {t(`measurements.points.${side}`)}
                  </p>
                ) : null}
                <ul className="flex flex-wrap gap-1.5" dir="ltr">
                  {list.map((pt) => (
                    <PointChip key={pt.position} point={pt} unit={unit} />
                  ))}
                </ul>
              </div>
            ) : null,
          )}
        </div>
      ))}
    </div>
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
  const [unit, setUnit] = useState<ThicknessUnit>("um");
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
  const unitPicker = (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      value={unit}
      onValueChange={(v) => v && setUnit(v as ThicknessUnit)}
      aria-label={t("measurements.units.label")}
      data-testid="thickness-unit"
    >
      {UNITS.map((u) => (
        <ToggleGroupItem key={u} value={u}>
          {t(`measurements.units.${u}`)}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
  const points = (
    <PointReadings map={map.data} readings={readings} unit={unit} />
  );
  if (!bodyType || map.isError || views.length === 0) {
    return (
      <div className="space-y-4">
        <p
          className="text-muted-foreground text-sm"
          data-testid="part-map-empty"
        >
          {t("measurements.map.unavailable")}
        </p>
        {readings.length > 0 ? (
          <>
            {unitPicker}
            {points}
          </>
        ) : null}
      </div>
    );
  }
  return (
    <div className="space-y-4" data-testid="part-map">
      <div className="flex justify-end">{unitPicker}</div>
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
      {points}
    </div>
  );
}
