"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Recycle } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { AppChart } from "@/components/charts";
import { ErrorState } from "@/components/common/error-state";
import { StatsCard } from "@/components/common/stats-card";
import {
  CLIENT_SIDE_MANUAL,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  DEFAULT_WARNING_WASTE_RATIO,
  EFFICIENCY_PERIOD_DAYS,
  efficiencyDimensions,
  efficiencyPeriod,
  formatWaste,
  metersValue,
  percentRangeParams,
  ratioValue,
  trendTotals,
  warningRowClassName,
  type EfficiencyPeriodDays,
} from "@/features/efficiency/lib/efficiency";
import {
  EFFICIENCY_ROLLS_EXPORT_PATH,
  EFFICIENCY_SUMMARY_EXPORT_PATH,
  efficiencyKeys,
  efficiencyService,
  type EfficiencyDimension,
  type EfficiencyPeriod,
  type EfficiencyRollRow,
  type EfficiencyRollService,
  type EfficiencySummaryRow,
} from "@/features/efficiency/services/efficiency.service";
import { ExportMenu } from "@/features/io/components/export-menu";
import { isKnownPart } from "@/features/services/lib/car-parts";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const EFFICIENCY_PAGE_SIZE = 20;
export const EFFICIENCY_ROLLS_PERSIST_KEY = "tenant-efficiency-rolls-v1";

export function efficiencySummaryPersistKey(dimension: EfficiencyDimension) {
  return `tenant-efficiency-${dimension}-v1`;
}

type Translate = ReturnType<typeof useLocale>["t"];

function dimensionLabel(
  dimension: EfficiencyDimension,
  row: EfficiencySummaryRow,
  t: Translate,
) {
  if (dimension === "body_type" && !row.dimension_label) {
    return t("efficiency.body_type_all");
  }
  if (dimension === "part" && isKnownPart(row.dimension_label)) {
    return t(`services.parts.names.${row.dimension_label}`);
  }
  return row.dimension_label || "—";
}

function SummaryTab({
  slug,
  dimension,
  period,
  threshold,
}: {
  slug: string;
  dimension: EfficiencyDimension;
  period: EfficiencyPeriod;
  threshold: number;
}) {
  const { t, format } = useLocale();
  const persistKey = efficiencySummaryPersistKey(dimension);
  const columns = useMemo(
    () =>
      [
        createColumn<EfficiencySummaryRow>({
          id: "dimension_label",
          accessorKey: "dimension_label",
          labelKey: `efficiency.dimensions.${dimension}`,
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {dimensionLabel(dimension, row.original, t)}
            </span>
          ),
        }),
        createColumn<EfficiencySummaryRow>({
          accessorKey: "services",
          labelKey: "efficiency.columns.services",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => format.number(row.original.services),
        }),
        createColumn<EfficiencySummaryRow>({
          accessorKey: "meters",
          labelKey: "efficiency.columns.meters",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => format.number(metersValue(row.original.meters)),
        }),
        createColumn<EfficiencySummaryRow>({
          accessorKey: "expected_meters",
          labelKey: "efficiency.columns.expected_meters",
          enableSorting: false,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            row.original.expected_meters == null
              ? "—"
              : format.number(metersValue(row.original.expected_meters)),
        }),
        createColumn<EfficiencySummaryRow>({
          accessorKey: "waste_ratio",
          labelKey: "efficiency.columns.waste_ratio",
          enableSorting: true,
          filterVariant: "number-range",
          param: "waste_ratio",
          paramFormat: percentRangeParams("waste_ratio"),
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => (
            <span data-testid="efficiency-waste-cell">
              {formatWaste(row.original.waste_ratio, format.percent)}
            </span>
          ),
        }),
      ] satisfies ColumnDef<EfficiencySummaryRow, unknown>[],
    [dimension, format, t],
  );
  const listState = useServerListState({
    columns,
    initialSort: "-waste_ratio",
    initialPageSize: EFFICIENCY_PAGE_SIZE,
    persistKey,
  });
  const query = { ...listState.params, ...period };
  const list = useQuery({
    queryKey: efficiencyKeys.summary(dimension, query),
    queryFn: () => efficiencyService.summary(dimension, query),
  });

  return (
    <EntityTable
      columns={columns}
      data={list.data?.items ?? []}
      getRowId={(row) => `${dimension}:${row.dimension_key}`}
      rowCount={list.data?.total ?? 0}
      isLoading={list.isLoading}
      isError={list.isError}
      onRetry={() => void list.refetch()}
      state={listState.tableState}
      getRowClassName={(row) => warningRowClassName(row.waste_ratio, threshold)}
      features={{
        persistKey,
        globalFilter: false,
        rowSelection: false,
        columnOrdering: true,
        columnPinning: true,
      }}
      toolbarExtra={
        <EntityToolbar>
          <ExportMenu
            exportPath={EFFICIENCY_SUMMARY_EXPORT_PATH}
            query={{
              ...listState.filterParams,
              ...period,
              dimension,
              sort: listState.params.sort,
            }}
            jobsHref={routes.tenant.exports.root(slug)}
            formats={["xlsx", "csv", "pdf"]}
          />
        </EntityToolbar>
      }
      emptyTitle={t("efficiency.empty.title")}
      emptyDescription={t("efficiency.empty.description")}
    />
  );
}

