"use client";

import { keepPreviousData, useQueries } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ChevronLeft, ChevronRight, Flame } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import type { CalendarView } from "@/features/appointments/components/appointment-calendar";
import {
  aggregateOccupancy,
  OCCUPANCY_LEVELS,
  OCCUPANCY_TONE,
  occupancyPercent,
  type OccupancyLevel,
  type OccupancyRow,
} from "@/features/appointments/lib/occupancy";
import { addDays, todayIn, weekDays } from "@/features/appointments/lib/time";
import {
  appointmentKeys,
  appointmentsService,
} from "@/features/appointments/services/appointments.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const OCCUPANCY_PERSIST_KEY = "tenant-appointment-occupancy-v1";

const NUMERIC = {
  headerClassName: "text-end",
  cellClassName: "text-end tabular-nums",
};

/** Heat badge of one occupancy rate (shared by the table and the widget). */
export function OccupancyBadge({
  level,
  percent,
  className,
}: {
  level: OccupancyLevel;
  percent: number | null;
  className?: string;
}) {
  const { t, format } = useLocale();
  return (
    <span
      data-testid="occupancy-badge"
      data-level={level}
      title={t(`appointments.occupancy.levels.${level}`)}
      className={cn(
        "inline-flex min-w-14 justify-center rounded-md border px-2 py-0.5 text-xs font-semibold tabular-nums",
        OCCUPANCY_TONE[level],
        className,
      )}
    >
      {percent === null
        ? "—"
        : t("appointments.occupancy.percent", {
            value: format.number(percent),
          })}
    </span>
  );
}

/**
 * Tenant > Appointment occupancy (TEC-328 on GET /v1/appointments/
 * occupancy): capacity, bookings and occupancy rate of every organization
 * of the read scope (center: the network, distributor: its dealers) for a
 * day or a week, with a heat color per row. The endpoint returns the full
 * per-day aggregate, so the DataTable sorts, searches and filters in the
 * browser; a week sums seven day requests. A row opens the organization's
 * calendar read-only.
 */
