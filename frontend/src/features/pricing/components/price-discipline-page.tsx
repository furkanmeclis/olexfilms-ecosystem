"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { Money } from "@/features/accounting/components/shared";
import { pricingService } from "@/features/catalog/services/pricing.service";
import { ExportMenu } from "@/features/io/components/export-menu";
import { DeviationBadge } from "@/features/pricing/components/deviation-badge";
import { DisciplineChart } from "@/features/pricing/components/discipline-chart";
import {
  dayValue,
  DEFAULT_DEVIATION_THRESHOLD,
  toNumber,
} from "@/features/pricing/lib/recommended";
import {
  DISCIPLINE_EXPORT_PATH,
  pricingKeys,
  recommendedService,
  type DisciplineCountry,
  type ListQuery,
  type PriceDisciplineRow,
} from "@/features/pricing/services/recommended.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";

export const PRICE_DISCIPLINE_PERSIST_KEY = "tenant-price-discipline-v1";

/**
 * Tenant > Catalog > Price discipline (TEC-507). pricing.discipline.read:
 * the center sees the brand, a distributor its subtree. Country summary
 * cards and the average deviation chart of the snapshot day, then the
 * organization × product list (TEC-506 list contract) with deviation range,
 * over-threshold, country / currency / distributor filters and export.
 */
