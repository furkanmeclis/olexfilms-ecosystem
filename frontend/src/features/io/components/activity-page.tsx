"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ResourceIOToolbar } from "@/features/io/components/resource-io-toolbar";
import { formatActivityResource } from "@/features/io/lib/display";
import { ioKeys } from "@/features/io/hooks/query-keys";
import {
  activityService,
  type ListActivityParams,
} from "@/features/io/services/activity.service";
import type { ActivityEvent } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";

export function ActivityPage() {
  const { t, format } = useLocale();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });

  const listParams = useMemo<ListActivityParams>(
    () => ({
      ...listState.params,
      q: listState.params.q,
    }),
    [listState.params],
  );

  const metaQuery = useQuery({
    queryKey: ioKeys.activity.meta(),
    queryFn: () => activityService.meta(),
    staleTime: 5 * 60_000,
  });

  const listQuery = useQuery({
    queryKey: ioKeys.activity.list(listParams),
    queryFn: () => activityService.list(listParams),
  });

  const columns = useMemo<ColumnDef<ActivityEvent>[]>(
    () => [
      createColumn<ActivityEvent>({
        accessorKey: "action",
        labelKey: "activity.columns.action",
        cell: ({ row }) => {
          const key = `activity.actions.${row.original.action}`;
          const label = t(key);
          const text = label === key ? row.original.action : label;
          if (row.original.payload?.via !== "ai") return text;
          return (
            <span className="inline-flex items-center gap-1.5">
              {text}
              <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
                {t("activity.via_ai")}
              </Badge>
            </span>
          );
        },
      }),
      createColumn<ActivityEvent>({
        accessorKey: "resource",
        labelKey: "activity.columns.resource",
        cell: ({ row }) => formatActivityResource(t, row.original.resource),
      }),
      createColumn<ActivityEvent>({
        accessorKey: "actor_user_id",
        labelKey: "activity.columns.actor",
        cell: ({ row }) => row.original.actor_user_id ?? "—",
      }),
      createColumn<ActivityEvent>({
        accessorKey: "created_at",
        labelKey: "activity.columns.created_at",
        cell: ({ row }) => format.dateTime(row.original.created_at),
      }),
    ],
    [t, format],
  );

  const pageCount = useMemo(() => {
    const total = listQuery.data?.total ?? 0;
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listQuery.data?.total, listState.pagination.pageSize]);

  return (
    <EntityPage
      title={t("activity.title")}
      description={t("activity.description")}
      permission={permissions.activity.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("activity.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("activity.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("activity.empty_title")}
        emptyDescription={t("activity.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{ persistKey: "platform-activity-v1", rowSelection: false }}
        toolbarExtra={
          <>
            <ResourceIOToolbar
              resource="platform.activity"
              query={{ q: listParams.q }}
              capabilities={metaQuery.data?.capabilities}
            />
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
    </EntityPage>
  );
}
