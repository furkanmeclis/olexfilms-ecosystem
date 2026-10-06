"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ChartColumn } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { PageHeader } from "@/components/layout/page-header";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ClaimReportExportButton } from "@/features/warranty-claims/components/claim-report-export-button";
import { FailureRateChart } from "@/features/warranty-claims/components/failure-rate-chart";
import {
  buildPeriodQuery,
  CLAIM_REPORT_TABS,
  dealerFilterOptions,
  failureRowKey,
  formatClaimRate,
  isPeriodValid,
  tabGroup,
  type ClaimReportTab,
} from "@/features/warranty-claims/lib/claim-reports";
import {
  claimReportKeys,
  claimReportsService,
  type ByDealerRow,
  type ClaimReportPeriod,
  type FailureRateRow,
  type PartsRow,
} from "@/features/warranty-claims/services/claim-reports.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * Tenant > Warranty claim report (TEC-340 on the TEC-338 endpoints): period
 * filter, product / lot failure rates (table + bar chart), claims by dealer
 * (with a dealer filter limited to the caller's scope) and part
 * distribution; each tab exports to CSV / XLSX through the export center.
 * TEC-378: each report is a client-side DataTable (the API returns the
 * full aggregate) with sortable counts and rates; the dealer filter is the
 * organization facet of the by-dealer table.
 */
export function WarrantyClaimReportsPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.warrantyClaims.read);
  const [tab, setTab] = useState<ClaimReportTab>("product");
  const [period, setPeriod] = useState<ClaimReportPeriod>({});
  const periodValid = isPeriodValid(period);
  const query = useMemo(() => buildPeriodQuery(period), [period]);

  const title = t("warranty.claim_reports.title");
  const header = (
    <PageHeader
      title={title}
      icon={<ChartColumn className="size-6" />}
      description={t("warranty.claim_reports.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("warranty.claim_reports.forbidden")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <Card>
        <CardContent className="flex flex-wrap items-end gap-4 pt-6">
          <div className="space-y-1.5">
            <Label htmlFor="claim-report-from">
              {t("warranty.claim_reports.from")}
            </Label>
            <DatePicker
              id="claim-report-from"
              className="w-44"
              value={period.from ?? ""}
              onChange={(from) => setPeriod((p) => ({ ...p, from }))}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="claim-report-to">
              {t("warranty.claim_reports.to")}
            </Label>
            <DatePicker
              id="claim-report-to"
              className="w-44"
              value={period.to ?? ""}
              aria-invalid={periodValid ? undefined : true}
              onChange={(to) => setPeriod((p) => ({ ...p, to }))}
            />
          </div>
          <ToggleGroup
            type="single"
            variant="outline"
            value={tab}
            aria-label={t("warranty.claim_reports.tabs_label")}
            onValueChange={(v) => v && setTab(v as ClaimReportTab)}
          >
            {CLAIM_REPORT_TABS.map((value) => (
              <ToggleGroupItem
                key={value}
                value={value}
                data-testid={`claim-report-tab-${value}`}
              >
                {t(`warranty.claim_reports.tabs.${value}`)}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          <div className="ms-auto">
            <ClaimReportExportButton
              slug={slug}
              tab={tab}
              period={query}
              disabled={!periodValid}
            />
          </div>
          {periodValid ? null : (
            <p
              role="alert"
              className="text-destructive w-full text-sm"
              data-testid="claim-report-period-error"
            >
              {t("warranty.claim_reports.period_invalid")}
            </p>
          )}
        </CardContent>
      </Card>

      {tab === "product" || tab === "lot" ? (
        <FailureRateSection tab={tab} query={query} enabled={periodValid} />
      ) : tab === "dealer" ? (
        <ByDealerSection query={query} enabled={periodValid} />
      ) : (
        <PartsSection query={query} enabled={periodValid} />
      )}
    </div>
  );
}

/** Right-aligned numeric column of the report tables. */
const NUMERIC = {
  headerClassName: "text-end",
  cellClassName: "text-end tabular-nums",
};

export const CLAIM_REPORT_PERSIST_KEYS = {
  product: "tenant-claim-report-product-v1",
  lot: "tenant-claim-report-lot-v1",
  dealer: "tenant-claim-report-dealer-v1",
  parts: "tenant-claim-report-parts-v1",
} as const;

const ORG_TYPES = ["center", "distributor", "dealer"] as const;

function ReportState({
  isLoading,
  isError,
  empty,
  onRetry,
  children,
}: {
  isLoading: boolean;
  isError: boolean;
  empty: boolean;
  onRetry: () => void;
  children: ReactNode;
}) {
  const { t } = useLocale();
  if (isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={onRetry}
        retryLabel={t("common.retry")}
      />
    );
  }
  if (isLoading) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("warranty.claim_reports.loading")}
      </p>
    );
  }
  if (empty) {
    return (
      <p
        className="text-muted-foreground py-8 text-center text-sm"
        data-testid="claim-report-empty"
      >
        {t("warranty.claim_reports.empty")}
      </p>
    );
  }
  return <>{children}</>;
}

