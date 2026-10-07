"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { AlertTriangle, CheckCircle2, ExternalLink, Gauge } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { routes } from "@/config/routes";
import { MeasurementPdfButton } from "@/features/measurements/components/measurement-pdf-button";
import {
  phaseLink,
  phaseOptions,
  serviceMeasurementKeys,
  serviceMeasurementsService,
  type MeasurementPhase,
  type ServiceMeasurementBrief,
  type ServiceMeasurementDiffPart,
  type ServiceMeasurements,
} from "@/features/measurements/services/service-measurements.service";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const SERVICE_MEASUREMENT_DIFF_PERSIST_KEY =
  "tenant-service-measurement-diff-v1";

const PHASES: MeasurementPhase[] = ["before", "after"];

/** Message key of a link error code (TEC-296). */
function linkErrorKey(error: unknown): string | null {
  if (!isApiError(error)) return null;
  switch (error.code) {
    case "MEASUREMENT_PHASE_TAKEN":
      return "measurements.link_errors.phase_taken";
    case "MEASUREMENT_ALREADY_LINKED":
      return "measurements.link_errors.already_linked";
    case "MEASUREMENT_VIN_MISMATCH":
      return "measurements.link_errors.vin_mismatch";
    case "MEASUREMENT_VIN_PENDING":
      return "measurements.pdf.vin_pending";
    default:
      return null;
  }
}

/** Toasts a link failure: a known code, the API message or a fallback. */
export function toastLinkError(
  error: unknown,
  t: (key: string) => string,
): void {
  const key = linkErrorKey(error);
  if (key) appToast.error(t(key));
  else if (isApiError(error)) appToast.error(error.message);
  else appToast.error(t("measurements.link_errors.failed"));
}

function measuredAt(m: ServiceMeasurementBrief): string {
  return m.measured_at ?? m.created_at;
}

/** One measurement as a selectable row (wizard and the change dialog). */
export function MeasurementOption({
  measurement,
  selected,
  suggested,
  disabled,
  onSelect,
}: {
  measurement: ServiceMeasurementBrief;
  selected: boolean;
  suggested?: boolean;
  disabled?: boolean;
  onSelect: () => void;
}) {
  const { t, format } = useLocale();
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      disabled={disabled}
      onClick={onSelect}
      data-testid="measurement-option"
      data-uuid={measurement.uuid}
      className={cn(
        "flex w-full items-center justify-between gap-3 rounded-lg border p-3 text-start text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50",
        selected ? "border-primary bg-primary/5" : "hover:bg-accent",
      )}
    >
      <span className="min-w-0 space-y-0.5">
        <span className="block font-medium">
          {format.dateTime(measuredAt(measurement))}
        </span>
        <span className="text-muted-foreground block text-xs">
          {t("measurements.columns.device")}:{" "}
          <span dir="ltr">{measurement.device_serial ?? "—"}</span>
        </span>
      </span>
      {suggested ? (
        <Badge variant="secondary">{t("measurements.picker.suggested")}</Badge>
      ) : null}
    </button>
  );
}

