"use client";

import { useQuery } from "@tanstack/react-query";
import { FileDown, Loader2, Sunset } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { Info } from "@/features/warehouse/components/list-controls";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import { eodScopeLabel, netQuantity } from "@/features/warehouse/lib/eod";
import {
  eodPdfClient,
  warehouseKeys,
  warehouseService,
} from "@/features/warehouse/services/warehouse.service";
import {
  fetchCertificate,
  type WaitOptions,
} from "@/features/warranty/lib/certificate";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * One end-of-day report (TEC-207): totals, the movement groups and the
 * product lines, plus the PDF (export job on worker-docs, polled, then
 * downloaded in the user language).
 */
export function EodReportDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const access = useWarehouseAccess(slug);
  const report = useQuery({
    queryKey: warehouseKeys.eodReport(uuid),
    queryFn: () => warehouseService.getEodReport(uuid),
    enabled: access.allowed,
  });
  const r = report.data;

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.eod.report_title")}
      icon={<Sunset className="size-6" />}
      crumbs={[
        {
          label: t("warehouse.eod.title"),
          href: routes.tenant.warehouse.endOfDay(slug),
        },
      ]}
      actions={
        r ? (
          <EodPdfButton
            reportUuid={r.uuid}
            filename={`eod-${r.report_date}.pdf`}
          />
        ) : null
      }
    >
      {report.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void report.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : !r ? (
        <p className="text-muted-foreground text-sm">
          {t("warehouse.list.loading")}
        </p>
      ) : (
        <>
          <Card>
            <CardContent className="grid gap-4 pt-6 sm:grid-cols-4">
              <Info label={t("warehouse.eod.columns.date")}>
                <span data-testid="eod-date-value">
                  {format.date(`${r.report_date}T12:00:00Z`)}
                </span>
                <div className="text-muted-foreground text-xs">
                  {r.timezone}
                </div>
              </Info>
              <Info label={t("warehouse.fields.scope")}>
                {eodScopeLabel(r) ?? t("warehouse.eod.system")}
              </Info>
              <Info label={t("warehouse.eod.columns.kind")}>
                {t(`warehouse.eod.kind.${r.kind}`)}
              </Info>
              <Info label={t("warehouse.eod.columns.generated")}>
                {format.dateTime(r.generated_at)}
              </Info>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("warehouse.eod.totals")}</CardTitle>
            </CardHeader>
            <CardContent
              className="grid gap-4 sm:grid-cols-4"
              data-testid="eod-totals"
            >
              <Info label={t("warehouse.eod.columns.movements")}>
                {format.number(r.summary.totals.movement_count)}
              </Info>
              <Info label={t("warehouse.eod.units")}>
                {format.number(r.summary.totals.unit_count)}
              </Info>
              <Info label={t("warehouse.eod.quantity_in_out")}>
                +{format.number(r.summary.totals.quantity_in)} / −
                {format.number(r.summary.totals.quantity_out)}
              </Info>
              <Info label={t("warehouse.eod.meters_in_out")}>
                <span dir="ltr">
                  +{r.summary.totals.meters_in} / −{r.summary.totals.meters_out}
                </span>
              </Info>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("warehouse.eod.groups")}</CardTitle>
            </CardHeader>
            <CardContent>
              {r.summary.groups.length === 0 ? (
                <p
                  className="text-muted-foreground text-sm"
                  data-testid="eod-no-movements"
                >
                  {t("warehouse.eod.no_movements")}
                </p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm" data-testid="eod-groups">
                    <thead>
                      <tr className="text-muted-foreground border-b text-xs">
                        <th className="p-2 text-start font-medium">
                          {t("warehouse.eod.group")}
                        </th>
                        <th className="p-2 text-end font-medium">
                          {t("warehouse.eod.columns.movements")}
                        </th>
                        <th className="p-2 text-end font-medium">
                          {t("warehouse.eod.net")}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {r.summary.groups.map((g) => (
                        <tr key={g.group} className="border-b last:border-0">
                          <td className="p-2">
                            {t(`warehouse.eod_group.${g.group}`)}
                          </td>
                          <td className="p-2 text-end">
                            {format.number(g.movement_count)}
                          </td>
                          <td className="p-2 text-end">
                            {format.number(netQuantity(g), {
                              signDisplay: "exceptZero",
                            })}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>

          {r.summary.products.length > 0 ? (
            <Card>
              <CardHeader>
                <CardTitle>{t("warehouse.eod.products")}</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="overflow-x-auto">
                  <table className="w-full text-sm" data-testid="eod-products">
                    <thead>
                      <tr className="text-muted-foreground border-b text-xs">
                        <th className="p-2 text-start font-medium">
                          {t("warehouse.fields.product")}
                        </th>
                        <th className="p-2 text-start font-medium">
                          {t("warehouse.eod.group")}
                        </th>
                        <th className="p-2 text-end font-medium">
                          {t("warehouse.eod.columns.movements")}
                        </th>
                        <th className="p-2 text-end font-medium">
                          {t("warehouse.eod.net")}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {r.summary.products.map((p) => (
                        <tr
                          key={`${p.product_uuid}-${p.type}`}
                          className="border-b last:border-0"
                        >
                          <td className="p-2">
                            {p.product_name}
                            <div
                              className="text-muted-foreground font-mono text-xs"
                              dir="ltr"
                            >
                              {p.sku}
                            </div>
                          </td>
                          <td className="p-2">
                            {t(`warehouse.eod_group.${p.group}`)}
                          </td>
                          <td className="p-2 text-end">
                            {format.number(p.movement_count)}
                          </td>
                          <td className="p-2 text-end">
                            {format.number(netQuantity(p), {
                              signDisplay: "exceptZero",
                            })}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </CardContent>
            </Card>
          ) : null}
        </>
      )}
    </WarehouseShell>
  );
}

/** Queues the PDF, waits for the export job and downloads it. */
export function EodPdfButton({
  reportUuid,
  filename,
  waitOptions,
}: {
  reportUuid: string;
  filename: string;
  waitOptions?: WaitOptions;
}) {
  const { t, locale } = useLocale();
  const [busy, setBusy] = useState(false);

  const onClick = async () => {
    setBusy(true);
    try {
      const { file } = await fetchCertificate(
        eodPdfClient(reportUuid),
        locale,
        waitOptions,
      );
      triggerBrowserDownload(file.blob, file.filename ?? filename);
      appToast.success(t("warehouse.eod.pdf_ready"));
    } catch {
      appToast.error(t("warehouse.eod.pdf_failed"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Button
      type="button"
      variant="outline"
      disabled={busy}
      aria-busy={busy}
      onClick={() => void onClick()}
      data-testid="eod-pdf"
    >
      {busy ? (
        <Loader2 className="size-4 animate-spin" />
      ) : (
        <FileDown className="size-4" />
      )}
      {busy ? t("warehouse.eod.pdf_preparing") : t("warehouse.eod.pdf")}
    </Button>
  );
}