function FailureRateSection({
  tab,
  query,
  enabled,
}: {
  tab: "product" | "lot";
  query: ClaimReportPeriod;
  enabled: boolean;
}) {
  const { t, format, locale } = useLocale();
  const group = tabGroup(tab) ?? "product";
  const params = useMemo(() => ({ ...query, group }), [query, group]);
  const report = useQuery({
    queryKey: claimReportKeys.failureRate(params),
    queryFn: () => claimReportsService.failureRate(params),
    enabled,
    placeholderData: keepPreviousData,
  });
  const rows = useMemo(
    () => (report.data?.group === group ? report.data.items : []),
    [report.data, group],
  );

  const columns = useMemo(() => {
    const rate = (v: number) => formatClaimRate(v, { locale });
    const count = (
      key: "warranty_count" | "claim_count" | "approved_claim_count",
      labelKey: string,
    ) =>
      createColumn<FailureRateRow>({
        accessorKey: key,
        labelKey,
        enableSorting: true,
        meta: NUMERIC,
        cell: ({ row }) => format.number(row.original[key]),
      });
    return [
      createColumn<FailureRateRow>({
        id: "product",
        accessorFn: (row) => row.product_name,
        labelKey: "warranty.claim_reports.columns.product",
        enableSorting: true,
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <div data-testid="claim-report-failure-row">
            <div>{row.original.product_name}</div>
            <div className="text-muted-foreground font-mono text-xs" dir="ltr">
              {row.original.product_sku}
            </div>
          </div>
        ),
      }),
      ...(tab === "lot"
        ? [
            createColumn<FailureRateRow>({
              id: "lot",
              accessorFn: (row) => row.lot_code ?? "",
              labelKey: "warranty.claim_reports.columns.lot",
              enableSorting: true,
              cell: ({ row }) => (
                <span className="font-mono text-xs" dir="ltr">
                  {row.original.lot_code ?? "—"}
                </span>
              ),
            }),
          ]
        : []),
      count("warranty_count", "warranty.claim_reports.columns.warranties"),
      count("claim_count", "warranty.claim_reports.columns.claims"),
      count("approved_claim_count", "warranty.claim_reports.columns.approved"),
      createColumn<FailureRateRow>({
        accessorKey: "claim_rate",
        labelKey: "warranty.claim_reports.columns.claim_rate",
        enableSorting: true,
        meta: NUMERIC,
        cell: ({ row }) => (
          <span data-testid="claim-rate">{rate(row.original.claim_rate)}</span>
        ),
      }),
      createColumn<FailureRateRow>({
        accessorKey: "approved_rate",
        labelKey: "warranty.claim_reports.columns.approved_rate",
        enableSorting: true,
        gridSecondary: true,
        meta: NUMERIC,
        cell: ({ row }) => (
          <span data-testid="approved-rate">
            {rate(row.original.approved_rate)}
          </span>
        ),
      }),
    ] as ColumnDef<FailureRateRow, unknown>[];
  }, [format, locale, tab]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t(`warranty.claim_reports.sections.${tab}`)}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <ReportState
          isLoading={report.isLoading}
          isError={report.isError}
          empty={rows.length === 0}
          onRetry={() => void report.refetch()}
        >
          <FailureRateChart rows={rows} />
          <div data-testid="claim-report-failure-table">
            <EntityTable
              key={tab}
              columns={columns}
              data={rows}
              getRowId={failureRowKey}
              manual={CLIENT_SIDE_MANUAL}
              initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
              emptyTitle={t("warranty.claim_reports.empty")}
              emptyDescription=""
              features={{ persistKey: CLAIM_REPORT_PERSIST_KEYS[tab] }}
            />
          </div>
        </ReportState>
      </CardContent>
    </Card>
  );
}