function RollsTab({
  slug,
  threshold,
  onOpen,
}: {
  slug: string;
  threshold: number;
  onOpen: (row: EfficiencyRollRow) => void;
}) {
  const { t, format } = useLocale();
  const columns = useMemo(() => {
    const meters = (value: string) => format.number(metersValue(value));
    return [
      createColumn<EfficiencyRollRow>({
        accessorKey: "barcode",
        labelKey: "efficiency.rolls.barcode",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-mono text-sm">{row.original.barcode}</span>
        ),
      }),
      createColumn<EfficiencyRollRow>({
        accessorKey: "product_name",
        labelKey: "efficiency.rolls.product",
        enableSorting: false,
        gridSecondary: true,
      }),
      createColumn<EfficiencyRollRow>({
        accessorKey: "initial_meters",
        labelKey: "efficiency.rolls.initial",
        enableSorting: false,
        meta: { cellClassName: "text-end tabular-nums" },
        cell: ({ row }) => meters(row.original.initial_meters),
      }),
      createColumn<EfficiencyRollRow>({
        accessorKey: "consumed_meters",
        labelKey: "efficiency.rolls.consumed",
        enableSorting: true,
        meta: { cellClassName: "text-end tabular-nums" },
        cell: ({ row }) => meters(row.original.consumed_meters),
      }),
      createColumn<EfficiencyRollRow>({
        accessorKey: "waste_meters",
        labelKey: "efficiency.rolls.waste",
        enableSorting: false,
        meta: { cellClassName: "text-end tabular-nums" },
        cell: ({ row }) => meters(row.original.waste_meters),
      }),
      createColumn<EfficiencyRollRow>({
        accessorKey: "remaining_meters",
        labelKey: "efficiency.rolls.remaining",
        enableSorting: true,
        meta: { cellClassName: "text-end tabular-nums" },
        cell: ({ row }) => meters(row.original.remaining_meters),
      }),
      createColumn<EfficiencyRollRow>({
        accessorKey: "service_count",
        labelKey: "efficiency.rolls.service_count",
        enableSorting: false,
        meta: { cellClassName: "text-end tabular-nums" },
        cell: ({ row }) => format.number(row.original.service_count),
      }),
      createColumn<EfficiencyRollRow>({
        accessorKey: "last_used_at",
        labelKey: "efficiency.rolls.last_used",
        enableSorting: true,
        filterVariant: "date-range",
        param: "last_used",
        cell: ({ row }) =>
          row.original.last_used_at
            ? format.dateTime(row.original.last_used_at)
            : "—",
      }),
      createColumn<EfficiencyRollRow>({
        accessorKey: "waste_ratio",
        labelKey: "efficiency.columns.waste_ratio",
        enableSorting: true,
        filterVariant: "number-range",
        param: "waste_ratio",
        paramFormat: percentRangeParams("waste_ratio"),
        meta: { cellClassName: "text-end tabular-nums" },
        cell: ({ row }) => (
          <span data-testid="efficiency-waste-cell">
            {formatWaste(row.original.waste_ratio, format.percent)}
          </span>
        ),
      }),
      createColumn<EfficiencyRollRow>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => (
          <EntityRowActions
            actions={[
              {
                id: "view",
                label: t("common.view"),
                icon: Eye,
                onSelect: () => onOpen(row.original),
              },
            ]}
          />
        ),
      }),
    ] satisfies ColumnDef<EfficiencyRollRow, unknown>[];
  }, [format, onOpen, t]);
  const listState = useServerListState({
    columns,
    initialSort: "-waste_ratio",
    initialPageSize: EFFICIENCY_PAGE_SIZE,
    persistKey: EFFICIENCY_ROLLS_PERSIST_KEY,
  });
  const list = useQuery({
    queryKey: efficiencyKeys.rolls(listState.params),
    queryFn: () => efficiencyService.rolls(listState.params),
  });

  return (
    <EntityTable
      columns={columns}
      data={list.data?.items ?? []}
      getRowId={(row) => row.uuid}
      rowCount={list.data?.total ?? 0}
      isLoading={list.isLoading}
      isError={list.isError}
      onRetry={() => void list.refetch()}
      state={listState.tableState}
      onRowClick={onOpen}
      getRowClassName={(row) => warningRowClassName(row.waste_ratio, threshold)}
      features={{
        persistKey: EFFICIENCY_ROLLS_PERSIST_KEY,
        rowSelection: false,
        columnOrdering: true,
        columnPinning: true,
      }}
      toolbarExtra={
        <EntityToolbar>
          <ExportMenu
            exportPath={EFFICIENCY_ROLLS_EXPORT_PATH}
            query={{
              ...listState.filterParams,
              q: listState.params.q,
              sort: listState.params.sort,
            }}
            jobsHref={routes.tenant.exports.root(slug)}
            formats={["xlsx", "csv", "pdf"]}
          />
        </EntityToolbar>
      }
      emptyTitle={t("efficiency.rolls.empty")}
    />
  );
}

