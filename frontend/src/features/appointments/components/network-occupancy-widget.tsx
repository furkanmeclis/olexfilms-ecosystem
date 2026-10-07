"use client";

import { useQuery } from "@tanstack/react-query";
import { Flame } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { OccupancyBadge } from "@/features/appointments/components/occupancy-page";
import {
  aggregateOccupancy,
  occupancyPercent,
  summarizeOccupancy,
} from "@/features/appointments/lib/occupancy";
import { todayIn } from "@/features/appointments/lib/time";
import {
  appointmentKeys,
  appointmentsService,
} from "@/features/appointments/services/appointments.service";
import { useFeature } from "@/features/modules/hooks/use-features";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Busiest organizations listed under the totals. */
const TOP_ROWS = 5;

/**
 * TEC-328: today's network occupancy on the tenant home (center and
 * distributor): network rate with its heat color, capacity / booked /
 * remaining totals, organizations per heat level and the busiest ones.
 * Rendered only with appointments.read and the appointments module on.
 */
export function NetworkOccupancyWidget({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const feature = useFeature(slug, "appointments");
  const visible =
    can(permissions.appointments.read) &&
    feature.enabled &&
    (org?.type === "center" || org?.type === "distributor");
  const today = todayIn(format.timeZone);

  const query = useQuery({
    queryKey: appointmentKeys.occupancy(today),
    queryFn: () => appointmentsService.occupancy(today),
    enabled: visible,
    staleTime: 60_000,
  });

  const rows = useMemo(
    () => aggregateOccupancy([query.data ?? []]),
    [query.data],
  );
  const summary = useMemo(() => summarizeOccupancy(rows), [rows]);
  const top = useMemo(
    () =>
      rows
        .filter((r) => r.rate !== null)
        .sort((a, b) => (b.rate ?? 0) - (a.rate ?? 0))
        .slice(0, TOP_ROWS),
    [rows],
  );

  if (!visible) return null;

  return (
    <Card data-testid="network-occupancy-widget">
      <CardHeader className="flex flex-row items-start justify-between gap-2">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2">
            <Flame className="size-4" />
            {t("appointments.occupancy.widget.title")}
          </CardTitle>
          <CardDescription>
            {t("appointments.occupancy.widget.description")}
          </CardDescription>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link
            href={routes.tenant.appointments.occupancy(slug)}
            data-testid="network-occupancy-widget-link"
          >
            {t("appointments.occupancy.widget.open")}
          </Link>
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        {query.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void query.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : query.isPending ? (
          <div className="grid gap-3 sm:grid-cols-4">
            <Skeleton className="h-16" />
            <Skeleton className="h-16" />
            <Skeleton className="h-16" />
            <Skeleton className="h-16" />
          </div>
        ) : rows.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="network-occupancy-widget-empty"
          >
            {t("appointments.occupancy.empty")}
          </p>
        ) : (
          <>
            <dl className="grid gap-3 sm:grid-cols-4">
              <div className="bg-muted/40 rounded-lg border p-3">
                <dt className="text-muted-foreground text-xs">
                  {t("appointments.occupancy.columns.rate")}
                </dt>
                <dd className="pt-1" data-testid="network-occupancy-rate">
                  <OccupancyBadge
                    level={summary.level}
                    percent={occupancyPercent(summary.rate)}
                    className="text-base"
                  />
                </dd>
              </div>
              <Stat
                label={t("appointments.occupancy.columns.capacity")}
                value={format.number(summary.capacity)}
                testId="network-occupancy-capacity"
              />
              <Stat
                label={t("appointments.occupancy.columns.occupied")}
                value={format.number(summary.occupied)}
                testId="network-occupancy-occupied"
              />
              <Stat
                label={t("appointments.occupancy.columns.remaining")}
                value={format.number(summary.remaining)}
                testId="network-occupancy-remaining"
              />
            </dl>
            <p
              className="text-muted-foreground text-xs"
              data-testid="network-occupancy-levels"
            >
              {t("appointments.occupancy.widget.breakdown", {
                organizations: format.number(summary.organizations),
                full: format.number(summary.levels.full),
                high: format.number(summary.levels.high),
              })}
            </p>
            {top.length > 0 ? (
              <ul
                className="divide-y text-sm"
                data-testid="network-occupancy-top"
              >
                {top.map((row) => (
                  <li
                    key={row.organization_uuid}
                    className="flex items-center justify-between gap-3 py-1.5"
                  >
                    <Link
                      href={routes.tenant.appointments.organizationCalendar(
                        slug,
                        row.organization_uuid,
                        {
                          name: row.organization_name,
                          date: today,
                          view: "day",
                        },
                      )}
                      className="min-w-0 truncate font-medium hover:underline"
                    >
                      {row.organization_name}
                    </Link>
                    <span className="text-muted-foreground flex items-center gap-2 text-xs whitespace-nowrap tabular-nums">
                      {t("appointments.calendar.capacity", {
                        occupied: format.number(row.occupied),
                        capacity: format.number(row.capacity),
                      })}
                      <OccupancyBadge
                        level={row.level}
                        percent={occupancyPercent(row.rate)}
                      />
                    </span>
                  </li>
                ))}
              </ul>
            ) : null}
          </>
        )}
      </CardContent>
    </Card>
  );
}

function Stat({
  label,
  value,
  testId,
}: {
  label: string;
  value: string;
  testId: string;
}) {
  return (
    <div className="bg-muted/40 rounded-lg border p-3">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-xl font-semibold tabular-nums" data-testid={testId}>
        {value}
      </dd>
    </div>
  );
}