function ByDealerSection({
  query,
  enabled,
}: {
  query: ClaimReportPeriod;
  enabled: boolean;
}) {
  const { t, format, locale } = useLocale();
  const report = useQuery({
    queryKey: claimReportKeys.byDealer(query),
    queryFn: () => claimReportsService.byDealer(query),
    enabled,
    placeholderData: keepPreviousData,
  });
  const rows = useMemo(() => report.data?.items ?? [], [report.data]);
  // Dealer options come only from the scoped rows (a distributor sees its
  // own subtree); the facet filters by organization uuid.
  const dealerOptions = useMemo(() => dealerFilterOptions(rows), [rows]);

  const columns = useMemo(() => {
    const count = (
      key: "claim_count" | "approved_claim_count" | "rejected_claim_count",
      labelKey: string,
    ) =>
      createColumn<ByDealerRow>({
        accessorKey: key,
        labelKey,
        enableSorting: true,
        meta: NUMERIC,
        cell: ({ row }) => format.number(row.original[key]),
      });
    return [
      createColumn<ByDealerRow>({
        id: "organization",
        accessorFn: (row) => row.organization_name,
        labelKey: "warranty.claim_reports.columns.organization",
        enableSorting: true,
        enableHiding: false,
        gridPrimary: true,
        filterVariant: "faceted",
        filterOptions: dealerOptions,
        enableColumnFilter: dealerOptions.length > 0,
        filterFn: (row, _id, value: unknown) => {
          const selected = value as string[] | undefined;
          return (
            !selected?.length ||
            selected.includes(row.original.organization_uuid)
          );
        },
        cell: ({ row }) => (
          <span
            data-testid="claim-report-dealer-row"
            data-uuid={row.original.organization_uuid}
          >
            {row.original.organization_name}
          </span>
        ),
      }),
      createColumn<ByDealerRow>({
        accessorKey: "organization_type",
        labelKey: "warranty.claim_reports.columns.org_type",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: ORG_TYPES.map((value) => ({
          value,
          label: value,
          labelKey: `warranty.claim_reports.org_type.${value}`,
        })),
        cell: ({ row }) =>
          t(
            `warranty.claim_reports.org_type.${row.original.organization_type}`,
          ),
      }),
      count("claim_count", "warranty.claim_reports.columns.claims"),
      count("approved_claim_count", "warranty.claim_reports.columns.approved"),
      count("rejected_claim_count", "warranty.claim_reports.columns.rejected"),
      createColumn<ByDealerRow>({
        accessorKey: "approval_rate",
        labelKey: "warranty.claim_reports.columns.approval_rate",
        enableSorting: true,
        gridSecondary: true,
        meta: NUMERIC,
        cell: ({ row }) =>
          formatClaimRate(row.original.approval_rate, { locale }),
      }),
    ] as ColumnDef<ByDealerRow, unknown>[];
  }, [dealerOptions, format, locale, t]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("warranty.claim_reports.sections.dealer")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <ReportState
          isLoading={report.isLoading}
          isError={report.isError}
          empty={rows.length === 0}
          onRetry={() => void report.refetch()}
        >
          <div data-testid="claim-report-dealer-table">
            <EntityTable
              columns={columns}
              data={rows}
              getRowId={(row) => row.organization_uuid}
              manual={CLIENT_SIDE_MANUAL}
              initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
              emptyTitle={t("warranty.claim_reports.empty")}
              emptyDescription=""
              features={{ persistKey: CLAIM_REPORT_PERSIST_KEYS.dealer }}
            />
          </div>
        </ReportState>
      </CardContent>
    </Card>
  );
}