function RollDrawer({
  slug,
  roll,
  onOpenChange,
}: {
  slug: string;
  roll: EfficiencyRollRow | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, format } = useLocale();
  const uuid = roll?.uuid ?? "";
  const detail = useQuery({
    queryKey: efficiencyKeys.roll(uuid),
    queryFn: () => efficiencyService.roll(uuid),
    enabled: Boolean(uuid),
  });
  const columns = useMemo(
    () =>
      [
        createColumn<EfficiencyRollService>({
          accessorKey: "service_no",
          labelKey: "efficiency.rolls.service_no",
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              className="font-mono text-sm hover:underline"
              href={routes.tenant.services.detail(slug, row.original.uuid)}
            >
              {row.original.service_no}
            </Link>
          ),
        }),
        createColumn<EfficiencyRollService>({
          accessorKey: "service_date",
          labelKey: "efficiency.rolls.service_date",
          cell: ({ row }) => format.date(row.original.service_date),
        }),
        createColumn<EfficiencyRollService>({
          accessorKey: "dealer_name",
          labelKey: "efficiency.dimensions.dealer",
        }),
        createColumn<EfficiencyRollService>({
          accessorFn: (row) => metersValue(row.actual_meters),
          id: "actual_meters",
          labelKey: "efficiency.columns.meters",
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            format.number(metersValue(row.original.actual_meters)),
        }),
        createColumn<EfficiencyRollService>({
          accessorFn: (row) => ratioValue(row.waste_ratio),
          id: "waste_ratio",
          labelKey: "efficiency.columns.waste_ratio",
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            formatWaste(row.original.waste_ratio, format.percent),
        }),
      ] satisfies ColumnDef<EfficiencyRollService, unknown>[],
    [format, slug],
  );
  const data = detail.data ?? roll;

  return (
    <Sheet open={Boolean(roll)} onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-auto sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle className="font-mono">{roll?.barcode ?? ""}</SheetTitle>
          <SheetDescription>{roll?.product_name ?? ""}</SheetDescription>
        </SheetHeader>
        {data ? (
          <dl className="mt-6 grid gap-3 sm:grid-cols-4">
            <Stat
              label={t("efficiency.rolls.initial")}
              value={format.number(metersValue(data.initial_meters))}
            />
            <Stat
              label={t("efficiency.rolls.consumed")}
              value={format.number(metersValue(data.consumed_meters))}
            />
            <Stat
              label={t("efficiency.rolls.remaining")}
              value={format.number(metersValue(data.remaining_meters))}
            />
            <Stat
              label={t("efficiency.columns.waste_ratio")}
              value={formatWaste(data.waste_ratio, format.percent)}
            />
          </dl>
        ) : null}
        <section className="mt-6 space-y-3">
          <h3 className="font-semibold">{t("efficiency.rolls.services")}</h3>
          <EntityTable
            columns={columns}
            data={detail.data?.services ?? []}
            getRowId={(row) => row.uuid}
            isLoading={detail.isLoading}
            isError={detail.isError}
            onRetry={() => void detail.refetch()}
            manual={CLIENT_SIDE_MANUAL}
            features={{
              persistKey: "tenant-efficiency-roll-services-v1",
              rowSelection: false,
              columnFilters: false,
              viewMode: false,
            }}
            emptyTitle={t("efficiency.rolls.services_empty")}
          />
        </section>
      </SheetContent>
    </Sheet>
  );
}

