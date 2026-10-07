"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ChartPie } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { AppChart } from "@/components/charts";
import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { ChartConfig } from "@/components/ui/chart";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { routes } from "@/config/routes";
import { Money } from "@/features/accounting/components/shared";
import { UpcomingStaffPayments } from "@/features/staff-reports/components/planned-payments";
import { ReportExport } from "@/features/staff-reports/components/report-export";
import { useStaffAccess } from "@/features/staff-reports/hooks/use-staff-access";
import {
  AGING_BUCKETS,
  agingChartRows,
  isReportQueryValid,
  marginChartRows,
  pnlChartRows,
  reportFields,
  staffCostChartRows,
} from "@/features/staff-reports/lib/reports";
import {
  REPORT_KINDS,
  staffReportsKeys,
  staffReportsService,
  type CariAgingLine,
  type MarginFigures,
  type MarginProduct,
  type PnlGroup,
  type PnlLine,
  type ReportKind,
  type ReportQuery,
  type StaffCostLine,
} from "@/features/staff-reports/services/staff-reports.service";
import { useLocale } from "@/providers/locale-provider";

export const REPORT_PERSIST_KEYS: Record<ReportKind, string> = {
  pnl: "tenant-accounting-report-pnl-v1",
  margin: "tenant-accounting-report-margin-v1",
  "cari-aging": "tenant-accounting-report-aging-v1",
  "staff-cost": "tenant-accounting-report-staff-cost-v1",
};

const PNL_GROUPS: PnlGroup[] = ["month", "category"];

/** Right-aligned numeric column of the report tables. */
const NUMERIC = {
  headerClassName: "text-end",
  cellClassName: "text-end tabular-nums",
};

/** Report query of a kind: only the fields that report reads. */
function queryOf(kind: ReportKind, q: ReportQuery): ReportQuery {
  const f = reportFields(kind);
  return {
    ...(f.period && q.from ? { from: q.from } : {}),
    ...(f.period && q.to ? { to: q.to } : {}),
    ...(f.asOf && q.as_of ? { as_of: q.as_of } : {}),
    ...(f.group ? { group: q.group ?? "month" } : {}),
  };
}

/**
 * Tenant > Reports (TEC-349 on the TEC-346 endpoints): P&L by month or
 * category, service / product margin, cari aging (0-30 / 31-60 / 61-90 /
 * 90+ days) and staff cost of the own book. Each report has a recharts
 * chart, a client-side DataTable (the API returns the full aggregate) and
 * CSV / XLSX / PDF export through the accounting export jobs.
 */
