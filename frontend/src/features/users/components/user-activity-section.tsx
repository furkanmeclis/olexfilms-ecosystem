"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import {
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { ioKeys } from "@/features/io/hooks/query-keys";
import { formatActivityResource } from "@/features/io/lib/display";
import {
  ACTIVITY_ACTION_OPTIONS,
  ACTIVITY_RESOURCE_OPTIONS,
} from "@/features/io/lib/filter-options";
import {
  activityService,
  type ListActivityParams,
} from "@/features/io/services/activity.service";
import type { ActivityEvent } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";

export const USER_ACTIVITY_PERSIST_KEY = "platform-user-activity-v1";

/**
 * TEC-1218: what this user did — GET /v1/platform/activity filtered by
 * `actor` (user uuid) on the server. The caller gates it with
 * platform.activity.read (same as the activity page).
 */
export function UserActivitySection({ uuid }: { uuid: string }) {
  const { t, format } = useLocale();

  // Sortable: created_at, action, resource (backend whitelist).
  const columns = useMemo<ColumnDef<ActivityEvent>[]>(
    () => [
      createColumn<ActivityEvent>({
        accessorKey: "created_at",
        labelKey: "activity.columns.created_at",
        enableSorting: true,
        filterVariant: "date-range",
        param: "created",
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="whitespace-nowrap">
            {format.dateTime(row.original.created_at)}
          </span>
        ),
      }),
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
        cell: ({ row }) => {
          const { resource, resource_uuid: id } = row.original;
          return (
            <span className="inline-flex flex-wrap items-center gap-1.5">
              {formatActivityResource(t, resource)}
              {id ? (
                <code
                  className="text-muted-foreground text-xs"
                  title={id}
                  dir="ltr"
                >
                  #{id.split("-")[0]}
                </code>
              ) : null}
            </span>
          );
        },
      }),
    ],
    [t, format],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 10,
    persistKey: USER_ACTIVITY_PERSIST_KEY,
  });
  const params: ListActivityParams = useMemo(
    () => ({ ...listState.params, actor: uuid }),
    [listState.params, uuid],
  );

  const list = useQuery({
    queryKey: ioKeys.activity.list(params),
    queryFn: () => activityService.list(params),
    placeholderData: (previous) => previous,
    enabled: Boolean(uuid),
  });

  return (
    <EntitySectionCard title={t("users.detail.activity")}>
      <div data-testid="user-activity">
        <EntityTable
          columns={columns}
          data={list.data?.items ?? []}
          getRowId={(row) => row.uuid}
          isLoading={list.isLoading}
          isError={list.isError}
          onRetry={() => void list.refetch()}
          emptyTitle={t("users.detail.activity_empty_title")}
          emptyDescription={t("users.detail.activity_empty_description")}
          rowCount={list.data?.total ?? 0}
          state={listState.tableState}
          features={{
            persistKey: USER_ACTIVITY_PERSIST_KEY,
            rowSelection: false,
          }}
          toolbarExtra={
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          }
        />
      </div>
    </EntitySectionCard>
  );
}
