"use client";

import { useCallback, useMemo } from "react";
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
import {
  ACTIVITY_ACTION_OPTIONS,
  ACTIVITY_RESOURCE_OPTIONS,
} from "@/features/io/lib/filter-options";
import { ioKeys } from "@/features/io/hooks/query-keys";
import {
  activityService,
  type ListActivityParams,
} from "@/features/io/services/activity.service";
import type { ActivityEvent } from "@/features/io/types";
import { UserFilterCombobox } from "@/features/users/components/user-filter-combobox";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const ACTIVITY_PERSIST_KEY = "platform-activity-v1";

export function ActivityPage() {
  const { t, format } = useLocale();
  const { can } = usePermission();

  const metaQuery = useQuery({
    queryKey: ioKeys.activity.meta(),
    queryFn: () => activityService.meta(),
    staleTime: 5 * 60_000,
  });

  // Sortable: created_at, action, resource (backend whitelist).
  const columns = useMemo<ColumnDef<ActivityEvent>[]>(
    () => [
      createColumn<ActivityEvent>({
        accessorKey: "action",
        labelKey: "activity.columns.action",
        enableSorting: true,
        filterVariant: "faceted",
        param: "action",
        filterOptions: ACTIVITY_ACTION_OPTIONS,
        gridPrimary: true,
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
        enableSorting: true,
        filterVariant: "faceted",
        param: "resource",
        filterOptions: ACTIVITY_RESOURCE_OPTIONS,
        gridSecondary: true,
        cell: ({ row }) => formatActivityResource(t, row.original.resource),
      }),
      createColumn<ActivityEvent>({
        // Filter value = user uuid, set from the toolbar user picker.
        id: "actor",
        accessorKey: "actor_user_id",
        labelKey: "activity.columns.actor",
        enableSorting: false,
        enableColumnFilter: false,
        param: "actor",
        paramFormat: "string",
        cell: ({ row }) => row.original.actor_user_id ?? "—",
      }),
      createColumn<ActivityEvent>({
        accessorKey: "created_at",
        labelKey: "activity.columns.created_at",
        enableSorting: true,
        filterVariant: "date-range",
        param: "created",
        cell: ({ row }) => format.dateTime(row.original.created_at),
      }),
    ],
    [t, format],
  );

  const listState = useServerListState({
    columns,
    initialSort: metaQuery.data?.default_sort ?? "-created_at",
    initialPageSize: 20,
    persistKey: ACTIVITY_PERSIST_KEY,
  });
  const listParams: ListActivityParams = listState.params;

  const listQuery = useQuery({
    queryKey: ioKeys.activity.list(listParams),
    queryFn: () => activityService.list(listParams),
    placeholderData: (previous) => previous,
  });

  // The export applies the same filters and sort.
  const exportQuery = useMemo(
    () => ({
      ...listState.filterParams,
      q: listParams.q,
      sort: listParams.sort,
    }),
    [listState.filterParams, listParams.q, listParams.sort],
  );

  const actorUuid =
    (listState.columnFilters.find((filter) => filter.id === "actor")?.value as
      string | undefined) ?? "";
  const { onColumnFiltersChange } = listState;
  const setActor = useCallback(
    (uuid: string) =>
      onColumnFiltersChange((current) => [
        ...current.filter((filter) => filter.id !== "actor"),
        ...(uuid ? [{ id: "actor", value: uuid }] : []),
      ]),
    [onColumnFiltersChange],
  );

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
        rowCount={listQuery.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: ACTIVITY_PERSIST_KEY, rowSelection: false }}
        toolbarExtra={
          <>
            {can(permissions.users.read) ? (
              <UserFilterCombobox
                value={actorUuid}
                onValueChange={setActor}
                placeholder={t("activity.filters.actor")}
                className="h-8 w-56"
              />
            ) : null}
            <ResourceIOToolbar
              resource="platform.activity"
              query={exportQuery}
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
