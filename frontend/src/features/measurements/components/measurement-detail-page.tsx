"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { FileDown, Gauge } from "lucide-react";
import Link from "next/link";
import { useMemo, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  MeasurementStatusBadge,
  MeasurementVin,
} from "@/features/measurements/components/measurement-status-badge";
import { MeasurementVinForm } from "@/features/measurements/components/measurement-vin-form";
import { PartMap } from "@/features/measurements/components/part-map";
import { useMeasurementPdf } from "@/features/measurements/hooks/use-measurement-pdf";
import {
  interpretationColor,
  interpretationLevel,
  LEGEND,
} from "@/features/measurements/lib/thresholds";
import {
  measurementKeys,
  measurementsService,
  type MeasurementTire,
  type MeasurementValue,
} from "@/features/measurements/services/measurements.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const MEASUREMENT_VALUES_PERSIST_KEY = "tenant-measurement-values-v1";
export const MEASUREMENT_TIRES_PERSIST_KEY = "tenant-measurement-tires-v1";

const dash = (v: string | number | null | undefined) =>
  v === null || v === undefined || v === "" ? "—" : v;

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm break-words">{children}</dd>
    </div>
  );
}

/** Legend key of an interpretation code (4 and 5 share thick putty). */
function interpretationKey(value: number | null): string {
  const level = interpretationLevel(value);
  if (level === 5) return "thick_putty";
  return LEGEND.find((l) => l.level === level)?.key ?? "unknown";
}

function tireSize(tire: MeasurementTire): string {
  const width = tire.width ?? "";
  const profile = tire.profile ? `/${tire.profile}` : "";
  const diameter = tire.diameter ? ` R${tire.diameter}` : "";
  return `${width}${profile}${diameter}`.trim();
}

/**
 * Measurement detail (TEC-299): header, the NexPTG part map with the color
 * legend, the reading and tire tables, "PDF indir" (accepted only) and the
 * VIN completion form of a vin_pending measurement (measurements.link).
 */
