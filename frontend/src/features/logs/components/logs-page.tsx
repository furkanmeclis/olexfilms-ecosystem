"use client";

import { Eye, Trash2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
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

export const LOGS_PERSIST_KEY = "platform-logs-v1";

const LOG_LEVELS = ["debug", "warn", "error"] as const;

function LogsListPanel() {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const { confirmDelete } = useDialogs();
  const deleteLog = useDeleteLog();
  const [selected, setSelected] = useState<AppLog | null>(null);

  const metaQuery = useQuery({
    queryKey: logsKeys.meta(),
    queryFn: () => logsService.meta(),
    staleTime: 5 * 60_000,
  });
  const sourcesQuery = useQuery({
    queryKey: logsKeys.sources(),
    queryFn: () => logsService.sources(),
    staleTime: 60_000,
  });
  const sourceOptions = useMemo(
    () =>
      (sourcesQuery.data?.items ?? []).map((source) => ({
        value: source,
        label: source,
      })),
    [sourcesQuery.data?.items],
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

  // Sortable: created_at, level, source (backend whitelist); message is not.
  const columns = useMemo<ColumnDef<AppLog>[]>(
    () => [
      createColumn<AppLog>({
        accessorKey: "level",
        labelKey: "logs.columns.level",
        enableSorting: true,
        filterVariant: "faceted",
        param: "level",
        filterOptions: LOG_LEVELS.map((value) => ({
          value,
          labelKey: `logs.levels.${value}`,
          label: value,
        })),
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
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="line-clamp-2 font-mono text-xs">
            {row.original.message}
          </span>
        ),
      }),
      createColumn<AppLog>({
        accessorKey: "source",
        labelKey: "logs.columns.source",
        enableSorting: true,
        // Single value (`source=`), options from /logs/sources.
        filterVariant: "select",
        param: "source",
        filterOptions: sourceOptions,
        enableColumnFilter: sourceOptions.length > 0,
        gridSecondary: true,
        cell: ({ row }) => row.original.source || "—",
      }),
      createColumn<AppLog>({
        accessorKey: "created_at",
        labelKey: "logs.columns.created_at",
        enableSorting: true,
        filterVariant: "date-range",
        param: "created",
        cell: ({ row }) =>
          format.dateTime(row.original.created_at, { seconds: true }),
      }),
      createColumn<AppLog>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
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
    [can, handleDelete, sourceOptions, t, format],
  );

  const listState = useServerListState({
    columns,
    initialSort: metaQuery.data?.default_sort ?? "-created_at",
    initialPageSize: 20,
    persistKey: LOGS_PERSIST_KEY,
  });
  const listParams: ListLogsParams = listState.params;

  const listQuery = useQuery({
    queryKey: logsKeys.list(listParams),
    queryFn: () => logsService.list(listParams),
    placeholderData: (previous) => previous,
  });

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
        rowCount={listQuery.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: LOGS_PERSIST_KEY, rowSelection: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
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
