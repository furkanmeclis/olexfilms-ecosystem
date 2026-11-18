"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircle2,
  ClipboardCheck,
  Play,
  ShieldCheck,
  XCircle,
} from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Progress } from "@/components/ui/progress";
import {
  countAccuracyPercent,
  countDurationMinutes,
  countProgressPercent,
  countResultBadges,
  countResultTone,
} from "@/features/warehouse/lib/count-progress";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CountLinesTable,
  CountScansTable,
  ExpectedLocationsTable,
} from "@/features/warehouse/components/count-tables";
import { countScopeText } from "@/features/warehouse/components/counts-page";
import { Info } from "@/features/warehouse/components/list-controls";
import { LocationPicker } from "@/features/warehouse/components/location-picker";
import { ScanInput } from "@/features/warehouse/components/scan-input";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  approveBody,
  canCancelCount,
  canStart,
  countStatusTone,
  linesToResolve,
  needsStartApproval,
  partialApproveBody,
  showsQuantity,
} from "@/features/warehouse/lib/counts";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import { useStepUp } from "@/features/step-up-engine";
import {
  countMetersError,
  countQuantityError,
} from "@/features/warehouse/lib/forms";
import {
  warehouseExportPaths,
  warehouseKeys,
  warehouseService,
  type StockCount,
  type StockCountLine,
  type StockCountReport,
  type StockCountResolution,
  type StockCountScan,
  type StockCountScanResult,
} from "@/features/warehouse/services/warehouse.service";
import { WarehouseExportMenu } from "@/features/warehouse/components/warehouse-export";
import { useDialogs } from "@/providers/dialog-provider";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/lib/toast";

type CountAction = "approve-start" | "start" | "complete" | "cancel";

/**
 * One stock count (TEC-206): approve the start (initial_placement), start,
 * scan (location QR sets the location context; units, fixed barcodes and
 * SKUs are counted there), complete (differences are computed, nothing
 * changes), then resolve every difference and approve (stock.adjust).
 * Blind counts never show expected values.
 */