function PartsSection({
  query,
  enabled,
}: {
  query: ClaimReportPeriod;
  enabled: boolean;
}) {
  const { t, format } = useLocale();
  const report = useQuery({
    queryKey: claimReportKeys.parts(query),
    queryFn: () => claimReportsService.parts(query),
    enabled,
    placeholderData: keepPreviousData,
  });
  const rows = useMemo(() => report.data?.items ?? [], [report.data]);

  const columns = useMemo(() => {
    // Car part names of the service wizard; unknown keys stay as they are.
    const partLabel = (key: string) => {
      const msg = `services.parts.names.${key}`;
      const text = t(msg);
      return text === msg ? key : text;
    };
    const count = (
      key: "part_count" | "claim_count" | "approved_claim_count",
      labelKey: string,
    ) =>
      createColumn<PartsRow>({
        accessorKey: key,
        labelKey,
        enableSorting: true,
        meta: NUMERIC,
        cell: ({ row }) => format.number(row.original[key]),
      });
    return [
      createColumn<PartsRow>({
        id: "part",
        accessorFn: (row) => partLabel(row.part_key),
        labelKey: "warranty.claim_reports.columns.part",
        enableSorting: true,
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <div data-testid="claim-report-parts-row">
            <div>{partLabel(row.original.part_key)}</div>
            <div className="text-muted-foreground font-mono text-xs" dir="ltr">
              {row.original.part_key}
            </div>
          </div>
        ),
      }),
      createColumn<PartsRow>({
        id: "product",
        accessorFn: (row) => row.product_name ?? "",
        labelKey: "warranty.claim_reports.columns.product",
        enableSorting: true,
        cell: ({ row }) => (
          <div>
            <div>{row.original.product_name || "—"}</div>
            {row.original.product_sku ? (
              <div
                className="text-muted-foreground font-mono text-xs"
                dir="ltr"
              >
                {row.original.product_sku}
              </div>
            ) : null}
          </div>
        ),
      }),
      count("part_count", "warranty.claim_reports.columns.part_count"),
      count("claim_count", "warranty.claim_reports.columns.claims"),
      count("approved_claim_count", "warranty.claim_reports.columns.approved"),
    ] as ColumnDef<PartsRow, unknown>[];
  }, [format, t]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("warranty.claim_reports.sections.parts")}</CardTitle>
      </CardHeader>
      <CardContent>
        <ReportState
          isLoading={report.isLoading}
          isError={report.isError}
          empty={rows.length === 0}
          onRetry={() => void report.refetch()}
        >
          <div data-testid="claim-report-parts-table">
            <EntityTable
              columns={columns}
              data={rows}
              getRowId={(row) => `${row.part_key}:${row.product_uuid ?? ""}`}
              manual={CLIENT_SIDE_MANUAL}
              initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
              emptyTitle={t("warranty.claim_reports.empty")}
              emptyDescription=""
              features={{ persistKey: CLAIM_REPORT_PERSIST_KEYS.parts }}
            />
          </div>
        </ReportState>
      </CardContent>
    </Card>
  );
}