export function ReportsPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const access = useStaffAccess(slug);
  const [kind, setKind] = useState<ReportKind>("pnl");
  const [filters, setFilters] = useState<ReportQuery>({ group: "month" });
  const fields = reportFields(kind);
  const query = useMemo(() => queryOf(kind, filters), [kind, filters]);
  const valid = isReportQueryValid(query);
  const enabled = valid && access.canReadReports && Boolean(access.orgUuid);

  const title = t("staff_reports.reports.title");
  const header = (
    <PageHeader
      title={title}
      icon={<ChartPie className="size-6" />}
      description={t("staff_reports.reports.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
    />
  );

  if (access.loaded && !access.canReadReports) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("staff_reports.reports.forbidden")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <Card>
        <CardContent className="flex flex-wrap items-end gap-4 pt-6">
          <ToggleGroup
            type="single"
            variant="outline"
            value={kind}
            aria-label={t("staff_reports.reports.tabs_label")}
            onValueChange={(v) => v && setKind(v as ReportKind)}
          >
            {REPORT_KINDS.map((value) => (
              <ToggleGroupItem
                key={value}
                value={value}
                data-testid={`report-tab-${value}`}
              >
                {t(`staff_reports.reports.tabs.${value}`)}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          {fields.period ? (
            <>
              <div className="space-y-1.5">
                <Label htmlFor="report-from">
                  {t("staff_reports.reports.from")}
                </Label>
                <DatePicker
                  id="report-from"
                  className="w-44"
                  value={filters.from ?? ""}
                  onChange={(from) => setFilters((f) => ({ ...f, from }))}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="report-to">
                  {t("staff_reports.reports.to")}
                </Label>
                <DatePicker
                  id="report-to"
                  className="w-44"
                  value={filters.to ?? ""}
                  aria-invalid={valid ? undefined : true}
                  onChange={(to) => setFilters((f) => ({ ...f, to }))}
                />
              </div>
            </>
          ) : null}
          {fields.asOf ? (
            <div className="space-y-1.5">
              <Label htmlFor="report-as-of">
                {t("staff_reports.reports.as_of")}
              </Label>
              <DatePicker
                id="report-as-of"
                className="w-44"
                value={filters.as_of ?? ""}
                onChange={(as_of) => setFilters((f) => ({ ...f, as_of }))}
              />
            </div>
          ) : null}
          {fields.group ? (
            <ToggleGroup
              type="single"
              variant="outline"
              value={filters.group ?? "month"}
              aria-label={t("staff_reports.reports.group_label")}
              onValueChange={(v) =>
                v && setFilters((f) => ({ ...f, group: v as PnlGroup }))
              }
            >
              {PNL_GROUPS.map((value) => (
                <ToggleGroupItem
                  key={value}
                  value={value}
                  data-testid={`report-group-${value}`}
                >
                  {t(`staff_reports.reports.group.${value}`)}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          ) : null}
          <div className="ms-auto">
            <ReportExport
              orgUuid={access.orgUuid}
              kind={kind}
              query={query}
              disabled={!valid}
            />
          </div>
          {valid ? null : (
            <p
              role="alert"
              className="text-destructive w-full text-sm"
              data-testid="report-period-error"
            >
              {t("staff_reports.reports.period_invalid")}
            </p>
          )}
        </CardContent>
      </Card>

      {kind === "pnl" ? (
        <PnlSection org={access.orgUuid} query={query} enabled={enabled} />
      ) : kind === "margin" ? (
        <MarginSection org={access.orgUuid} query={query} enabled={enabled} />
      ) : kind === "cari-aging" ? (
        <AgingSection org={access.orgUuid} query={query} enabled={enabled} />
      ) : (
        <StaffCostSection
          org={access.orgUuid}
          query={query}
          enabled={enabled}
          upcoming={
            access.canManage ? (
              <UpcomingStaffPayments org={access.orgUuid} slug={slug} />
            ) : null
          }
        />
      )}
    </div>
  );
}

type SectionProps = { org: string; query: ReportQuery; enabled: boolean };

function ReportState({
  isLoading,
  isError,
  onRetry,
  children,
}: {
  isLoading: boolean;
  isError: boolean;
  onRetry: () => void;
  children: ReactNode;
}) {
  const { t } = useLocale();
  if (isError) {
    return (
      <ErrorState
        title={t("staff_reports.load_failed")}
        onRetry={onRetry}
        retryLabel={t("common.retry")}
      />
    );
  }
  if (isLoading) {
    return (
      <p className="text-muted-foreground text-sm">{t("accounting.loading")}</p>
    );
  }
  return <>{children}</>;
}

function Stat({
  label,
  amount,
  currency,
  testId,
}: {
  label: string;
  amount: string | number | null;
  currency: string;
  testId?: string;
}) {
  return (
    <div className="rounded-md border p-3" data-testid={testId}>
      <p className="text-muted-foreground text-xs">{label}</p>
      {amount == null ? (
        <p className="text-lg font-semibold">—</p>
      ) : (
        <Money
          amount={amount}
          currency={currency}
          className="text-lg font-semibold"
        />
      )}
    </div>
  );
}

function useMoneyFormatter(currency: string | undefined) {
  const { format } = useLocale();
  return (v: number) => format.currency(v, currency ?? "TRY");
}

function SectionCard({
  kind,
  children,
}: {
  kind: ReportKind;
  children: ReactNode;
}) {
  const { t } = useLocale();
  return (
    <Card data-testid={`report-${kind}`}>
      <CardHeader>
        <CardTitle>{t(`staff_reports.reports.tabs.${kind}`)}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">{children}</CardContent>
    </Card>
  );
}

// --- P&L ---------------------------------------------------------------------

function PnlSection({ org, query, enabled }: SectionProps) {
  const { t, format, locale } = useLocale();
  const report = useQuery({
    queryKey: staffReportsKeys.report(org, "pnl", { ...query, locale }),
    queryFn: () => staffReportsService.pnl({ ...query, locale }),
    enabled,
    placeholderData: keepPreviousData,
  });
  const data = report.data;
  const currency = data?.currency ?? "";
  const money = useMoneyFormatter(data?.currency);
  const rows = useMemo(() => data?.lines ?? [], [data]);
  const chart = useMemo(() => pnlChartRows(rows), [rows]);
  const config = useMemo<ChartConfig>(
    () => ({
      income: {
        label: t("staff_reports.reports.pnl.income"),
        color: "var(--chart-2)",
      },
      expense: {
        label: t("staff_reports.reports.pnl.expense"),
        color: "var(--chart-5)",
      },
    }),
    [t],
  );

  const columns = useMemo(() => {
    const amount = (key: "income" | "expense" | "net", labelKey: string) =>
      createColumn<PnlLine>({
        id: key,
        accessorFn: (row) => Number(row[key]),
        labelKey,
        enableSorting: true,
        filterVariant: "number-range",
        meta: NUMERIC,
        cell: ({ row }) => (
          <Money amount={row.original[key]} currency={currency} />
        ),
      });
    return [
      createColumn<PnlLine>({
        accessorKey: "label",
        labelKey:
          data?.group === "category"
            ? "staff_reports.reports.pnl.category"
            : "staff_reports.reports.pnl.month",
        enableSorting: true,
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) => row.original.label,
      }),
      amount("income", "staff_reports.reports.pnl.income"),
      amount("expense", "staff_reports.reports.pnl.expense"),
      amount("net", "staff_reports.reports.pnl.net"),
      createColumn<PnlLine>({
        accessorKey: "entry_count",
        labelKey: "staff_reports.reports.pnl.entry_count",
        enableSorting: true,
        meta: NUMERIC,
        cell: ({ row }) => format.number(row.original.entry_count),
      }),
    ] as ColumnDef<PnlLine, unknown>[];
  }, [currency, data?.group, format]);

  return (
    <SectionCard kind="pnl">
      <ReportState
        isLoading={report.isLoading}
        isError={report.isError}
        onRetry={() => void report.refetch()}
      >
        {data ? (
          <div className="grid gap-3 sm:grid-cols-3">
            <Stat
              label={t("staff_reports.reports.pnl.income")}
              amount={data.totals.income}
              currency={currency}
            />
            <Stat
              label={t("staff_reports.reports.pnl.expense")}
              amount={data.totals.expense}
              currency={currency}
            />
            <Stat
              label={t("staff_reports.reports.pnl.net")}
              amount={data.totals.net}
              currency={currency}
              testId="pnl-net"
            />
          </div>
        ) : null}
        <AppChart
          type="bar"
          data={chart}
          config={config}
          series={["income", "expense"]}
          valueFormatter={money}
          emptyTitle={t("staff_reports.reports.empty")}
        />
        <EntityTable
          key={data?.group ?? "month"}
          columns={columns}
          data={rows}
          getRowId={(row) => row.key}
          manual={CLIENT_SIDE_MANUAL}
          initialState={{ pagination: { pageIndex: 0, pageSize: 24 } }}
          emptyTitle={t("staff_reports.reports.empty")}
          emptyDescription=""
          features={{
            persistKey: REPORT_PERSIST_KEYS.pnl,
            rowSelection: false,
          }}
        />
      </ReportState>
    </SectionCard>
  );
}

// --- Margin ------------------------------------------------------------------

/** Margin percent (the API sends "40.00" for 40 %). */
function formatPct(
  format: { percent: (v: number, digits?: number) => string },
  v: string | null,
) {
  return v == null ? "—" : format.percent(Number(v) / 100, 2);
}

function MarginSection({ org, query, enabled }: SectionProps) {
  const { t, format } = useLocale();
  const report = useQuery({
    queryKey: staffReportsKeys.report(org, "margin", query),
    queryFn: () => staffReportsService.margin(query),
    enabled,
    placeholderData: keepPreviousData,
  });
  const data = report.data;
  const currency = data?.currency ?? "";
  const costVisible = data?.cost_visible ?? false;
  const money = useMoneyFormatter(data?.currency);
  const rows = useMemo(() => data?.products ?? [], [data]);
  const chart = useMemo(() => marginChartRows(rows), [rows]);
  const config = useMemo<ChartConfig>(
    () => ({
      revenue: {
        label: t("staff_reports.reports.margin.revenue"),
        color: "var(--chart-1)",
      },
      ...(costVisible
        ? {
            gross_profit: {
              label: t("staff_reports.reports.margin.gross_profit"),
              color: "var(--chart-2)",
            },
          }
        : {}),
    }),
    [costVisible, t],
  );
  const pct = (v: string | null) => formatPct(format, v);

  const columns = useMemo(() => {
    const amount = (
      key: "revenue" | "cost" | "gross_profit",
      labelKey: string,
    ) =>
      createColumn<MarginProduct>({
        id: key,
        accessorFn: (row) => (row[key] == null ? null : Number(row[key])),
        labelKey,
        enableSorting: true,
        sortUndefined: "last",
        meta: NUMERIC,
        cell: ({ row }) =>
          row.original[key] == null ? (
            "—"
          ) : (
            <Money amount={row.original[key]!} currency={currency} />
          ),
      });
    return [
      createColumn<MarginProduct>({
        accessorKey: "name",
        labelKey: "staff_reports.reports.margin.product",
        enableSorting: true,
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <div>
            <div>{row.original.name}</div>
            <div className="text-muted-foreground font-mono text-xs" dir="ltr">
              {row.original.sku}
            </div>
          </div>
        ),
      }),
      createColumn<MarginProduct>({
        id: "quantity",
        accessorFn: (row) => Number(row.quantity),
        labelKey: "staff_reports.reports.margin.quantity",
        enableSorting: true,
        meta: NUMERIC,
        cell: ({ row }) => format.number(Number(row.original.quantity)),
      }),
      createColumn<MarginProduct>({
        accessorKey: "sale_count",
        labelKey: "staff_reports.reports.margin.sale_count",
        enableSorting: true,
        meta: NUMERIC,
        cell: ({ row }) => format.number(row.original.sale_count),
      }),
      amount("revenue", "staff_reports.reports.margin.revenue"),
      ...(costVisible
        ? [
            amount("cost", "staff_reports.reports.margin.cost"),
            amount("gross_profit", "staff_reports.reports.margin.gross_profit"),
            createColumn<MarginProduct>({
              id: "margin_pct",
              accessorFn: (row) =>
                row.margin_pct == null ? null : Number(row.margin_pct),
              labelKey: "staff_reports.reports.margin.margin_pct",
              enableSorting: true,
              sortUndefined: "last",
              gridSecondary: true,
              filterVariant: "number-range",
              meta: NUMERIC,
              cell: ({ row }) => (
                <span className="inline-flex items-center gap-2">
                  {formatPct(format, row.original.margin_pct)}
                  {row.original.cost_incomplete ? (
                    <StatusChip
                      label={t("staff_reports.reports.margin.cost_incomplete")}
                      tone="warning"
                    />
                  ) : null}
                </span>
              ),
            }),
          ]
        : []),
    ] as ColumnDef<MarginProduct, unknown>[];
  }, [costVisible, currency, format, t]);

  const figures = (label: string, f: MarginFigures, testId: string) => (
    <div className="space-y-1 rounded-md border p-3" data-testid={testId}>
      <p className="text-sm font-medium">{label}</p>
      <p className="flex justify-between gap-2 text-sm">
        <span className="text-muted-foreground">
          {t("staff_reports.reports.margin.revenue")}
        </span>
        <Money amount={f.revenue} currency={currency} />
      </p>
      {costVisible ? (
        <>
          <p className="flex justify-between gap-2 text-sm">
            <span className="text-muted-foreground">
              {t("staff_reports.reports.margin.gross_profit")}
            </span>
            {f.gross_profit == null ? (
              "—"
            ) : (
              <Money amount={f.gross_profit} currency={currency} />
            )}
          </p>
          <p className="flex justify-between gap-2 text-sm">
            <span className="text-muted-foreground">
              {t("staff_reports.reports.margin.margin_pct")}
            </span>
            <span className="tabular-nums">{pct(f.margin_pct)}</span>
          </p>
        </>
      ) : null}
    </div>
  );

  return (
    <SectionCard kind="margin">
      <ReportState
        isLoading={report.isLoading}
        isError={report.isError}
        onRetry={() => void report.refetch()}
      >
        {data ? (
          <>
            <div className="grid gap-3 sm:grid-cols-3">
              {figures(
                t("staff_reports.reports.margin.services", {
                  n: format.number(data.services.service_count),
                }),
                data.services,
                "margin-services",
              )}
              {figures(
                t("staff_reports.reports.margin.product_sales"),
                data.product_sales,
                "margin-products",
              )}
              {figures(
                t("staff_reports.reports.margin.total"),
                data.total,
                "margin-total",
              )}
            </div>
            {costVisible ? null : (
              <p className="text-muted-foreground text-xs">
                {t("staff_reports.reports.margin.cost_hidden")}
              </p>
            )}
          </>
        ) : null}
        <AppChart
          type="bar-horizontal"
          data={chart}
          config={config}
          valueFormatter={money}
          height={Math.max(160, chart.length * 36 + 48)}
          emptyTitle={t("staff_reports.reports.empty")}
        />
        <EntityTable
          key={costVisible ? "cost" : "no-cost"}
          columns={columns}
          data={rows}
          getRowId={(row) => row.product_uuid}
          manual={CLIENT_SIDE_MANUAL}
          initialState={{ sorting: [{ id: "revenue", desc: true }] }}
          emptyTitle={t("staff_reports.reports.empty")}
          emptyDescription=""
          features={{
            persistKey: REPORT_PERSIST_KEYS.margin,
            rowSelection: false,
          }}
        />
      </ReportState>
    </SectionCard>
  );
}

// --- Cari aging --------------------------------------------------------------

const SIDES = ["receivable", "payable"] as const;
const COUNTERPARTY_TYPES = ["organization", "user"] as const;

/** Cari aging table columns; the bucket headers come from AGING_BUCKETS. */
export function agingColumns(
  currency: string,
  t: (key: string) => string,
): ColumnDef<CariAgingLine, unknown>[] {
  return [
    createColumn<CariAgingLine>({
      id: "counterparty",
      accessorFn: (row) => row.counterparty.name,
      labelKey: "staff_reports.reports.aging.counterparty",
      enableSorting: true,
      enableHiding: false,
      gridPrimary: true,
      cell: ({ row }) => row.original.counterparty.name,
    }),
    createColumn<CariAgingLine>({
      id: "counterparty_type",
      accessorFn: (row) => row.counterparty.type,
      labelKey: "staff_reports.reports.aging.counterparty_type",
      enableSorting: true,
      defaultHidden: true,
      filterVariant: "faceted",
      filterOptions: COUNTERPARTY_TYPES.map((value) => ({
        value,
        label: value,
        labelKey: `staff_reports.reports.aging.counterparty_types.${value}`,
      })),
      cell: ({ row }) =>
        t(
          `staff_reports.reports.aging.counterparty_types.${row.original.counterparty.type}`,
        ),
    }),
    createColumn<CariAgingLine>({
      accessorKey: "side",
      labelKey: "staff_reports.reports.aging.side",
      enableSorting: true,
      gridSecondary: true,
      filterVariant: "faceted",
      filterOptions: SIDES.map((value) => ({
        value,
        label: value,
        labelKey: `staff_reports.reports.aging.sides.${value}`,
      })),
      cell: ({ row }) => (
        <StatusChip
          label={t(`staff_reports.reports.aging.sides.${row.original.side}`)}
          tone={row.original.side === "receivable" ? "success" : "warning"}
        />
      ),
    }),
    createColumn<CariAgingLine>({
      id: "balance",
      accessorFn: (row) => Number(row.balance),
      labelKey: "staff_reports.reports.aging.balance",
      enableSorting: true,
      filterVariant: "number-range",
      meta: NUMERIC,
      cell: ({ row }) => (
        <Money amount={row.original.balance} currency={currency} />
      ),
    }),
    ...AGING_BUCKETS.map(({ key, labelKey }) =>
      createColumn<CariAgingLine>({
        id: key,
        accessorFn: (row) => Number(row.buckets[key]),
        labelKey,
        enableSorting: true,
        meta: NUMERIC,
        cell: ({ row }) => (
          <Money amount={row.original.buckets[key]} currency={currency} />
        ),
      }),
    ),
  ] as ColumnDef<CariAgingLine, unknown>[];
}

function AgingSection({ org, query, enabled }: SectionProps) {
  const { t } = useLocale();
  const report = useQuery({
    queryKey: staffReportsKeys.report(org, "cari-aging", query),
    queryFn: () => staffReportsService.cariAging(query),
    enabled,
    placeholderData: keepPreviousData,
  });
  const data = report.data;
  const currency = data?.currency ?? "";
  const money = useMoneyFormatter(data?.currency);
  const rows = useMemo(() => data?.lines ?? [], [data]);
  const chart = useMemo(
    () =>
      data
        ? agingChartRows(data, (key) =>
            t(AGING_BUCKETS.find((b) => b.key === key)!.labelKey),
          )
        : [],
    [data, t],
  );
  const config = useMemo<ChartConfig>(
    () => ({
      receivable: {
        label: t("staff_reports.reports.aging.sides.receivable"),
        color: "var(--chart-2)",
      },
      payable: {
        label: t("staff_reports.reports.aging.sides.payable"),
        color: "var(--chart-4)",
      },
    }),
    [t],
  );
  const columns = useMemo(() => agingColumns(currency, t), [currency, t]);

  return (
    <SectionCard kind="cari-aging">
      <ReportState
        isLoading={report.isLoading}
        isError={report.isError}
        onRetry={() => void report.refetch()}
      >
        {data ? (
          <div className="grid gap-3 sm:grid-cols-2">
            <Stat
              label={t("staff_reports.reports.aging.sides.receivable")}
              amount={data.totals.receivable.balance}
              currency={currency}
            />
            <Stat
              label={t("staff_reports.reports.aging.sides.payable")}
              amount={data.totals.payable.balance}
              currency={currency}
            />
          </div>
        ) : null}
        <AppChart
          type="bar"
          data={chart}
          config={config}
          series={["receivable", "payable"]}
          valueFormatter={money}
          emptyTitle={t("staff_reports.reports.empty")}
        />
        <div data-testid="aging-table">
          <EntityTable
            columns={columns}
            data={rows}
            getRowId={(row) => row.cari_uuid}
            manual={CLIENT_SIDE_MANUAL}
            initialState={{ sorting: [{ id: "balance", desc: true }] }}
            emptyTitle={t("staff_reports.reports.empty")}
            emptyDescription=""
            features={{
              persistKey: REPORT_PERSIST_KEYS["cari-aging"],
              rowSelection: false,
            }}
          />
        </div>
      </ReportState>
    </SectionCard>
  );
}

// --- Staff cost --------------------------------------------------------------

/**
 * Staff cost counts booked payments on their payment day, like the P&L
 * (TEC-381); planned ones show in the "upcoming" summary instead.
 */
function StaffCostSection({
  org,
  query,
  enabled,
  upcoming,
}: SectionProps & { upcoming?: ReactNode }) {
  const { t, format } = useLocale();
  const report = useQuery({
    queryKey: staffReportsKeys.report(org, "staff-cost", query),
    queryFn: () => staffReportsService.staffCost(query),
    enabled,
    placeholderData: keepPreviousData,
  });
  const data = report.data;
  const currency = data?.currency ?? "";
  const money = useMoneyFormatter(data?.currency);
  const rows = useMemo(() => data?.lines ?? [], [data]);
  const chart = useMemo(() => staffCostChartRows(rows), [rows]);
  const config = useMemo<ChartConfig>(
    () => ({
      salary: {
        label: t("staff_reports.payment_types.salary"),
        color: "var(--chart-1)",
      },
      advance: {
        label: t("staff_reports.payment_types.advance"),
        color: "var(--chart-3)",
      },
      bonus: {
        label: t("staff_reports.payment_types.bonus"),
        color: "var(--chart-2)",
      },
    }),
    [t],
  );

  const columns = useMemo(() => {
    const amount = (
      key: "salary" | "advance" | "bonus" | "total",
      labelKey: string,
    ) =>
      createColumn<StaffCostLine>({
        id: key,
        accessorFn: (row) => Number(row[key]),
        labelKey,
        enableSorting: true,
        filterVariant: key === "total" ? "number-range" : undefined,
        meta: NUMERIC,
        cell: ({ row }) => (
          <Money amount={row.original[key]} currency={currency} />
        ),
      });
    return [
      createColumn<StaffCostLine>({
        accessorKey: "name",
        labelKey: "staff_reports.fields.name",
        enableSorting: true,
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) => row.original.name,
      }),
      createColumn<StaffCostLine>({
        id: "title",
        accessorFn: (row) => row.title ?? "",
        labelKey: "staff_reports.fields.title",
        enableSorting: true,
        cell: ({ row }) => row.original.title || "—",
      }),
      createColumn<StaffCostLine>({
        accessorKey: "active",
        labelKey: "staff_reports.fields.status",
        enableSorting: true,
        defaultHidden: true,
        filterVariant: "boolean",
        filterFn: (row, id, value) =>
          value === undefined || row.getValue(id) === value,
        cell: ({ row }) =>
          row.original.active
            ? t("accounting.status.active")
            : t("accounting.status.inactive"),
      }),
      amount("salary", "staff_reports.payment_types.salary"),
      amount("advance", "staff_reports.payment_types.advance"),
      amount("bonus", "staff_reports.payment_types.bonus"),
      amount("total", "staff_reports.reports.staff_cost.total"),
      createColumn<StaffCostLine>({
        accessorKey: "payment_count",
        labelKey: "staff_reports.reports.staff_cost.payment_count",
        enableSorting: true,
        meta: NUMERIC,
        cell: ({ row }) => format.number(row.original.payment_count),
      }),
    ] as ColumnDef<StaffCostLine, unknown>[];
  }, [currency, format, t]);

  return (
    <SectionCard kind="staff-cost">
      <ReportState
        isLoading={report.isLoading}
        isError={report.isError}
        onRetry={() => void report.refetch()}
      >
        {data ? (
          <div className="grid gap-3 sm:grid-cols-4">
            <Stat
              label={t("staff_reports.payment_types.salary")}
              amount={data.totals.salary}
              currency={currency}
            />
            <Stat
              label={t("staff_reports.payment_types.advance")}
              amount={data.totals.advance}
              currency={currency}
            />
            <Stat
              label={t("staff_reports.payment_types.bonus")}
              amount={data.totals.bonus}
              currency={currency}
            />
            <Stat
              label={t("staff_reports.reports.staff_cost.total")}
              amount={data.totals.total}
              currency={currency}
            />
          </div>
        ) : null}
        {upcoming}
        <AppChart
          type="bar"
          data={chart}
          config={config}
          series={["salary", "advance", "bonus"]}
          stacked
          valueFormatter={money}
          emptyTitle={t("staff_reports.reports.empty")}
        />
        <EntityTable
          columns={columns}
          data={rows}
          getRowId={(row) => row.staff_uuid}
          manual={CLIENT_SIDE_MANUAL}
          initialState={{ sorting: [{ id: "total", desc: true }] }}
          emptyTitle={t("staff_reports.reports.empty")}
          emptyDescription=""
          features={{
            persistKey: REPORT_PERSIST_KEYS["staff-cost"],
            rowSelection: false,
          }}
        />
      </ReportState>
    </SectionCard>
  );
}
