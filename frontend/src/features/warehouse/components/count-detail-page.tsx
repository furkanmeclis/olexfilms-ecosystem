"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircle2,
  ClipboardCheck,
  FileDown,
  Play,
  ShieldCheck,
  XCircle,
} from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
  showsQuantity,
} from "@/features/warehouse/lib/counts";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  countMetersError,
  countQuantityError,
} from "@/features/warehouse/lib/forms";
import {
  countExportPath,
  warehouseKeys,
  warehouseService,
  type StockCount,
  type StockCountResolution,
  type StockCountScan,
  type StockCountScanResult,
} from "@/features/warehouse/services/warehouse.service";
import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

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
  const { t } = useLocale();
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
                    {t("warehouse.count.progress_text", {
                      scans: count.progress.scans,
                      units: count.progress.serial_units,
                      quantity:
                        count.progress.fixed_quantity +
                        count.progress.product_quantity,
                    })}
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
              count={count}
              canApprove={
                canWrite &&
                access.can(Permission.StockAdjust) &&
                count.status === "pending_review"
              }
            />
          ) : null}
        </>
      )}
    </WarehouseShell>
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
  count,
  canApprove,
}: {
  count: StockCount;
  canApprove: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const { confirm } = useDialogs();
  const [choices, setChoices] = useState<
    Record<string, StockCountResolution | undefined>
  >({});
  const [error, setError] = useState<string | null>(null);
  const [downloading, setDownloading] = useState(false);

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
    if (ok) approve.mutate(resolutions);
  };

  const onExport = async () => {
    setDownloading(true);
    try {
      const { blob, filename } = await platformDownloadFile(
        countExportPath(count.uuid),
      );
      triggerBrowserDownload(blob, filename ?? `count-${count.uuid}.csv`);
    } catch {
      appToast.error(t("warehouse.count.export_failed"));
    } finally {
      setDownloading(false);
    }
  };

  return (
    <Card data-testid="count-report">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <div>
          <CardTitle>{t("warehouse.count.report_title")}</CardTitle>
          {count.summary ? (
            <p className="text-muted-foreground text-sm">
              {t("warehouse.count.summary", {
                lines: count.summary.lines,
                unresolved: count.summary.unresolved,
              })}
            </p>
          ) : null}
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={downloading}
            onClick={() => void onExport()}
            data-testid="count-export"
          >
            <FileDown className="size-4" />
            {t("warehouse.count.export")}
          </Button>
          {canApprove ? (
            <Button
              type="button"
              size="sm"
              disabled={approve.isPending || report.isLoading}
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
          />
        )}
      </CardContent>
    </Card>
  );
}