/** Ratio → percent with one decimal for the trend line (null stays a gap). */
function wastePercent(value: unknown) {
  const ratio = ratioValue(value);
  return ratio == null ? null : Math.round(ratio * 1000) / 10;
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border p-3">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="font-semibold tabular-nums">{value}</dd>
    </div>
  );
}

export function EfficiencyPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const canRead = can(Permission.EfficiencyRead);
  const [days, setDays] = useState<EfficiencyPeriodDays>(90);
  const period = useMemo(() => efficiencyPeriod(days), [days]);
  const dimensions = efficiencyDimensions(org?.type);
  const [tab, setTab] = useState<string>("");
  const activeTab =
    tab && (tab === "rolls" || dimensions.includes(tab as EfficiencyDimension))
      ? tab
      : (dimensions[0] ?? "rolls");
  const [roll, setRoll] = useState<EfficiencyRollRow | null>(null);

  const settings = useQuery({
    queryKey: efficiencyKeys.settings,
    queryFn: () => efficiencyService.settings(),
    enabled: canRead,
    staleTime: 5 * 60 * 1000,
  });
  const threshold =
    ratioValue(settings.data?.warning_waste_ratio) ??
    DEFAULT_WARNING_WASTE_RATIO;
  const trend = useQuery({
    queryKey: efficiencyKeys.trend(period),
    queryFn: () => efficiencyService.trend(period),
    enabled: canRead,
  });
  const compare = useQuery({
    queryKey: efficiencyKeys.compare(period),
    queryFn: () => efficiencyService.compare(period),
    enabled: canRead,
  });
  const topQuery = { limit: 1, offset: 0, sort: "-waste_ratio", ...period };
  const top = useQuery({
    queryKey: efficiencyKeys.summary("product", topQuery),
    queryFn: () => efficiencyService.summary("product", topQuery),
    enabled: canRead,
  });

  const totals = trendTotals(trend.data ?? []);
  const topProduct = top.data?.items[0];
  const trendData = (trend.data ?? []).map((row) => ({
    month: row.month,
    waste: wastePercent(row.waste_ratio),
    meters: metersValue(row.meters),
  }));
  const compareData = (compare.data ?? [])
    // A center's own scope is the network: only its own and network bars.
    .filter((row) => !(org?.type === "center" && row.bucket === "subtree"))
    .map((row) => ({
      bucket: t(
        row.bucket === "subtree"
          ? `efficiency.compare.subtree_${org?.type ?? "dealer"}`
          : `efficiency.compare.${row.bucket}`,
      ),
      waste: ratioValue(row.waste_ratio) ?? 0,
    }));

  const title = t("efficiency.page.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Recycle className="size-6" />}
      description={t("efficiency.page.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        canRead ? (
          <Select
            value={String(days)}
            onValueChange={(v) => setDays(Number(v) as EfficiencyPeriodDays)}
          >
            <SelectTrigger
              className="w-44"
              aria-label={t("efficiency.period.label")}
              data-testid="efficiency-period"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {EFFICIENCY_PERIOD_DAYS.map((d) => (
                <SelectItem key={d} value={String(d)}>
                  {t("efficiency.period.days", { count: d })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
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
          description={t("efficiency.page.forbidden")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatsCard
          title={t("efficiency.kpi.avg_waste")}
          value={formatWaste(totals.wasteRatio, format.percent)}
          hint={t("efficiency.kpi.threshold", {
            value: format.percent(threshold, 1),
          })}
        />
        <StatsCard
          title={t("efficiency.kpi.meters")}
          value={format.number(totals.meters)}
        />
        <StatsCard
          title={t("efficiency.kpi.services")}
          value={format.number(totals.services)}
        />
        <StatsCard
          title={t("efficiency.kpi.top_product")}
          value={topProduct?.dimension_label || "—"}
          hint={
            topProduct
              ? formatWaste(topProduct.waste_ratio, format.percent)
              : undefined
          }
        />
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        {compare.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void compare.refetch()}
          />
        ) : (
          <AppChart
            type="bar"
            title={t("efficiency.compare.title")}
            data={compareData}
            categoryKey="bucket"
            config={{ waste: { label: t("efficiency.columns.waste_ratio") } }}
            loading={compare.isLoading}
            emptyTitle={t("efficiency.empty.title")}
            valueFormatter={(v) => format.percent(v, 1)}
            height={240}
          />
        )}
        {trend.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void trend.refetch()}
          />
        ) : (
          <AppChart
            type="composed"
            title={t("efficiency.trend.title")}
            data={trendData}
            categoryKey="month"
            config={{
              waste: { label: t("efficiency.trend.waste_pct") },
              meters: { label: t("efficiency.columns.meters") },
            }}
            series={[
              { key: "meters", type: "bar", yAxisId: "right" },
              { key: "waste", type: "line" },
            ]}
            showRightYAxis
            loading={trend.isLoading}
            emptyTitle={t("efficiency.empty.title")}
            tickValueFormatter={(v) => format.number(v)}
            valueFormatter={(v) => format.number(v)}
            height={240}
          />
        )}
      </div>
      <Tabs value={activeTab} onValueChange={setTab}>
        <TabsList className="flex-wrap">
          {dimensions.map((dimension) => (
            <TabsTrigger
              key={dimension}
              value={dimension}
              data-testid={`efficiency-tab-${dimension}`}
            >
              {t(`efficiency.tabs.${dimension}`)}
            </TabsTrigger>
          ))}
          <TabsTrigger value="rolls" data-testid="efficiency-tab-rolls">
            {t("efficiency.tabs.rolls")}
          </TabsTrigger>
        </TabsList>
        {dimensions.map((dimension) => (
          <TabsContent key={dimension} value={dimension} className="mt-4">
            {activeTab === dimension ? (
              <SummaryTab
                slug={slug}
                dimension={dimension}
                period={period}
                threshold={threshold}
              />
            ) : null}
          </TabsContent>
        ))}
        <TabsContent value="rolls" className="mt-4">
          {activeTab === "rolls" ? (
            <RollsTab slug={slug} threshold={threshold} onOpen={setRoll} />
          ) : null}
        </TabsContent>
      </Tabs>
      <RollDrawer
        slug={slug}
        roll={roll}
        onOpenChange={(open) => {
          if (!open) setRoll(null);
        }}
      />
    </div>
  );
}