export function CountDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const qc = useQueryClient();
  const { confirm } = useDialogs();

  const key = warehouseKeys.count(uuid);
  const countQuery = useQuery({
    queryKey: key,
    queryFn: () => warehouseService.getCount(uuid),
    enabled: access.allowed,
  });
  const count = countQuery.data;
  const reviewed =
    count?.status === "pending_review" || count?.status === "approved";

  const action = useMutation({
    mutationFn: (a: CountAction) => warehouseService.countAction(uuid, a),
    onSuccess: (next, a) => {
      qc.setQueryData(key, next);
      void qc.invalidateQueries({ queryKey: ["warehouse", "counts"] });
      void qc.invalidateQueries({ queryKey: warehouseKeys.countReport(uuid) });
      appToast.success(t(`warehouse.count.${a.replace("-", "_")}_done`));
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });

  const ask = async (a: CountAction) => {
    const k = a.replace("-", "_");
    if (a === "start" || a === "approve-start") {
      action.mutate(a);
      return;
    }
    const ok = await confirm({
      title: t(`warehouse.count.${k}_title`),
      description: t(`warehouse.count.${k}_description`),
      confirmLabel: t(`warehouse.count.${k}`),
      variant: a === "cancel" ? "destructive" : "default",
    });
    if (ok) action.mutate(a);
  };

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.count.title")}
      icon={<ClipboardCheck className="size-6" />}
      crumbs={[
        {
          label: t("warehouse.counts.title"),
          href: routes.tenant.warehouse.counts(slug),
        },
      ]}
      actions={
        count && canWrite ? (
          <div className="flex flex-wrap gap-2">
            {canCancelCount(count) ? (
              <Button
                type="button"
                variant="outline"
                disabled={action.isPending}
                onClick={() => void ask("cancel")}
                data-testid="count-cancel"
              >
                <XCircle className="size-4" />
                {t("warehouse.count.cancel")}
              </Button>
            ) : null}
            {needsStartApproval(count) ? (
              <Button
                type="button"
                variant="outline"
                disabled={action.isPending}
                onClick={() => void ask("approve-start")}
                data-testid="count-approve-start"
              >
                <ShieldCheck className="size-4" />
                {t("warehouse.count.approve_start")}
              </Button>
            ) : null}
            {count.status === "draft" ? (
              <Button
                type="button"
                disabled={action.isPending || !canStart(count)}
                onClick={() => void ask("start")}
                data-testid="count-start"
              >
                <Play className="size-4" />
                {t("warehouse.count.start")}
              </Button>
            ) : null}
            {count.status === "in_progress" ? (
              <Button
                type="button"
                disabled={action.isPending}
                onClick={() => void ask("complete")}
                data-testid="count-complete"
              >
                <CheckCircle2 className="size-4" />
                {t("warehouse.count.complete")}
              </Button>
            ) : null}
          </div>
        ) : null
      }
    >
      {countQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void countQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : !count ? (
        <p className="text-muted-foreground text-sm">
          {t("warehouse.list.loading")}
        </p>
      ) : (
        <>
          <Card>
            <CardContent className="grid gap-4 pt-6 sm:grid-cols-4">
              <Info label={t("warehouse.fields.warehouse")}>
                {count.warehouse.code} · {count.warehouse.name}
              </Info>
              <Info label={t("warehouse.fields.method")}>
                {t(`warehouse.count_method.${count.method}`)}
                <div className="text-muted-foreground text-xs">
                  {t(`warehouse.count_visibility.${count.visibility}`)}
                </div>
              </Info>
              <Info label={t("warehouse.fields.scope")}>
                {t(`warehouse.count_scope.${count.scope_type}`)}
                {count.scope_type !== "warehouse" ? (
                  <div className="text-muted-foreground text-xs" dir="ltr">
                    {countScopeText(count, t)}
                  </div>
                ) : null}
              </Info>
              <Info label={t("warehouse.entries.columns.status")}>
                <span data-testid="count-status" data-status={count.status}>
                  <StatusChip
                    label={t(`warehouse.count_status.${count.status}`)}
                    tone={countStatusTone(count.status)}
                  />
                </span>
              </Info>
              {count.progress ? (
                <div className="sm:col-span-4" data-testid="count-progress">
                  <Info label={t("warehouse.count.progress")}>
                    {/* TEC-1032: guided counts show a bar against the expected totals, blind counts the big scan total. */}
                    {countProgressPercent(count) !== null && count.expected ? (
                      <div className="space-y-1.5">
                        <div className="flex items-center justify-between gap-2 text-sm">
                          <span>
                            {t("warehouse.count.progress_guided", {
                              counted:
                                count.progress.serial_units +
                                count.progress.fixed_quantity,
                              expected:
                                count.expected.serial_units +
                                count.expected.fixed_quantity,
                            })}
                          </span>
                          <span
                            className="font-semibold tabular-nums"
                            data-testid="count-progress-percent"
                          >
                            %{countProgressPercent(count)}
                          </span>
                        </div>
                        <Progress
                          value={countProgressPercent(count) ?? 0}
                          aria-label={t("warehouse.count.progress")}
                        />
                      </div>
                    ) : (
                      <div className="flex items-baseline gap-2">
                        <span
                          className="text-2xl font-semibold tabular-nums"
                          data-testid="count-progress-scans"
                        >
                          {format.number(count.progress.scans)}
                        </span>
                        <span className="text-muted-foreground text-sm">
                          {t("warehouse.count.progress_blind")}
                        </span>
                      </div>
                    )}
                    <p className="text-muted-foreground mt-1 text-xs">
                      {t("warehouse.count.progress_text", {
                        scans: count.progress.scans,
                        units: count.progress.serial_units,
                        quantity:
                          count.progress.fixed_quantity +
                          count.progress.product_quantity,
                      })}
                    </p>
                  </Info>
                </div>
              ) : null}
              {needsStartApproval(count) ? (
                <p className="text-muted-foreground text-sm sm:col-span-4">
                  {t("warehouse.count.approval_needed")}
                </p>
              ) : null}
              {count.note ? (
                <div className="sm:col-span-4">
                  <Info label={t("warehouse.fields.note")}>{count.note}</Info>
                </div>
              ) : null}
            </CardContent>
          </Card>

          {count.expected ? <ExpectedCard count={count} /> : null}

          {count.status === "in_progress" && canWrite ? (
            <CountScanCard count={count} />
          ) : null}

          {count.status === "in_progress" || count.status === "draft" ? (
            <ScansCard count={count} canWrite={canWrite} />
          ) : null}

          {reviewed ? (
            <CountReportCard
              slug={slug}
              count={count}
              canApprove={
                canWrite &&
                access.can(Permission.StockAdjust) &&
                count.status === "pending_review"
              }
              canRecount={canWrite && count.status === "pending_review"}
            />
          ) : null}
        </>
      )}
    </WarehouseShell>
  );
}