export function MeasurementDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.measurements.read);
  const canLink = can(permissions.measurements.link);
  const canServices = can(permissions.services.read);
  const pdf = useMeasurementPdf();

  const query = useQuery({
    queryKey: measurementKeys.detail(uuid),
    queryFn: () => measurementsService.get(uuid),
    enabled: canRead && uuid !== "",
  });
  const m = query.data;

  const partLabel = useMemo(
    () => (part: string) => {
      const key = `measurements.parts.${part}`;
      const label = t(key);
      return label === key ? part : label;
    },
    [t],
  );

  const valueColumns = useMemo(
    () =>
      [
        createColumn<MeasurementValue>({
          id: "part",
          accessorFn: (row) => row.part_type,
          labelKey: "measurements.values.part",
          enableSorting: true,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: Array.from(
            new Set((m?.values ?? []).map((v) => v.part_type)),
          ).map((p) => ({ value: p, label: partLabel(p) })),
          cell: ({ row }) => partLabel(row.original.part_type),
        }),
        createColumn<MeasurementValue>({
          accessorKey: "position",
          labelKey: "measurements.values.position",
          enableSorting: true,
          cell: ({ row }) => dash(row.original.position),
        }),
        createColumn<MeasurementValue>({
          accessorKey: "is_inside",
          labelKey: "measurements.values.inside",
          enableSorting: true,
          filterVariant: "boolean",
          cell: ({ row }) =>
            row.original.is_inside ? t("common.yes") : t("common.no"),
        }),
        createColumn<MeasurementValue>({
          id: "value_um",
          accessorFn: (row) =>
            row.value_um === null ? null : Number(row.value_um),
          labelKey: "measurements.values.value",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums" dir="ltr">
              {row.original.value_um === null
                ? "—"
                : format.number(Number(row.original.value_um))}
            </span>
          ),
        }),
        createColumn<MeasurementValue>({
          id: "interpretation",
          accessorFn: (row) => interpretationKey(row.interpretation),
          labelKey: "measurements.values.interpretation",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: LEGEND.map((l) => ({
            value: l.key,
            label: t(`measurements.interpretations.${l.key}`),
          })),
          cell: ({ row }) => (
            <span className="flex items-center gap-2">
              <span
                className="inline-block size-3 shrink-0 rounded-full border"
                style={{
                  backgroundColor: interpretationColor(
                    row.original.interpretation,
                  ),
                }}
                aria-hidden
              />
              {t(
                `measurements.interpretations.${interpretationKey(row.original.interpretation)}`,
              )}
            </span>
          ),
        }),
        createColumn<MeasurementValue>({
          accessorKey: "substrate_type",
          labelKey: "measurements.values.substrate",
          enableSorting: true,
          cell: ({ row }) => dash(row.original.substrate_type),
        }),
      ] as ColumnDef<MeasurementValue, unknown>[],
    [format, m?.values, partLabel, t],
  );

  const tireColumns = useMemo(
    () =>
      [
        createColumn<MeasurementTire>({
          accessorKey: "section",
          labelKey: "measurements.tires.position",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => dash(row.original.section),
        }),
        createColumn<MeasurementTire>({
          id: "size",
          accessorFn: (row) => tireSize(row),
          labelKey: "measurements.tires.size",
          enableSorting: true,
          cell: ({ getValue }) => (
            <span dir="ltr">{dash(String(getValue()))}</span>
          ),
        }),
        createColumn<MeasurementTire>({
          accessorKey: "maker",
          labelKey: "measurements.tires.maker",
          enableSorting: true,
          cell: ({ row }) => dash(row.original.maker),
        }),
        createColumn<MeasurementTire>({
          accessorKey: "season",
          labelKey: "measurements.tires.season",
          enableSorting: true,
          cell: ({ row }) => dash(row.original.season),
        }),
        createColumn<MeasurementTire>({
          id: "tread",
          accessorFn: (row) =>
            [row.tread_depth_1_mm, row.tread_depth_2_mm]
              .filter((v) => v !== null)
              .join(" / "),
          labelKey: "measurements.tires.tread_depth",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ getValue }) => (
            <span className="tabular-nums" dir="ltr">
              {dash(String(getValue()))}
            </span>
          ),
        }),
      ] as ColumnDef<MeasurementTire, unknown>[],
    [],
  );

  const title = t("measurements.detail.title");
  const header = (
    <PageHeader
      title={m?.vin || m?.plate || title}
      icon={<Gauge className="size-6" />}
      description={t("measurements.detail.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("measurements.list.title"),
          href: routes.tenant.measurements.list(slug),
        },
        { label: title },
      ]}
      actions={
        m ? (
          <Button
            type="button"
            variant="outline"
            disabled={m.status !== "accepted" || pdf.isPending}
            onClick={() => pdf.mutate(m.uuid)}
            data-testid="measurement-pdf"
          >
            <FileDown className="size-4" />
            {pdf.isPending
              ? t("measurements.pdf.preparing")
              : t("measurements.pdf.download")}
          </Button>
        ) : null
      }
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("measurements.list.forbidden")}
        />
      </div>
    );
  }
  if (query.isLoading) return <Loading />;
  if (query.isError || !m) {
    const notFound = isApiError(query.error) && query.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={
            notFound
              ? t("measurements.detail.not_found")
              : t("common.error_generic")
          }
          onRetry={notFound ? undefined : () => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }

  const pending = m.status === "vin_pending";
  return (
    <div className="space-y-6" data-testid="measurement-detail">
      {header}
      <Card>
        <CardContent className="pt-6">
          <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Field label={t("measurements.columns.measured_at")}>
              {format.dateTime(m.measured_at ?? m.created_at)}
            </Field>
            <Field label={t("measurements.columns.vin")}>
              <MeasurementVin vin={m.vin} />
            </Field>
            <Field label={t("measurements.columns.plate")}>
              <span className="font-mono" dir="ltr">
                {dash(m.plate)}
              </span>
            </Field>
            <Field label={t("measurements.columns.status")}>
              <MeasurementStatusBadge status={m.status} />
            </Field>
            <Field label={t("measurements.columns.device")}>
              <span className="font-mono" dir="ltr">
                {dash(m.device?.serial ?? m.device_serial)}
              </span>
              {m.device?.label ? ` · ${m.device.label}` : ""}
            </Field>
            <Field label={t("measurements.detail.body_type")}>
              {dash(m.body_type)}
            </Field>
            <Field label={t("measurements.columns.organization")}>
              {m.organization.name}
            </Field>
            <Field label={t("measurements.columns.service")}>
              {m.service ? (
                <span className="flex flex-wrap items-center gap-2">
                  {canServices ? (
                    <Link
                      href={routes.tenant.services.detail(slug, m.service.uuid)}
                      className="font-mono hover:underline"
                      dir="ltr"
                    >
                      {m.service.service_no}
                    </Link>
                  ) : (
                    <span className="font-mono" dir="ltr">
                      {m.service.service_no}
                    </span>
                  )}
                  <Badge variant="outline">
                    {t(`measurements.phase.${m.service.phase}`)}
                  </Badge>
                </span>
              ) : (
                t("measurements.service.unlinked")
              )}
            </Field>
          </dl>
        </CardContent>
      </Card>

      {pending && canLink ? <MeasurementVinForm uuid={m.uuid} /> : null}
      {pending && !canLink ? (
        <p className="text-muted-foreground text-sm">
          {t("measurements.vin.no_permission")}
        </p>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>{t("measurements.map.title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <PartMap bodyType={m.body_type} readings={m.values} />
        </CardContent>
      </Card>

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">
          {t("measurements.values.title")}
        </h2>
        <EntityTable
          columns={valueColumns}
          data={m.values}
          manual={CLIENT_SIDE_MANUAL}
          emptyTitle={t("measurements.values.empty")}
          features={{ persistKey: MEASUREMENT_VALUES_PERSIST_KEY }}
        />
      </section>

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">
          {t("measurements.tires.title")}
        </h2>
        <EntityTable
          columns={tireColumns}
          data={m.tires}
          manual={CLIENT_SIDE_MANUAL}
          emptyTitle={t("measurements.tires.empty")}
          features={{ persistKey: MEASUREMENT_TIRES_PERSIST_KEY }}
        />
      </section>
    </div>
  );
}
