"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Money } from "@/features/accounting/components/shared";
import { useCountries } from "@/features/geo/hooks/use-geo";
import { dayValue } from "@/features/pricing/lib/recommended";
import {
  pricingKeys,
  recommendedService,
  type ListQuery,
  type RecommendedPriceVersion,
} from "@/features/pricing/services/recommended.service";
import { useLocale } from "@/providers/locale-provider";

export const RECOMMENDED_HISTORY_PERSIST_KEY =
  "tenant-recommended-price-history-v1";

const STATUS_VARIANT = {
  current: "success",
  scheduled: "warning",
  superseded: "outline",
} as const;

const SOURCES = ["publish", "price_list", "migration"] as const;
const CURRENCIES = ["TRY", "EUR", "USD"];

/**
 * Recommended price version history (TEC-506 list contract): sort
 * effective_from (default -effective_from), published_at, price; filters
 * country (ISO2 CSV, `none` = currency-wide), currency, source, batch,
 * effective day range and q.
 */
export function RecommendedHistory() {
  const { t, format, locale } = useLocale();
  const countries = useCountries();

  const countryOptions = useMemo(
    () => [
      {
        value: "none",
        label: t("catalog.recommended.scope_currency"),
      },
      ...(countries.data ?? []).map((c) => ({
        value: c.iso2,
        label: `${c.iso2} · ${locale === "tr" ? c.name_tr : c.name_en}`,
      })),
    ],
    [countries.data, locale, t],
  );
  const currencyOptions = useMemo(() => {
    const set = new Set(CURRENCIES);
    for (const c of countries.data ?? []) {
      if (c.default_currency) set.add(c.default_currency);
    }
    return [...set].sort().map((c) => ({ value: c, label: c }));
  }, [countries.data]);

  const columns = useMemo(
    () =>
      [
        createColumn<RecommendedPriceVersion>({
          accessorKey: "product_name",
          labelKey: "catalog.fields.name",
          enableSorting: false,
          enableHiding: false,
          enableColumnFilter: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.product_name}</span>
          ),
        }),
        createColumn<RecommendedPriceVersion>({
          accessorKey: "product_sku",
          labelKey: "catalog.fields.sku",
          enableSorting: false,
          enableColumnFilter: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.product_sku}
            </span>
          ),
        }),
        createColumn<RecommendedPriceVersion>({
          accessorKey: "country_iso2",
          labelKey: "catalog.recommended.fields.country",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: countryOptions,
          param: "country",
          cell: ({ row }) =>
            row.original.country_iso2 ? (
              <span className="font-mono text-xs">
                {row.original.country_iso2}
              </span>
            ) : (
              <span className="text-muted-foreground text-xs">
                {t("catalog.recommended.scope_currency")}
              </span>
            ),
        }),
        createColumn<RecommendedPriceVersion>({
          accessorKey: "currency",
          labelKey: "catalog.recommended.fields.currency",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: currencyOptions,
          param: "currency",
          cell: ({ row }) => (
            <span className="font-mono text-xs">{row.original.currency}</span>
          ),
        }),
        createColumn<RecommendedPriceVersion>({
          accessorKey: "price",
          labelKey: "catalog.recommended.fields.price",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <Money
              amount={row.original.price}
              currency={row.original.currency}
            />
          ),
        }),
        createColumn<RecommendedPriceVersion>({
          accessorKey: "effective_from",
          labelKey: "catalog.recommended.fields.effective_from",
          enableSorting: true,
          filterVariant: "date-range",
          param: "effective_from",
          cell: ({ row }) => format.date(dayValue(row.original.effective_from)),
        }),
        createColumn<RecommendedPriceVersion>({
          accessorKey: "status",
          labelKey: "catalog.recommended.fields.status",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <Badge variant={STATUS_VARIANT[row.original.status]}>
              {t(`catalog.recommended.status.${row.original.status}`)}
            </Badge>
          ),
        }),
        createColumn<RecommendedPriceVersion>({
          accessorKey: "source",
          labelKey: "catalog.recommended.fields.source",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: SOURCES.map((s) => ({
            value: s,
            label: s,
            labelKey: `catalog.recommended.sources.${s}`,
          })),
          param: "source",
          cell: ({ row }) =>
            t(`catalog.recommended.sources.${row.original.source}`),
        }),
        createColumn<RecommendedPriceVersion>({
          accessorKey: "note",
          labelKey: "catalog.recommended.fields.note",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.note ? (
              <span className="line-clamp-2 text-xs">{row.original.note}</span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<RecommendedPriceVersion>({
          accessorKey: "published_at",
          labelKey: "catalog.recommended.fields.published_at",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) => format.dateTime(row.original.published_at),
        }),
        createColumn<RecommendedPriceVersion>({
          accessorKey: "published_by_name",
          labelKey: "catalog.recommended.fields.published_by",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => row.original.published_by_name || "—",
        }),
        createColumn<RecommendedPriceVersion>({
          id: "batch_id",
          accessorFn: (row) => row.batch_id ?? "",
          labelKey: "catalog.recommended.fields.batch",
          enableSorting: false,
          filterVariant: "text",
          param: "batch_id",
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.batch_id ?? "—"}
            </span>
          ),
        }),
        createColumn<RecommendedPriceVersion>({
          accessorKey: "superseded_at",
          labelKey: "catalog.recommended.fields.superseded_at",
          enableSorting: false,
          enableColumnFilter: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.superseded_at
              ? format.dateTime(row.original.superseded_at)
              : "—",
        }),
      ] as ColumnDef<RecommendedPriceVersion, unknown>[],
    [countryOptions, currencyOptions, format, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-effective_from",
    persistKey: RECOMMENDED_HISTORY_PERSIST_KEY,
  });
  const params: ListQuery = listState.params;
  const list = useQuery({
    queryKey: pricingKeys.versions(params),
    queryFn: () => recommendedService.versions(params),
  });

  return (
    <EntityTable
      columns={columns}
      data={list.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={list.isLoading}
      isError={list.isError}
      onRetry={() => void list.refetch()}
      emptyTitle={t("catalog.recommended.history_empty")}
      rowCount={list.data?.total ?? 0}
      state={listState.tableState}
      features={{
        persistKey: RECOMMENDED_HISTORY_PERSIST_KEY,
        rowSelection: false,
        columnOrdering: true,
        columnPinning: true,
      }}
      toolbarExtra={
        <EntityToolbar
          onRefresh={() => void list.refetch()}
          refreshDisabled={list.isFetching}
        />
      }
    />
  );
}