/** One KPI tile of the count report (TEC-1032). */
function Kpi({
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

/** Guided counts only: what the ledger expects per location. */
function ExpectedCard({ count }: { count: StockCount }) {
  const { t } = useLocale();
  const exp = count.expected;
  if (!exp) return null;
  return (
    <Card data-testid="count-expected">
      <CardHeader>
        <CardTitle>{t("warehouse.count.expected_title")}</CardTitle>
        <p className="text-muted-foreground text-sm">
          {t("warehouse.count.expected_totals", {
            units: exp.serial_units,
            quantity: exp.fixed_quantity,
          })}
        </p>
      </CardHeader>
      <CardContent>
        <ExpectedLocationsTable locations={exp.locations} />
      </CardContent>
    </Card>
  );
}

/**
 * The scanning station: location QR first (it sets the user's location
 * context), then units / fixed barcodes / SKUs with an optional quantity
 * and roll length.
 */
export function CountScanCard({ count }: { count: StockCount }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [quantity, setQuantity] = useState("");
  const [meters, setMeters] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [last, setLast] = useState<StockCountScanResult | null>(null);
  const qtyError = countQuantityError(quantity, t);
  const metersError = countMetersError(meters, t);
  const withQty = showsQuantity(count.method);

  const scan = useMutation({
    mutationFn: (code: string) =>
      warehouseService.scanCount(count.uuid, {
        code,
        ...(withQty && quantity.trim() ? { quantity: Number(quantity) } : {}),
        ...(meters.trim() ? { meters: meters.trim() } : {}),
      }),
    onSuccess: (res) => {
      setError(null);
      setLast(res);
      setQuantity("");
      setMeters("");
      void qc.invalidateQueries({ queryKey: warehouseKeys.count(count.uuid) });
      void qc.invalidateQueries({
        queryKey: warehouseKeys.countScans(count.uuid),
      });
    },
    onError: (err) =>
      setError(warehouseErrorMessage(err, t, t("warehouse.count.scan_failed"))),
  });

  const context = last?.location_context ?? null;

  return (
    <Card data-testid="count-scan">
      <CardHeader>
        <CardTitle>{t("warehouse.count.scan_title")}</CardTitle>
        <p
          className="text-muted-foreground text-sm"
          data-testid="count-context"
        >
          {context
            ? t("warehouse.count.context", { location: context.full_code })
            : t("warehouse.count.no_context")}
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        <ScanInput
          expect="any"
          warehouseUuid={count.warehouse.uuid}
          id="count-code"
          label={t("warehouse.scan.label")}
          hint={t(`warehouse.count_method_hint.${count.method}`)}
          busy={scan.isPending}
          disabled={Boolean(qtyError || metersError)}
          onScan={(code) => scan.mutate(code)}
        />
        <div className="grid gap-3 sm:grid-cols-2">
          {withQty ? (
            <div className="space-y-1.5">
              <Label htmlFor="count-quantity">
                {t("warehouse.fields.quantity")}
              </Label>
              <Input
                id="count-quantity"
                inputMode="numeric"
                value={quantity}
                placeholder="1"
                aria-invalid={Boolean(qtyError)}
                onChange={(e) => setQuantity(e.target.value)}
                data-testid="count-quantity"
              />
              {qtyError ? (
                <p role="alert" className="text-destructive text-xs">
                  {qtyError}
                </p>
              ) : (
                <p className="text-muted-foreground text-xs">
                  {t("warehouse.count.quantity_hint")}
                </p>
              )}
            </div>
          ) : null}
          <div className="space-y-1.5">
            <Label htmlFor="count-meters">{t("warehouse.fields.meters")}</Label>
            <Input
              id="count-meters"
              inputMode="decimal"
              dir="ltr"
              value={meters}
              aria-invalid={Boolean(metersError)}
              onChange={(e) => setMeters(e.target.value)}
              data-testid="count-meters"
            />
            {metersError ? (
              <p role="alert" className="text-destructive text-xs">
                {metersError}
              </p>
            ) : (
              <p className="text-muted-foreground text-xs">
                {t("warehouse.count.meters_hint")}
              </p>
            )}
          </div>
        </div>
        {error ? (
          <p
            role="alert"
            className="text-destructive text-sm"
            data-testid="count-scan-error"
          >
            {error}
          </p>
        ) : null}
        {last && last.scan.kind !== "location" ? (
          <p className="text-sm" role="status" data-testid="count-last">
            {t("warehouse.count.last_scan", {
              code: last.scan.unit?.barcode ?? last.scan.raw_code,
              quantity: last.scan.quantity,
            })}
            {last.expected && !last.expected.in_scope ? (
              <span className="text-destructive ms-2">
                {t("warehouse.count.not_expected_here")}
              </span>
            ) : null}
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}

function ScansCard({
  count,
  canWrite,
}: {
  count: StockCount;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const scans = useQuery({
    queryKey: warehouseKeys.countScans(count.uuid),
    queryFn: () => warehouseService.listCountScans(count.uuid),
  });
  const remove = useMutation({
    mutationFn: (scanUuid: string) =>
      warehouseService.deleteCountScan(count.uuid, scanUuid),
    onSuccess: () => {
      void qc.invalidateQueries({
        queryKey: warehouseKeys.countScans(count.uuid),
      });
      void qc.invalidateQueries({ queryKey: warehouseKeys.count(count.uuid) });
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const items = useMemo(
    () => (scans.data?.items ?? []).filter((s) => s.kind !== "location"),
    [scans.data?.items],
  );
  const editable = canWrite && count.status === "in_progress";
  const removeScan = remove.mutate;
  const onRemove = useCallback(
    (scan: StockCountScan) => removeScan(scan.uuid),
    [removeScan],
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("warehouse.count.scans_title")}</CardTitle>
      </CardHeader>
      <CardContent>
        {!scans.isLoading && items.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="count-scans-empty"
          >
            {t("warehouse.count.no_scans")}
          </p>
        ) : (
          <CountScansTable
            scans={items}
            isLoading={scans.isLoading}
            editable={editable}
            onRemove={onRemove}
            removing={remove.isPending}
          />
        )}
      </CardContent>
    </Card>
  );
}

/**
 * Completed count: the difference lines with a resolution per line (from
 * its allowed resolutions) and the approval, plus the CSV export.
 */
export function CountReportCard({
  slug,
  count,
  canApprove,
  canRecount = false,
}: {
  slug: string;
  count: StockCount;
  canApprove: boolean;
  /** TEC-1229: warehouse.write enters recounted values. */
  canRecount?: boolean;
}) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const { confirm } = useDialogs();
  const { ensure } = useStepUp();
  const { user } = useAuth();
  const isCreator = Boolean(user && count.created_by_user_uuid === user.uuid);
  const [choices, setChoices] = useState<
    Record<string, StockCountResolution | undefined>
  >({});
  const [error, setError] = useState<string | null>(null);

  const report = useQuery({
    queryKey: warehouseKeys.countReport(count.uuid),
    queryFn: () => warehouseService.getCountReport(count.uuid),
  });
  const lines = useMemo(() => report.data?.lines ?? [], [report.data?.lines]);
  const pending = useMemo(() => linesToResolve(lines), [lines]);
  const onChoices = useCallback(
    (next: Record<string, StockCountResolution>) =>
      setChoices((prev) => ({ ...prev, ...next })),
    [],
  );

  const approve = useMutation({
    mutationFn: (body: ReturnType<typeof approveBody>["resolutions"]) =>
      warehouseService.approveCount(count.uuid, { resolutions: body }),
    onSuccess: (next) => {
      setError(null);
      qc.setQueryData(warehouseKeys.countReport(count.uuid), next);
      qc.setQueryData(warehouseKeys.count(count.uuid), next.count);
      void qc.invalidateQueries({ queryKey: ["warehouse", "counts"] });
      appToast.success(t("warehouse.count.approved"));
    },
    onError: (err) =>
      setError(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });

  const applyReport = (next: StockCountReport) => {
    setError(null);
    qc.setQueryData(warehouseKeys.countReport(count.uuid), next);
    qc.setQueryData(warehouseKeys.count(count.uuid), next.count);
    void qc.invalidateQueries({ queryKey: ["warehouse", "counts"] });
  };
  const fail = (err: unknown) =>
    setError(warehouseErrorMessage(err, t, t("warehouse.form.error")));

  // TEC-1229: recount and partial approval.
  const requestRecount = useMutation({
    mutationFn: (uuids: string[]) =>
      warehouseService.requestCountRecount(count.uuid, uuids),
    onSuccess: (next) => {
      applyReport(next);
      appToast.success(t("warehouse.count.recount_requested_done"));
    },
    onError: fail,
  });
  const partial = useMutation({
    mutationFn: (body: ReturnType<typeof partialApproveBody>) =>
      warehouseService.approveCount(count.uuid, {
        resolutions: body,
        partial: true,
      }),
    onSuccess: (next) => {
      applyReport(next);
      appToast.success(t("warehouse.count.partial_approved"));
    },
    onError: fail,
  });
  const [recountLine, setRecountLine] = useState<StockCountLine | null>(null);
  const onPartialApprove = async (selected: StockCountLine[]) => {
    const body = partialApproveBody(
      lines,
      choices,
      selected.map((l) => l.uuid),
    );
    if (body.length === 0) return;
    const ok = await confirm({
      title: t("warehouse.count.partial_approve"),
      description: t("warehouse.count.approve_description", {
        count: body.length,
      }),
      confirmLabel: t("warehouse.count.approve"),
    });
    if (ok && (await ensure())) partial.mutate(body);
  };

  const onApprove = async () => {
    const { resolutions, missing } = approveBody(lines, choices);
    if (missing.length > 0) {
      setError(t("warehouse.count.resolve_all"));
      return;
    }
    const ok = await confirm({
      title: t("warehouse.count.approve_title"),
      description: t("warehouse.count.approve_description", {
        count: resolutions.length,
      }),
      confirmLabel: t("warehouse.count.approve"),
    });
    if (ok && (await ensure())) approve.mutate(resolutions);
  };

  return (
    <Card data-testid="count-report">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <div className="space-y-2">
          <CardTitle>{t("warehouse.count.report_title")}</CardTitle>
          {count.summary ? (
            <p className="text-muted-foreground text-sm">
              {t("warehouse.count.summary", {
                lines: count.summary.lines,
                unresolved: count.summary.unresolved,
              })}
            </p>
          ) : null}
          {/* TEC-1032: per-result badges (legacy report header). */}
          {countResultBadges(count).length > 0 ? (
            <div
              className="flex flex-wrap gap-1.5"
              data-testid="count-result-badges"
            >
              {countResultBadges(count).map((b) => (
                <StatusChip
                  key={b.result}
                  label={`${t(`warehouse.count_result.${b.result}`)} · ${b.count}`}
                  tone={countResultTone(b.result)}
                />
              ))}
            </div>
          ) : null}
        </div>
        <div className="flex flex-wrap gap-2">
          <WarehouseExportMenu
            slug={slug}
            exportPath={warehouseExportPaths.countReport(count.uuid)}
            label={t("warehouse.export.count_report")}
            testId="count-export"
          />
          {canApprove ? (
            <Button
              type="button"
              size="sm"
              disabled={approve.isPending || report.isLoading || isCreator}
              title={
                isCreator ? t("warehouse.count.self_approval_hint") : undefined
              }
              onClick={() => void onApprove()}
              data-testid="count-approve"
            >
              <CheckCircle2 className="size-4" />
              {t("warehouse.count.approve")}
            </Button>
          ) : null}
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {/* TEC-1032: report KPIs (legacy: Doğruluk / Beklenen-Sayılan / Tarama / Süre). */}
        <dl
          className="grid gap-3 sm:grid-cols-4"
          data-testid="count-report-kpis"
        >
          <Kpi
            label={t("warehouse.count.kpi.accuracy")}
            value={
              countAccuracyPercent(count) === null
                ? "—"
                : `%${countAccuracyPercent(count)}`
            }
            testId="count-kpi-accuracy"
          />
          <Kpi
            label={t("warehouse.count.kpi.expected_counted")}
            value={
              count.expected && count.progress
                ? `${format.number(
                    count.expected.serial_units + count.expected.fixed_quantity,
                  )} / ${format.number(
                    count.progress.serial_units + count.progress.fixed_quantity,
                  )}`
                : count.progress
                  ? format.number(
                      count.progress.serial_units +
                        count.progress.fixed_quantity,
                    )
                  : "—"
            }
            testId="count-kpi-expected"
          />
          <Kpi
            label={t("warehouse.count.kpi.scans")}
            value={count.progress ? format.number(count.progress.scans) : "—"}
            testId="count-kpi-scans"
          />
          <Kpi
            label={t("warehouse.count.kpi.duration")}
            value={
              countDurationMinutes(count) === null
                ? "—"
                : t("warehouse.count.duration_minutes", {
                    minutes: countDurationMinutes(count) ?? 0,
                  })
            }
            testId="count-kpi-duration"
          />
        </dl>
        {canApprove && isCreator ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="count-self-approval-hint"
          >
            {t("warehouse.count.self_approval_hint")}
          </p>
        ) : null}
        {error ? (
          <p
            role="alert"
            className="text-destructive text-sm"
            data-testid="count-approve-error"
          >
            {error}
          </p>
        ) : null}
        {report.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void report.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : pending.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="count-no-differences"
          >
            {report.isLoading
              ? t("warehouse.list.loading")
              : t("warehouse.count.no_differences")}
          </p>
        ) : (
          <CountLinesTable
            lines={pending}
            canApprove={canApprove}
            choices={choices}
            onChoices={onChoices}
            onRequestRecount={
              canApprove && count.status === "pending_review"
                ? (sel) => requestRecount.mutate(sel.map((l) => l.uuid))
                : undefined
            }
            onRecountLine={
              canRecount && count.status === "pending_review"
                ? setRecountLine
                : undefined
            }
            onPartialApprove={
              canApprove && !isCreator && count.status === "pending_review"
                ? (sel) => void onPartialApprove(sel)
                : undefined
            }
          />
        )}
        {recountLine ? (
          <RecountDialog
            count={count}
            line={recountLine}
            onClose={() => setRecountLine(null)}
            onDone={(next) => {
              applyReport(next);
              setRecountLine(null);
              appToast.success(t("warehouse.count.recount_saved"));
            }}
          />
        ) : null}
      </CardContent>
    </Card>
  );
}

/**
 * TEC-1229: the recounted values of one difference line. A serial unit is
 * 0 (not found) or 1 with its location (and meters for a roll); fixed
 * barcodes and products take the counted quantity.
 */
function RecountDialog({
  count,
  line,
  onClose,
  onDone,
}: {
  count: StockCount;
  line: StockCountLine;
  onClose: () => void;
  onDone: (next: StockCountReport) => void;
}) {
  const { t } = useLocale();
  const serial = line.line_kind === "serial";
  const [qty, setQty] = useState(serial ? "1" : String(line.counted_quantity));
  const [meters, setMeters] = useState(line.counted_meters ?? "");
  const [location, setLocation] = useState(
    line.counted_location?.uuid ?? line.expected_location?.uuid ?? "",
  );
  const [error, setError] = useState<string | null>(null);
  const save = useMutation({
    mutationFn: () =>
      warehouseService.recountCountLine(count.uuid, line.uuid, {
        counted_quantity: Number(qty),
        counted_meters: meters.trim() ? meters.trim() : null,
        location_uuid: serial && qty === "1" && location ? location : null,
      }),
    onSuccess: onDone,
    onError: (err) =>
      setError(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const qtyError =
    !/^\d+$/.test(qty) || (serial && qty !== "0" && qty !== "1")
      ? t("warehouse.count.recount_qty_invalid")
      : null;
  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent data-testid="count-recount-dialog">
        <DialogHeader>
          <DialogTitle>
            {t("warehouse.count.recount_title", {
              code: line.unit?.barcode ?? line.product.sku,
            })}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1">
            <Label htmlFor="recount-qty">
              {serial
                ? t("warehouse.count.recount_found")
                : t("warehouse.count.recount_quantity")}
            </Label>
            <Input
              id="recount-qty"
              inputMode="numeric"
              value={qty}
              onChange={(e) => setQty(e.target.value)}
              data-testid="count-recount-qty"
            />
            {qtyError ? (
              <p className="text-destructive text-xs">{qtyError}</p>
            ) : null}
          </div>
          {serial && line.expected_meters !== null ? (
            <div className="space-y-1">
              <Label htmlFor="recount-meters">
                {t("warehouse.count.recount_meters")}
              </Label>
              <Input
                id="recount-meters"
                inputMode="decimal"
                value={meters}
                onChange={(e) => setMeters(e.target.value)}
              />
            </div>
          ) : null}
          {serial && qty === "1" ? (
            <LocationPicker
              warehouseUuid={count.warehouse.uuid}
              value={location}
              onChange={setLocation}
              testId="count-recount-location"
            />
          ) : null}
          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={Boolean(qtyError) || save.isPending}
            onClick={() => save.mutate()}
            data-testid="count-recount-save"
          >
            {t("warehouse.count.recount_save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