export function PriceDisciplinePage({ slug }: { slug: string }) {
  const { t, format, locale } = useLocale();
  const org = useActiveOrganization(slug);
  const isCenter = org?.type === "center";
  const [date, setDate] = useState("");

  const summary = useQuery({
    queryKey: pricingKeys.disciplineSummary(date),
    queryFn: () => recommendedService.disciplineSummary(date),
  });
  const threshold = summary.data?.threshold_pct ?? DEFAULT_DEVIATION_THRESHOLD;
  const distributors = useQuery({
    queryKey: ["pricing", "discipline", "distributors"],
    queryFn: () => pricingService.listDistributors(),
    enabled: isCenter,
    staleTime: 5 * 60 * 1000,
  });

  const countryOptions = useMemo(() => {
    const seen = new Map<string, string>();
    for (const c of summary.data?.countries ?? []) {
      if (c.country_iso2 && !seen.has(c.country_iso2)) {
        seen.set(
          c.country_iso2,
          locale === "tr" ? c.country_name_tr : c.country_name_en,
        );
      }
    }
    return [...seen].map(([value, name]) => ({
      value,
      label: `${value} · ${name}`,
    }));
  }, [locale, summary.data?.countries]);
  const currencyOptions = useMemo(
    () =>
      [...new Set((summary.data?.countries ?? []).map((c) => c.currency))]
        .sort()
        .map((c) => ({ value: c, label: c })),
    [summary.data?.countries],
  );
  const distributorOptions = useMemo(
    () =>
      (distributors.data ?? []).map((d) => ({ value: d.uuid, label: d.name })),
    [distributors.data],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<PriceDisciplineRow>({
          id: "org_name",
          accessorFn: (row) => row.organization_name,
          labelKey: "catalog.discipline.fields.organization",
          enableSorting: true,
          enableHiding: false,
          enableColumnFilter: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-2">
              <span className="font-medium">
                {row.original.organization_name}
              </span>
              <Badge variant="outline">
                {t(
                  `catalog.discipline.org_types.${row.original.organization_type}`,
                )}
              </Badge>
            </span>
          ),
        }),
        ...(isCenter
          ? [
              createColumn<PriceDisciplineRow>({
                id: "distributor",
                accessorFn: () => "",
                labelKey: "catalog.discipline.fields.distributor",
                enableSorting: false,
                enableHiding: false,
                filterVariant: "faceted",
                filterOptions: distributorOptions,
                param: "distributor",
                defaultHidden: true,
                cell: () => null,
              }),
            ]
          : []),
        createColumn<PriceDisciplineRow>({
          id: "product_name",
          accessorFn: (row) => row.product_name,
          labelKey: "catalog.discipline.fields.product",
          enableSorting: true,
          enableColumnFilter: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span>{row.original.product_name}</span>
              <span
                className="text-muted-foreground font-mono text-xs"
                dir="ltr"
              >
                {row.original.product_sku}
              </span>
            </span>
          ),
        }),
        createColumn<PriceDisciplineRow>({
          accessorKey: "country_iso2",
          labelKey: "catalog.recommended.fields.country",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: countryOptions,
          param: "country",
          cell: ({ row }) => (
            <span className="font-mono text-xs">
              {row.original.country_iso2 || "—"}
            </span>
          ),
        }),
        createColumn<PriceDisciplineRow>({
          accessorKey: "currency",
          labelKey: "catalog.recommended.fields.currency",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: currencyOptions,
          param: "currency",
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs">{row.original.currency}</span>
          ),
        }),
        createColumn<PriceDisciplineRow>({
          accessorKey: "list_price",
          labelKey: "catalog.discipline.fields.list_price",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <Money
              amount={row.original.list_price}
              currency={row.original.currency}
            />
          ),
        }),
        createColumn<PriceDisciplineRow>({
          accessorKey: "recommended_price",
          labelKey: "catalog.recommended.column",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <Money
              amount={row.original.recommended_price}
              currency={row.original.currency}
            />
          ),
        }),
        createColumn<PriceDisciplineRow>({
          id: "deviation_pct",
          accessorFn: (row) => toNumber(row.deviation_pct),
          labelKey: "catalog.discipline.fields.deviation",
          enableSorting: true,
          filterVariant: "number-range",
          param: "deviation_pct",
          cell: ({ row }) => (
            <DeviationBadge
              value={row.original.deviation_pct}
              threshold={threshold}
            />
          ),
        }),
        createColumn<PriceDisciplineRow>({
          accessorKey: "over_threshold",
          labelKey: "catalog.discipline.fields.over_threshold",
          enableSorting: false,
          filterVariant: "boolean",
          param: "over_threshold",
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.over_threshold ? t("table.true") : t("table.false"),
        }),
        createColumn<PriceDisciplineRow>({
          accessorKey: "avg_sale_price",
          labelKey: "catalog.discipline.fields.avg_sale_price",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.avg_sale_price ? (
              <Money
                amount={row.original.avg_sale_price}
                currency={row.original.currency}
              />
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<PriceDisciplineRow>({
          accessorKey: "sales_quantity",
          labelKey: "catalog.discipline.fields.sales_quantity",
          enableSorting: false,
          enableColumnFilter: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {format.number(toNumber(row.original.sales_quantity))}
            </span>
          ),
        }),
      ] as ColumnDef<PriceDisciplineRow, unknown>[],
    [
      countryOptions,
      currencyOptions,
      distributorOptions,
      format,
      isCenter,
      t,
      threshold,
    ],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-deviation_pct",
    persistKey: PRICE_DISCIPLINE_PERSIST_KEY,
  });
  const params: ListQuery = useMemo(
    () => ({ ...listState.params, date: date || undefined }),
    [date, listState.params],
  );
  const list = useQuery({
    queryKey: pricingKeys.discipline(params),
    queryFn: () => recommendedService.discipline(params),
  });
  const exportQuery = useMemo(
    () => ({
      ...listState.filterParams,
      q: params.q,
      sort: params.sort,
      date: date || undefined,
    }),
    [date, listState.filterParams, params.q, params.sort],
  );

  const title = t("catalog.discipline.title");
  return (
    <EntityPage
      title={title}
      description={t("catalog.discipline.description", { threshold })}
      permission={permissions.pricing.disciplineRead}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("catalog.discipline.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.section_catalog"),
          href: routes.tenant.catalog.products(slug),
        },
        { label: title },
      ]}
    >
      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1">
          <Label htmlFor="discipline-date">
            {t("catalog.discipline.snapshot_date")}
          </Label>
          <DatePicker
            id="discipline-date"
            className="w-48"
            value={date}
            onChange={setDate}
          />
        </div>
        <p className="text-muted-foreground text-xs">
          {summary.data?.snapshot_date
            ? t("catalog.discipline.snapshot_of", {
                date: format.date(dayValue(summary.data.snapshot_date)),
              })
            : t("catalog.discipline.no_snapshot")}
        </p>
      </div>

      {summary.isLoading ? (
        <Loading />
      ) : summary.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void summary.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : summary.data && summary.data.countries.length > 0 ? (
        <div className="grid gap-4 lg:grid-cols-2">
          <div
            className="grid gap-3 sm:grid-cols-2"
            data-testid="discipline-cards"
          >
            {summary.data.countries.map((c) => (
              <CountryCard
                key={`${c.country_iso2}-${c.currency}`}
                country={c}
                threshold={threshold}
              />
            ))}
          </div>
          <Card>
            <CardHeader>
              <CardTitle>{t("catalog.discipline.chart_title")}</CardTitle>
              <CardDescription>
                {t("catalog.discipline.chart_description", { threshold })}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <DisciplineChart
                countries={summary.data.countries}
                threshold={threshold}
              />
            </CardContent>
          </Card>
        </div>
      ) : null}

      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) =>
          `${row.organization_uuid}-${row.product_uuid}-${row.currency}`
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("catalog.discipline.empty")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        getRowClassName={(row) =>
          row.over_threshold ? "bg-amber-500/5" : undefined
        }
        features={{
          persistKey: PRICE_DISCIPLINE_PERSIST_KEY,
          rowSelection: false,
          columnOrdering: true,
          columnPinning: true,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => {
              void list.refetch();
              void summary.refetch();
            }}
            refreshDisabled={list.isFetching}
          >
            <ExportMenu
              exportPath={DISCIPLINE_EXPORT_PATH}
              query={exportQuery}
              formats={["csv", "xlsx", "pdf"]}
              jobsHref={routes.tenant.exports.root(slug)}
            />
          </EntityToolbar>
        }
      />
    </EntityPage>
  );
}

function CountryCard({
  country,
  threshold,
}: {
  country: DisciplineCountry;
  threshold: number;
}) {
  const { t, format, locale } = useLocale();
  const name =
    (locale === "tr" ? country.country_name_tr : country.country_name_en) ||
    t("catalog.recommended.scope_currency");
  return (
    <Card data-testid="discipline-country-card">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">
          {name} · <span className="font-mono">{country.currency}</span>
        </CardTitle>
        <CardDescription>
          {t("catalog.discipline.org_count", { count: country.org_count })}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-2 text-sm">
        <div className="flex items-center justify-between gap-2">
          <span className="text-muted-foreground">
            {t("catalog.discipline.avg_deviation")}
          </span>
          <DeviationBadge
            value={country.avg_deviation_pct}
            threshold={threshold}
          />
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-muted-foreground">
            {t("catalog.discipline.median_deviation")}
          </span>
          <DeviationBadge
            value={country.median_deviation_pct}
            threshold={threshold}
          />
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-muted-foreground">
            {t("catalog.discipline.over_threshold_orgs")}
          </span>
          <span className="font-medium tabular-nums">
            {format.number(country.over_threshold_org_count)}
          </span>
        </div>
      </CardContent>
    </Card>
  );
}
