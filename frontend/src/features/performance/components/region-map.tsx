"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/common/card";
import { ErrorState } from "@/components/common/error-state";
import { LeafletMap, type MapMarker } from "@/components/common/leaflet-map";
import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { mapConfig } from "@/config/map";
import { useEnabledFeatures } from "@/features/modules/hooks/use-features";
import {
  CLUSTER_CELL,
  clusterDealers,
  formatMetric,
  MAP_LEVELS,
  metricColor,
  metricModuleOff,
  PERFORMANCE_DEFAULT_COUNTRY,
  PERFORMANCE_METRICS,
  regionRadius,
  valueRange,
  type MapLevel,
} from "@/features/performance/lib/performance";
import {
  performanceKeys,
  performanceService,
  type MapQuery,
  type PerformanceEmptyRegion,
} from "@/features/performance/services/performance.service";
import { useLocale } from "@/providers/locale-provider";

const LEVEL_ZOOM: Record<MapLevel, number> = {
  country: 3,
  province: 5,
  district: 6,
};

/**
 * Region map (TEC-494): level picker (country / province / district),
 * circles sized by the dealer count and colored by the selected metric,
 * an optional clustered dealer point layer and, beside the map, the empty
 * regions (territory without dealers / no territory owner). Region
 * tooltips carry the owning distributor of the territory.
 */
export function RegionMap({ slug, period }: { slug: string; period: string }) {
  const { t, format } = useLocale();
  const enabled = useEnabledFeatures(slug);
  const [level, setLevel] = useState<MapLevel>("province");
  const [metric, setMetric] = useState<string>("services_count");
  const [showDealers, setShowDealers] = useState(false);
  const metrics = PERFORMANCE_METRICS.filter(
    (m) => !metricModuleOff(m, enabled),
  );

  const query: MapQuery = {
    level,
    period,
    metric,
    ...(level === "country" ? {} : { country: PERFORMANCE_DEFAULT_COUNTRY }),
  };
  const regions = useQuery({
    queryKey: performanceKeys.regionMap(query),
    queryFn: () => performanceService.regionMap(query),
  });
  const dealerQuery = { period, metric, country: query.country };
  const dealers = useQuery({
    queryKey: performanceKeys.dealerMap(dealerQuery),
    queryFn: () => performanceService.dealerMap(dealerQuery),
    enabled: showDealers,
  });

  const markers = useMemo<MapMarker[]>(() => {
    const items = (regions.data?.items ?? []).filter(
      (r) => r.latitude != null && r.longitude != null,
    );
    const maxCount = Math.max(0, ...items.map((r) => r.dealer_count));
    const { min, max } = valueRange(items.map((r) => r.metric_avg));
    const out: MapMarker[] = items.map((r) => {
      const parts = [
        r.name,
        t("performance.map.dealer_count", { count: r.dealer_count }),
        `${t(`performance.metrics.${metric}`)}: ${formatMetric(metric, r.metric_avg ?? null, format)}`,
      ];
      if (r.distributor) {
        parts.push(
          t("performance.map.territory_owner", { name: r.distributor.name }),
        );
      }
      return {
        id: `region:${r.level}:${r.id}`,
        lat: r.latitude as number,
        lng: r.longitude as number,
        label: parts.join(" · "),
        radius: regionRadius(r.dealer_count, maxCount),
        color: metricColor(metric, r.metric_avg, min, max),
      };
    });
    if (showDealers) {
      const points = dealers.data?.items ?? [];
      const range = valueRange(points.map((p) => p.metric_value));
      for (const c of clusterDealers(points, CLUSTER_CELL[level])) {
        const single = c.dealers.length === 1 ? c.dealers[0] : null;
        out.push({
          id: c.id,
          lat: c.lat,
          lng: c.lng,
          kind: "point",
          radius: single ? 5 : Math.min(16, 6 + c.dealers.length),
          color: single
            ? metricColor(metric, single.metric_value, range.min, range.max)
            : undefined,
          label: single
            ? single.name
            : t("performance.map.cluster", { count: c.dealers.length }),
        });
      }
    }
    return out;
  }, [
    dealers.data?.items,
    format,
    level,
    metric,
    regions.data?.items,
    showDealers,
    t,
  ]);

  const emptyColumns = useMemo(
    () =>
      [
        createColumn<PerformanceEmptyRegion>({
          accessorKey: "name",
          labelKey: "performance.map.region",
          gridPrimary: true,
        }),
        createColumn<PerformanceEmptyRegion>({
          accessorKey: "empty_reason",
          labelKey: "performance.map.reason",
          cell: ({ row }) =>
            t(`performance.map.reasons.${row.original.empty_reason}`),
        }),
        createColumn<PerformanceEmptyRegion>({
          id: "distributor",
          accessorFn: (row) => row.distributor?.name ?? "",
          labelKey: "performance.ranking.distributor",
          gridSecondary: true,
          cell: ({ row }) => row.original.distributor?.name ?? "—",
        }),
      ] satisfies ColumnDef<PerformanceEmptyRegion, unknown>[],
    [t],
  );

  return (
    <div className="space-y-4" data-testid="performance-region-map">
      <div className="flex flex-wrap items-center gap-3">
        <div
          className="flex gap-1"
          role="tablist"
          aria-label={t("performance.map.level")}
        >
          {MAP_LEVELS.map((l) => (
            <Button
              key={l}
              type="button"
              role="tab"
              size="sm"
              aria-selected={level === l}
              variant={level === l ? "default" : "outline"}
              data-testid={`map-level-${l}`}
              onClick={() => setLevel(l)}
            >
              {t(`performance.map.levels.${l}`)}
            </Button>
          ))}
        </div>
        <Select value={metric} onValueChange={setMetric}>
          <SelectTrigger
            className="w-56"
            aria-label={t("performance.map.metric")}
            data-testid="map-metric"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {metrics.map((m) => (
              <SelectItem key={m} value={m}>
                {t(`performance.metrics.${m}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className="flex items-center gap-2">
          <Switch
            id="performance-map-dealers"
            checked={showDealers}
            onCheckedChange={setShowDealers}
          />
          <Label htmlFor="performance-map-dealers">
            {t("performance.map.dealer_layer")}
          </Label>
        </div>
      </div>
      {regions.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void regions.refetch()}
        />
      ) : (
        <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
          <div className="space-y-2">
            <LeafletMap
              center={mapConfig.defaultCenter}
              zoom={LEVEL_ZOOM[level]}
              markers={markers}
              fitToMarkers
              ariaLabel={t("performance.map.title")}
              className="h-[28rem]"
              testId="performance-leaflet"
            />
            {(regions.data?.missing_coordinates ?? 0) > 0 ? (
              <p className="text-muted-foreground text-xs">
                {t("performance.map.missing_coordinates", {
                  count: regions.data?.missing_coordinates ?? 0,
                })}
              </p>
            ) : null}
          </div>
          <Card>
            <CardHeader>
              <CardTitle>{t("performance.map.empty_regions")}</CardTitle>
            </CardHeader>
            <CardContent>
              <EntityTable
                columns={emptyColumns}
                data={regions.data?.empty_regions ?? []}
                getRowId={(row) => `${row.level}:${row.id}`}
                isLoading={regions.isLoading}
                manual={CLIENT_SIDE_MANUAL}
                features={{
                  persistKey: "tenant-performance-empty-regions-v1",
                  rowSelection: false,
                  columnFilters: false,
                  viewMode: false,
                }}
                emptyTitle={t("performance.map.no_empty_regions")}
              />
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}