export function OccupancyPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const org = useActiveOrganization(slug);
  const canRead = can(permissions.appointments.read);
  const networkOrg = org?.type === "center" || org?.type === "distributor";
  const [view, setView] = useState<CalendarView>("day");
  const [anchor, setAnchor] = useState(() => todayIn(format.timeZone));

  const days = useMemo(
    () => (view === "week" ? weekDays(anchor) : [anchor]),
    [view, anchor],
  );
  const queries = useQueries({
    queries: days.map((date) => ({
      queryKey: appointmentKeys.occupancy(date),
      queryFn: () => appointmentsService.occupancy(date),
      enabled: canRead && networkOrg,
      placeholderData: keepPreviousData,
    })),
  });
  const isError = queries.some((q) => q.isError);
  const isLoading = queries.some((q) => q.isLoading);
  const dataKey = queries.map((q) => q.dataUpdatedAt).join(",");
  const rows = useMemo(
    () =>
      isLoading ? [] : aggregateOccupancy(queries.map((q) => q.data ?? [])),
    // queries is a new array every render; dataKey tracks the data.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [dataKey, isLoading],
  );

  const firstDay = days[0] ?? anchor;
  const lastDay = days[days.length - 1] ?? anchor;
  const calendarHref = (row: OccupancyRow) =>
    routes.tenant.appointments.organizationCalendar(
      slug,
      row.organization_uuid,
      { name: row.organization_name, date: anchor, view },
    );

  const columns = useMemo(() => {
    const count = (
      key: "capacity" | "occupied" | "remaining",
      labelKey: string,
    ) =>
      createColumn<OccupancyRow>({
        accessorKey: key,
        labelKey,
        enableSorting: true,
        filterVariant: "number-range",
        filterFn: (row, id, value: unknown) =>
          inRange(row.getValue<number>(id), value),
        meta: NUMERIC,
        cell: ({ row }) => format.number(row.original[key]),
      });
    return [
      createColumn<OccupancyRow>({
        id: "organization",
        accessorFn: (row) => row.organization_name,
        labelKey: "appointments.occupancy.columns.organization",
        enableSorting: true,
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span
            className="font-medium"
            data-testid="occupancy-row"
            data-uuid={row.original.organization_uuid}
          >
            {row.original.organization_name}
          </span>
        ),
      }),
      count("capacity", "appointments.occupancy.columns.capacity"),
      count("occupied", "appointments.occupancy.columns.occupied"),
      count("remaining", "appointments.occupancy.columns.remaining"),
      createColumn<OccupancyRow>({
        id: "rate",
        // Organizations without capacity sort below 0 %.
        accessorFn: (row) => occupancyPercent(row.rate) ?? -1,
        labelKey: "appointments.occupancy.columns.rate",
        enableSorting: true,
        gridSecondary: true,
        filterVariant: "number-range",
        filterFn: (row, id, value: unknown) =>
          inRange(row.getValue<number>(id), value),
        meta: NUMERIC,
        cell: ({ row }) => (
          <OccupancyBadge
            level={row.original.level}
            percent={occupancyPercent(row.original.rate)}
          />
        ),
      }),
      createColumn<OccupancyRow>({
        id: "level",
        accessorFn: (row) => row.level,
        labelKey: "appointments.occupancy.columns.level",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: OCCUPANCY_LEVELS.map((value) => ({
          value,
          label: t(`appointments.occupancy.levels.${value}`),
        })),
        cell: ({ row }) =>
          t(`appointments.occupancy.levels.${row.original.level}`),
      }),
    ] as ColumnDef<OccupancyRow, unknown>[];
  }, [format, t]);

  const title = t("appointments.occupancy.title");
  const header = (
    <PageHeader
      title={title}
      description={t("appointments.occupancy.description")}
      icon={<Flame className="size-6" />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("appointments.title"),
          href: routes.tenant.appointments.calendar(slug),
        },
        { label: title },
      ]}
    />
  );

  if (!canRead || (org && !networkOrg)) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("appointments.occupancy.forbidden")}
        />
      </div>
    );
  }

  const step = view === "week" ? 7 : 1;
  return (
    <div className="space-y-6">
      {header}
      <Card>
        <CardContent className="flex flex-wrap items-center gap-2 pt-6">
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label={t("appointments.calendar.previous")}
            onClick={() => setAnchor(addDays(anchor, -step))}
          >
            <ChevronLeft className="size-4 rtl:-scale-x-100" />
          </Button>
          <DatePicker
            id="occupancy-date"
            className="w-44"
            value={anchor}
            onChange={(v) => v && setAnchor(v)}
          />
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label={t("appointments.calendar.next")}
            onClick={() => setAnchor(addDays(anchor, step))}
          >
            <ChevronRight className="size-4 rtl:-scale-x-100" />
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => setAnchor(todayIn(format.timeZone))}
          >
            {t("appointments.calendar.today")}
          </Button>
          <ToggleGroup
            type="single"
            variant="outline"
            value={view}
            aria-label={t("appointments.occupancy.range_label")}
            onValueChange={(v) => {
              if (v === "day" || v === "week") setView(v);
            }}
          >
            <ToggleGroupItem value="day" data-testid="occupancy-view-day">
              {t("appointments.calendar.day")}
            </ToggleGroupItem>
            <ToggleGroupItem value="week" data-testid="occupancy-view-week">
              {t("appointments.calendar.week")}
            </ToggleGroupItem>
          </ToggleGroup>
          <span
            className="text-muted-foreground ms-auto text-sm"
            data-testid="occupancy-range"
          >
            {view === "week"
              ? `${format.date(`${firstDay}T12:00:00Z`)} – ${format.date(`${lastDay}T12:00:00Z`)}`
              : format.date(`${anchor}T12:00:00Z`)}
          </span>
        </CardContent>
      </Card>

      <OccupancyLegend />

      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.organization_uuid}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={isLoading}
        isError={isError}
        onRetry={() => {
          for (const q of queries) if (q.isError) void q.refetch();
        }}
        initialState={{
          pagination: { pageIndex: 0, pageSize: 50 },
          sorting: [{ id: "rate", desc: true }],
        }}
        emptyTitle={t("appointments.occupancy.empty")}
        emptyDescription={t("appointments.occupancy.empty_description")}
        features={{
          persistKey: OCCUPANCY_PERSIST_KEY,
          rowSelection: false,
        }}
        onRowClick={(row) => router.push(calendarHref(row))}
        rowActions={(row) => (
          <Button asChild variant="ghost" size="sm">
            <Link
              href={calendarHref(row)}
              data-testid="occupancy-open-calendar"
            >
              {t("appointments.occupancy.open_calendar")}
            </Link>
          </Button>
        )}
      />
    </div>
  );
}

function inRange(v: number, value: unknown): boolean {
  const [min, max] = Array.isArray(value)
    ? (value as (number | undefined)[])
    : [];
  if (min !== undefined && v < min) return false;
  if (max !== undefined && v > max) return false;
  return true;
}

/** Color key of the heat levels. */
export function OccupancyLegend() {
  const { t } = useLocale();
  return (
    <ul
      className="text-muted-foreground flex flex-wrap items-center gap-3 text-xs"
      aria-label={t("appointments.occupancy.legend")}
    >
      {OCCUPANCY_LEVELS.map((level) => (
        <li key={level} className="flex items-center gap-1.5">
          <span
            className={cn("size-3 rounded-sm border", OCCUPANCY_TONE[level])}
            aria-hidden
          />
          {t(`appointments.occupancy.levels.${level}`)}
        </li>
      ))}
    </ul>
  );
}
