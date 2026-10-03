"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { RefreshCw, Sunset } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState, type FormEvent } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ListBody, Pager } from "@/features/warehouse/components/list-controls";
import { nativeSelectClass } from "@/features/warehouse/components/native-select-field";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  eodScopeLabel,
  isFutureDay,
  todayIn,
} from "@/features/warehouse/lib/eod";
import {
  pageCount,
  warehouseErrorMessage,
} from "@/features/warehouse/lib/errors";
import {
  warehouseKeys,
  warehouseService,
  type EodListQuery,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const EOD_PAGE_SIZE = 20;
const SYSTEM = "system";

/**
 * Warehouse > End of day (TEC-207): stored daily reports (the hourly cron
 * writes yesterday's, kind auto) and a manual run for a day and scope
 * (system report or one warehouse, kind manual, replacing the stored one).
 */
export function EodReportsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const [scope, setScope] = useState<"" | "system" | "warehouse">("");
  const [page, setPage] = useState(0);

  const query = useMemo<EodListQuery>(
    () => ({
      ...(scope ? { scope } : {}),
      limit: EOD_PAGE_SIZE,
      offset: page * EOD_PAGE_SIZE,
    }),
    [scope, page],
  );
  const list = useQuery({
    queryKey: warehouseKeys.eodReports(query),
    queryFn: () => warehouseService.listEodReports(query),
    enabled: access.allowed,
    placeholderData: keepPreviousData,
  });
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const pages = pageCount(total, EOD_PAGE_SIZE);

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.eod.title")}
      description={t("warehouse.eod.description")}
      icon={<Sunset className="size-6" />}
    >
      {canWrite ? <GenerateEodForm slug={slug} /> : null}

      <Card>
        <CardContent className="space-y-4 pt-6">
          <div
            className="flex flex-wrap gap-2"
            role="group"
            aria-label={t("warehouse.fields.scope")}
          >
            {(["", "system", "warehouse"] as const).map((s) => (
              <Button
                key={s || "all"}
                type="button"
                size="sm"
                variant={scope === s ? "secondary" : "ghost"}
                aria-pressed={scope === s}
                onClick={() => {
                  setScope(s);
                  setPage(0);
                }}
              >
                {t(`warehouse.eod.scope_filter.${s || "all"}`)}
              </Button>
            ))}
          </div>
          <ListBody
            isError={list.isError}
            isLoading={list.isLoading}
            isEmpty={rows.length === 0}
            onRetry={() => void list.refetch()}
            emptyTitle={t("warehouse.eod.empty_title")}
            emptyDescription={t("warehouse.eod.empty_description")}
            emptyTestId="eod-empty"
          >
            <div className="overflow-x-auto">
              <table className="w-full text-sm" data-testid="eod-table">
                <thead>
                  <tr className="text-muted-foreground border-b text-xs">
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.eod.columns.date")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.fields.scope")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.eod.columns.movements")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.eod.columns.kind")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.eod.columns.generated")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <tr
                      key={r.uuid}
                      className="hover:bg-accent/50 border-b last:border-0"
                      data-testid="eod-row"
                    >
                      <td className="p-2">
                        <Link
                          href={routes.tenant.warehouse.endOfDayReport(
                            slug,
                            r.uuid,
                          )}
                          className="font-medium hover:underline"
                        >
                          {format.date(`${r.report_date}T12:00:00Z`)}
                        </Link>
                      </td>
                      <td className="p-2">
                        {eodScopeLabel(r) ?? t("warehouse.eod.system")}
                      </td>
                      <td className="p-2">
                        {format.number(r.summary.totals.movement_count)}
                      </td>
                      <td className="p-2">
                        <StatusChip
                          label={t(`warehouse.eod.kind.${r.kind}`)}
                          tone={r.kind === "manual" ? "warning" : "default"}
                        />
                      </td>
                      <td className="text-muted-foreground p-2 text-xs whitespace-nowrap">
                        {format.dateTime(r.generated_at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </ListBody>
          <Pager
            page={page}
            pages={pages}
            total={total}
            busy={list.isFetching}
            hidden={rows.length === 0 && page === 0}
            onPage={setPage}
          />
        </CardContent>
      </Card>
    </WarehouseShell>
  );
}

/** Manual run: a day (not in the future) and the system or one warehouse. */
export function GenerateEodForm({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const today = todayIn(format.timeZone);
  const [date, setDate] = useState(today);
  const [warehouseUuid, setWarehouseUuid] = useState(SYSTEM);
  const [error, setError] = useState<string | null>(null);

  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
  });

  const generate = useMutation({
    mutationFn: () =>
      warehouseService.generateEodReport({
        date,
        warehouse_uuid: warehouseUuid === SYSTEM ? null : warehouseUuid,
      }),
    onSuccess: async (report) => {
      await qc.invalidateQueries({ queryKey: ["warehouse", "eod-reports"] });
      appToast.success(t("warehouse.eod.generated"));
      router.push(routes.tenant.warehouse.endOfDayReport(slug, report.uuid));
    },
    onError: (err) =>
      setError(warehouseErrorMessage(err, t, t("warehouse.eod.failed"))),
  });

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) {
      setError(t("warehouse.validation.date"));
      return;
    }
    if (isFutureDay(date, today)) {
      setError(t("warehouse.validation.future_day"));
      return;
    }
    setError(null);
    generate.mutate();
  };

  return (
    <Card data-testid="eod-form">
      <CardHeader>
        <CardTitle>{t("warehouse.eod.generate_title")}</CardTitle>
        <p className="text-muted-foreground text-sm">
          {t("warehouse.eod.generate_hint")}
        </p>
      </CardHeader>
      <CardContent>
        <form
          className="grid items-end gap-4 sm:grid-cols-3"
          onSubmit={onSubmit}
          noValidate
        >
          <div className="space-y-1.5">
            <Label htmlFor="eod-date">{t("warehouse.eod.columns.date")}</Label>
            <div data-testid="eod-date">
              <DatePicker id="eod-date" value={date} onChange={setDate} />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="eod-warehouse">{t("warehouse.fields.scope")}</Label>
            <select
              id="eod-warehouse"
              className={nativeSelectClass}
              value={warehouseUuid}
              onChange={(e) => setWarehouseUuid(e.target.value)}
              data-testid="eod-warehouse"
            >
              <option value={SYSTEM}>{t("warehouse.eod.system")}</option>
              {(warehouses.data?.items ?? []).map((w) => (
                <option key={w.uuid} value={w.uuid}>
                  {w.code} · {w.name}
                </option>
              ))}
            </select>
          </div>
          <Button
            type="submit"
            disabled={generate.isPending}
            data-testid="eod-generate"
          >
            <RefreshCw className="size-4" />
            {t("warehouse.eod.generate")}
          </Button>
          {error ? (
            <p
              role="alert"
              className="text-destructive text-sm sm:col-span-3"
              data-testid="eod-error"
            >
              {error}
            </p>
          ) : null}
        </form>
      </CardContent>
    </Card>
  );
}