function PickerDialog({
  serviceUuid,
  phase,
  data,
  current,
  onClose,
}: {
  serviceUuid: string;
  phase: MeasurementPhase;
  data: ServiceMeasurements;
  current: string | undefined;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const options = phaseOptions(data, phase);
  const suggested = new Set(
    data.suggestions
      .filter((s) => s.phase === phase)
      .map((s) => s.measurement.uuid),
  );
  const [selected, setSelected] = useState<string | null>(null);

  const link = useMutation({
    mutationFn: (uuid: string) =>
      serviceMeasurementsService.link(serviceUuid, uuid, phase),
    onSuccess: (next) => {
      queryClient.setQueryData(serviceMeasurementKeys.links(serviceUuid), next);
      void queryClient.invalidateQueries({
        queryKey: serviceMeasurementKeys.diff(serviceUuid),
      });
      appToast.success(t("measurements.picker.saved"));
      onClose();
    },
    onError: (error) => toastLinkError(error, t),
  });

  return (
    <Dialog open onOpenChange={(o) => (!o ? onClose() : undefined)}>
      <DialogContent data-testid="measurement-picker">
        <DialogHeader>
          <DialogTitle>
            {t("measurements.picker.title", {
              phase: t(`measurements.phase.${phase}`),
            })}
          </DialogTitle>
          <DialogDescription>
            {t("measurements.picker.description")}
          </DialogDescription>
        </DialogHeader>
        {options.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="measurement-picker-empty"
          >
            {t("measurements.picker.empty")}
          </p>
        ) : (
          <div className="space-y-2" role="radiogroup">
            {options.map((m) => (
              <MeasurementOption
                key={m.uuid}
                measurement={m}
                selected={selected === m.uuid}
                suggested={suggested.has(m.uuid)}
                disabled={m.uuid === current}
                onSelect={() => setSelected(m.uuid)}
              />
            ))}
          </div>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!selected || link.isPending}
            onClick={() => selected && link.mutate(selected)}
            data-testid="measurement-picker-save"
          >
            {t("measurements.picker.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function PhaseCard({
  slug,
  serviceUuid,
  phase,
  data,
  canEdit,
  canOpen,
  onPick,
}: {
  slug: string;
  serviceUuid: string;
  phase: MeasurementPhase;
  data: ServiceMeasurements;
  canEdit: boolean;
  canOpen: boolean;
  onPick: () => void;
}) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const link = phaseLink(data, phase);
  const pending = Boolean(link && !link.confirmed);

  const confirm = useMutation({
    mutationFn: (uuid: string) =>
      serviceMeasurementsService.link(serviceUuid, uuid, phase),
    onSuccess: (next) => {
      queryClient.setQueryData(serviceMeasurementKeys.links(serviceUuid), next);
      appToast.success(t("measurements.service.confirmed_toast"));
    },
    onError: (error) => toastLinkError(error, t),
  });

  return (
    <div
      className="space-y-3 rounded-lg border p-4"
      data-testid={`measurement-phase-${phase}`}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="font-medium">{t(`measurements.phase.${phase}`)}</p>
        {link ? (
          <div className="flex flex-wrap gap-1">
            <Badge variant="outline">
              {t(`measurements.service.source_${link.link_source}`)}
            </Badge>
            {pending ? (
              <Badge variant="warning" data-testid="measurement-pending">
                {t("measurements.service.pending")}
              </Badge>
            ) : (
              <Badge variant="success">
                {t("measurements.service.confirmed")}
              </Badge>
            )}
          </div>
        ) : null}
      </div>
      {link ? (
        <>
          <dl className="grid gap-2 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-muted-foreground text-xs">
                {t("measurements.columns.measured_at")}
              </dt>
              <dd>{format.dateTime(measuredAt(link.measurement))}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground text-xs">
                {t("measurements.columns.device")}
              </dt>
              <dd dir="ltr" className="font-mono text-xs">
                {link.measurement.device_serial ?? "—"}
              </dd>
            </div>
          </dl>
          {link.confirmed && link.confirmed_at ? (
            <p className="text-muted-foreground text-xs">
              {t("measurements.service.confirmed_by", {
                name: link.confirmed_by?.name ?? "—",
                date: format.dateTime(link.confirmed_at),
              })}
            </p>
          ) : null}
          <div className="flex flex-wrap gap-2">
            {canEdit && pending ? (
              <>
                <Button
                  type="button"
                  size="sm"
                  disabled={confirm.isPending}
                  onClick={() => confirm.mutate(link.measurement.uuid)}
                  data-testid={`measurement-confirm-${phase}`}
                >
                  <CheckCircle2 className="size-4" />
                  {t("measurements.service.confirm")}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={onPick}
                  data-testid={`measurement-change-${phase}`}
                >
                  {t("measurements.service.change")}
                </Button>
              </>
            ) : null}
            <MeasurementPdfButton
              uuid={link.measurement.uuid}
              status={link.measurement.status}
              size="sm"
              testId={`measurement-pdf-${phase}`}
            />
            {canOpen ? (
              <Button asChild size="sm" variant="link">
                <Link
                  href={routes.tenant.measurements.detail(
                    slug,
                    link.measurement.uuid,
                  )}
                >
                  <ExternalLink className="size-4" />
                  {t("measurements.service.open_measurement")}
                </Link>
              </Button>
            ) : null}
          </div>
        </>
      ) : (
        <div className="space-y-2">
          <p
            className="text-muted-foreground text-sm"
            data-testid={`measurement-empty-${phase}`}
          >
            {t("measurements.service.no_link")}
          </p>
          {canEdit && phaseOptions(data, phase).length > 0 ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={onPick}
              data-testid={`measurement-select-${phase}`}
            >
              {t("measurements.service.select")}
            </Button>
          ) : null}
        </div>
      )}
    </div>
  );
}

function CheckDialog({
  serviceUuid,
  onClose,
}: {
  serviceUuid: string;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const [note, setNote] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const empty = note.trim() === "";

  const save = useMutation({
    mutationFn: () =>
      serviceMeasurementsService.markChecked(serviceUuid, note.trim()),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: serviceMeasurementKeys.diff(serviceUuid),
      });
      appToast.success(t("measurements.check.saved"));
      onClose();
    },
    onError: () => appToast.error(t("measurements.check.failed")),
  });

  return (
    <Dialog open onOpenChange={(o) => (!o ? onClose() : undefined)}>
      <DialogContent data-testid="measurement-check-dialog">
        <DialogHeader>
          <DialogTitle>{t("measurements.check.title")}</DialogTitle>
          <DialogDescription>
            {t("measurements.check.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="measurement-check-note">
            {t("measurements.check.note")}
          </Label>
          <Textarea
            id="measurement-check-note"
            value={note}
            maxLength={1000}
            onChange={(e) => setNote(e.target.value)}
            aria-invalid={submitted && empty}
            data-testid="measurement-check-note"
          />
          {submitted && empty ? (
            <p className="text-destructive text-sm">
              {t("measurements.check.note_required")}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={save.isPending}
            onClick={() => {
              setSubmitted(true);
              if (!empty) save.mutate();
            }}
            data-testid="measurement-check-save"
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Number cell of a decimal micron string ("—" when absent). */
function Microns({ value }: { value: string | null }) {
  const { format } = useLocale();
  if (value === null) return <>—</>;
  const n = Number(value);
  return (
    <span className="tabular-nums" dir="ltr">
      {Number.isFinite(n) ? format.number(n) : value}
    </span>
  );
}

const num = (v: string | null) => (v === null ? null : Number(v));

function DiffTable({ parts }: { parts: ServiceMeasurementDiffPart[] }) {
  const { t } = useLocale();
  const partLabel = useMemo(
    () => (part: string) => {
      const key = `measurements.parts.${part}`;
      const label = t(key);
      return label === key ? part : label;
    },
    [t],
  );
  const placeLabel = useMemo(
    () => (place: string) => {
      const key = `measurements.places.${place}`;
      const label = t(key);
      return label === key ? place : label;
    },
    [t],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<ServiceMeasurementDiffPart>({
          id: "part",
          accessorFn: (row) => partLabel(row.part_type),
          labelKey: "measurements.diff.part",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: Array.from(new Set(parts.map((p) => p.part_type)))
            .map((p) => ({ value: partLabel(p), label: partLabel(p) }))
            .sort((a, b) => a.label.localeCompare(b.label)),
          cell: ({ row }) => (
            <span data-testid="diff-row" data-part={row.original.part_type}>
              {partLabel(row.original.part_type)}
            </span>
          ),
        }),
        createColumn<ServiceMeasurementDiffPart>({
          id: "place",
          accessorFn: (row) => row.place_id,
          labelKey: "measurements.diff.place",
          enableSorting: true,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: Array.from(new Set(parts.map((p) => p.place_id))).map(
            (p) => ({ value: p, label: placeLabel(p) }),
          ),
          cell: ({ row }) => placeLabel(row.original.place_id),
        }),
        createColumn<ServiceMeasurementDiffPart>({
          id: "before",
          accessorFn: (row) => num(row.before.average_um),
          labelKey: "measurements.diff.before",
          enableSorting: true,
          cell: ({ row }) => <Microns value={row.original.before.average_um} />,
        }),
        createColumn<ServiceMeasurementDiffPart>({
          id: "after",
          accessorFn: (row) => num(row.after.average_um),
          labelKey: "measurements.diff.after",
          enableSorting: true,
          cell: ({ row }) => <Microns value={row.original.after.average_um} />,
        }),
        createColumn<ServiceMeasurementDiffPart>({
          id: "diff",
          accessorFn: (row) => num(row.diff_um),
          labelKey: "measurements.diff.diff",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => <Microns value={row.original.diff_um} />,
        }),
        createColumn<ServiceMeasurementDiffPart>({
          id: "expected",
          accessorFn: (row) => num(row.expected_um),
          labelKey: "measurements.diff.expected",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.expected_status === "missing" ? (
              <span className="text-muted-foreground text-xs">
                {t("measurements.diff.expected_missing")}
              </span>
            ) : (
              <span className="inline-flex items-center gap-1">
                <Microns value={row.original.expected_um} />
                {row.original.expected_status === "incomplete" ? (
                  <Badge variant="outline" className="text-xs">
                    {t("measurements.diff.expected_incomplete")}
                  </Badge>
                ) : null}
              </span>
            ),
        }),
        createColumn<ServiceMeasurementDiffPart>({
          id: "deviation",
          accessorFn: (row) => (row.deviation ? "yes" : "no"),
          labelKey: "measurements.diff.deviation",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: [
            {
              value: "yes",
              label: "yes",
              labelKey: "measurements.diff.deviation_yes",
            },
            {
              value: "no",
              label: "no",
              labelKey: "measurements.diff.deviation_no",
            },
          ],
          cell: ({ row }) =>
            row.original.deviation ? (
              <Badge variant="danger" data-testid="diff-deviation">
                {t("measurements.diff.deviation_yes")}
              </Badge>
            ) : (
              <Badge variant="success">
                {t("measurements.diff.deviation_no")}
              </Badge>
            ),
        }),
      ] as ColumnDef<ServiceMeasurementDiffPart, unknown>[],
    [partLabel, parts, placeLabel, t],
  );

  return (
    <EntityTable
      columns={columns}
      data={parts}
      getRowId={(row) => `${row.place_id}:${row.part_type}`}
      manual={CLIENT_SIDE_MANUAL}
      initialState={{
        pagination: { pageIndex: 0, pageSize: 20 },
        sorting: [{ id: "deviation", desc: true }],
      }}
      emptyTitle={t("measurements.diff.empty")}
      emptyDescription=""
      features={{ persistKey: SERVICE_MEASUREMENT_DIFF_PERSIST_KEY }}
    />
  );
}

/**
 * Service measurement section (TEC-300, panel only): the before/after
 * links with "Onayla" / "Değiştir" of a pending auto match, the part diff
 * table, the "Kontrol gerekli" band with "Kontrol edildi" and the PDF of
 * each linked measurement. The caller renders it only for a service that
 * expects a measurement while the module and measurements.link are on.
 */
export function ServiceMeasurementsSection({
  slug,
  serviceUuid,
  editable,
  canOpen,
}: {
  slug: string;
  serviceUuid: string;
  /** False for a cancelled service: links can no longer change. */
  editable: boolean;
  /** measurements.read: the cards link to the measurement page. */
  canOpen: boolean;
}) {
  const { t, format } = useLocale();
  const [picking, setPicking] = useState<MeasurementPhase | null>(null);
  const [checking, setChecking] = useState(false);

  const links = useQuery({
    queryKey: serviceMeasurementKeys.links(serviceUuid),
    queryFn: () => serviceMeasurementsService.list(serviceUuid),
  });
  const diff = useQuery({
    queryKey: serviceMeasurementKeys.diff(serviceUuid),
    queryFn: () => serviceMeasurementsService.diff(serviceUuid),
  });

  const d = diff.data;
  const checkRequired = Boolean(d?.check_required && !d.checked_at);

  let body;
  if (links.isLoading) {
    body = <Loading />;
  } else if (links.isError || !links.data) {
    body = (
      <ErrorState
        title={t("measurements.service.load_failed")}
        onRetry={() => void links.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  } else {
    const data = links.data;
    body = (
      <div className="space-y-4">
        {checkRequired ? (
          <Alert variant="destructive" data-testid="measurement-check-band">
            <AlertTriangle />
            <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
              <span>{t("measurements.check.banner")}</span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setChecking(true)}
                data-testid="measurement-check-open"
              >
                {t("measurements.check.action")}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {d?.check_required && d.checked_at ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="measurement-checked"
          >
            {t("measurements.check.done", {
              date: format.dateTime(d.checked_at),
            })}
          </p>
        ) : null}
        {data.vin ? null : (
          <p className="text-muted-foreground text-sm">
            {t("measurements.service.no_vin")}
          </p>
        )}
        <div className="grid gap-4 lg:grid-cols-2">
          {PHASES.map((phase) => (
            <PhaseCard
              key={phase}
              slug={slug}
              serviceUuid={serviceUuid}
              phase={phase}
              data={data}
              canEdit={editable}
              canOpen={canOpen}
              onPick={() => setPicking(phase)}
            />
          ))}
        </div>
        <div className="space-y-2" data-testid="measurement-diff">
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <p className="font-medium">{t("measurements.diff.title")}</p>
            {d ? (
              <p className="text-muted-foreground text-xs">
                {t("measurements.diff.tolerance", {
                  value: format.number(Number(d.tolerance_um)),
                })}
              </p>
            ) : null}
          </div>
          {diff.isError ? (
            <ErrorState
              title={t("measurements.service.load_failed")}
              onRetry={() => void diff.refetch()}
              retryLabel={t("common.retry")}
            />
          ) : d ? (
            <DiffTable parts={d.parts} />
          ) : (
            <Loading />
          )}
        </div>
        {picking ? (
          <PickerDialog
            serviceUuid={serviceUuid}
            phase={picking}
            data={data}
            current={phaseLink(data, picking)?.measurement.uuid}
            onClose={() => setPicking(null)}
          />
        ) : null}
        {checking ? (
          <CheckDialog
            serviceUuid={serviceUuid}
            onClose={() => setChecking(false)}
          />
        ) : null}
      </div>
    );
  }

  return (
    <Card data-testid="detail-measurements">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="text-muted-foreground">
            <Gauge className="size-4" />
          </span>
          {t("measurements.service.title")}
        </CardTitle>
      </CardHeader>
      <CardContent>{body}</CardContent>
    </Card>
  );
}
