"use client";

import { Eye, Trash2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityFilters,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  countActiveFilters,
  useServerListState,
  type EntityFilterDef,
  type EntityFilterValues,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Card, CardContent } from "@/components/common/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { LogDetailDrawer } from "@/features/logs/components/log-detail-drawer";
import { PurgeRulesPanel } from "@/features/logs/components/purge-rules-panel";
import { logsKeys } from "@/features/logs/hooks/query-keys";
import { useDeleteLog } from "@/features/logs/hooks/use-log-mutations";
import {
  logsService,
  type AppLog,
  type ListLogsParams,
} from "@/features/logs/services/logs.service";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function firstString(value: string | string[] | undefined) {
  if (Array.isArray(value)) return value[0];
  return value;
}

function levelTone(level: AppLog["level"]) {
  if (level === "error") return "danger" as const;
  if (level === "warn") return "warning" as const;
  return "default" as const;
}

function StatsCards() {
  const { t } = useLocale();
  const statsQuery = useQuery({
    queryKey: logsKeys.stats(),
    queryFn: () => logsService.stats(),
    staleTime: 30_000,
  });

  const stats = statsQuery.data;
  const items = [
    { key: "error", value: stats?.error ?? 0, tone: "danger" as const },
    { key: "warn", value: stats?.warn ?? 0, tone: "warning" as const },
    { key: "debug", value: stats?.debug ?? 0, tone: "default" as const },
  ];

  return (
    <div className="grid gap-3 sm:grid-cols-3">
      {items.map((item) => (
        <Card key={item.key} className="shadow-none">
          <CardContent className="flex items-center justify-between p-4">
            <div>
              <p className="text-muted-foreground text-xs">
                {t(`logs.levels.${item.key}`)}
              </p>
              <p className="text-2xl font-semibold tabular-nums">
                {item.value}
              </p>
            </div>
            <StatusChip label={t(`logs.levels.${item.key}`)} tone={item.tone} />
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

function LogsListPanel() {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const { confirmDelete } = useDialogs();
  const deleteLog = useDeleteLog();

  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });

  const [filtersOpen, setFiltersOpen] = useState(false);
  const [draftFilters, setDraftFilters] = useState<EntityFilterValues>({});
  const [appliedFilters, setAppliedFilters] = useState<EntityFilterValues>({});
  const [selected, setSelected] = useState<AppLog | null>(null);

  const sourcesQuery = useQuery({
    queryKey: logsKeys.sources(),
    queryFn: () => logsService.sources(),
    staleTime: 60_000,
  });

  const listParams = useMemo<ListLogsParams>(() => {
    const level = firstString(appliedFilters.level);
    const source = firstString(appliedFilters.source);
    const createdFrom = firstString(appliedFilters.created_from);
    const createdTo = firstString(appliedFilters.created_to);

    return {
      ...listState.params,
      q: listState.params.q,
      level,
      source,
      created_from: createdFrom,
      created_to: createdTo,
    };
  }, [appliedFilters, listState.params]);

  const listQuery = useQuery({
    queryKey: logsKeys.list(listParams),
    queryFn: () => logsService.list(listParams),
  });

  const filterDefs = useMemo<EntityFilterDef[]>(
    () => [
      {
        key: "level",
        labelKey: "logs.columns.level",
        variant: "select",
        options: [
          {
            value: "debug",
            label: t("logs.levels.debug"),
            labelKey: "logs.levels.debug",
          },
          {
            value: "warn",
            label: t("logs.levels.warn"),
            labelKey: "logs.levels.warn",
          },
          {
            value: "error",
            label: t("logs.levels.error"),
            labelKey: "logs.levels.error",
          },
        ],
      },
      {
        key: "source",
        labelKey: "logs.columns.source",
        variant: "select",
        options: (sourcesQuery.data?.items ?? []).map((source) => ({
          value: source,
          label: source,
        })),
      },
      {
        key: "created_from",
        labelKey: "logs.filters.created_from",
        variant: "date",
      },
      {
        key: "created_to",
        labelKey: "logs.filters.created_to",
        variant: "date",
      },
    ],
    [sourcesQuery.data?.items, t],
  );

  const handleDelete = useCallback(
    async (log: AppLog) => {
      const confirmed = await confirmDelete({
        title: t("logs.delete_title"),
        description: t("logs.delete_description"),
      });
      if (!confirmed) return;
      await deleteLog.mutateAsync(log.uuid);
    },
    [confirmDelete, deleteLog, t],
  );

  const columns = useMemo<ColumnDef<AppLog>[]>(
    () => [
      createColumn<AppLog>({
        accessorKey: "level",
        labelKey: "logs.columns.level",
        filterVariant: "faceted",
        filterOptions: [
          { value: "debug", label: t("logs.levels.debug") },
          { value: "warn", label: t("logs.levels.warn") },
          { value: "error", label: t("logs.levels.error") },
        ],
        cell: ({ row }) => (
          <StatusChip
            label={t(`logs.levels.${row.original.level}`)}
            tone={levelTone(row.original.level)}
          />
        ),
      }),
      createColumn<AppLog>({
        accessorKey: "message",
        labelKey: "logs.columns.message",
        cell: ({ row }) => (
          <span className="line-clamp-2 font-mono text-xs">
            {row.original.message}
          </span>
        ),
      }),
      createColumn<AppLog>({
        accessorKey: "source",
        labelKey: "logs.columns.source",
        cell: ({ row }) => row.original.source || "—",
      }),
      createColumn<AppLog>({
        accessorKey: "created_at",
        labelKey: "logs.columns.created_at",
        cell: ({ row }) =>
          format.dateTime(row.original.created_at, { seconds: true }),
      }),
      createColumn<AppLog>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        cell: ({ row }) => (
          <EntityRowActions
            actions={[
              {
                id: "view",
                label: t("logs.view_detail"),
                icon: Eye,
                onSelect: () => setSelected(row.original),
              },
              ...(can(permissions.logs.write)
                ? [
                    {
                      id: "delete",
                      label: t("common.delete"),
                      icon: Trash2,
                      variant: "destructive" as const,
                      onSelect: () => void handleDelete(row.original),
                    },
                  ]
                : []),
            ]}
          />
        ),
      }),
    ],
    [can, handleDelete, t, format],
  );

  const pageCount = useMemo(() => {
    const total = listQuery.data?.total ?? 0;
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listQuery.data?.total, listState.pagination.pageSize]);

  const activeFilterCount = countActiveFilters(appliedFilters);

  return (
    <div className="space-y-6">
      <StatsCards />

      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={setSelected}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("logs.empty_title")}
        emptyDescription={t("logs.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{ persistKey: "platform-logs-v1", rowSelection: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
            onFiltersOpen={() => {
              setDraftFilters(appliedFilters);
              setFiltersOpen(true);
            }}
            filtersActive={activeFilterCount > 0}
            filtersLabel={
              activeFilterCount > 0
                ? `${t("entity.filters")} (${activeFilterCount})`
                : t("entity.filters")
            }
          />
        }
      />

      <EntityFilters
        open={filtersOpen}
        onOpenChange={setFiltersOpen}
        title={t("logs.filters_title")}
        description={t("logs.filters_description")}
        filters={filterDefs}
        values={draftFilters}
        onChange={(key, value) =>
          setDraftFilters((current) => ({ ...current, [key]: value }))
        }
        onReset={() => setDraftFilters({})}
        onApply={() => setAppliedFilters(draftFilters)}
      />

      <LogDetailDrawer
        log={selected}
        open={Boolean(selected)}
        onOpenChange={(next) => {
          if (!next) setSelected(null);
        }}
      />
    </div>
  );
}

export function LogsPage() {
  const { t } = useLocale();
  const { can } = usePermission();

  return (
    <EntityPage
      title={t("logs.title")}
      description={t("logs.description")}
      permission={permissions.logs.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("logs.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("logs.title") },
      ]}
    >
      <Tabs defaultValue="logs" className="gap-6">
        <TabsList>
          <TabsTrigger value="logs">{t("logs.tab_logs")}</TabsTrigger>
          {can(permissions.logs.read) ? (
            <TabsTrigger value="rules">{t("logs.tab_rules")}</TabsTrigger>
          ) : null}
        </TabsList>
        <TabsContent value="logs" className="mt-2">
          <LogsListPanel />
        </TabsContent>
        {can(permissions.logs.read) ? (
          <TabsContent value="rules" className="mt-2">
            <PurgeRulesPanel />
          </TabsContent>
        ) : null}
      </Tabs>
    </EntityPage>
  );
}
