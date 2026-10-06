"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Archive, Eye, Pin, PinOff, Send } from "lucide-react";
import { useMemo } from "react";
import { toast } from "sonner";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  announcementKeys,
  announcementsService,
  type Announcement,
  type AnnouncementManageQuery,
} from "@/features/announcements/services/announcements.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

export const ANNOUNCEMENTS_MANAGE_PERSIST_KEY =
  "tenant-announcements-manage-v1";

const ANNOUNCEMENT_STATUSES = ["draft", "published", "archived"] as const;

const STATUS_TONE = {
  draft: "warning",
  published: "success",
  archived: "default",
} as const;

/**
 * Author list of the organization's announcements in every status
 * (`GET /v1/announcements/manage`): q on title, CSV status, pinned,
 * publish date range, single-field sort; publish / archive / pin per row.
 */
export function AnnouncementsManageTable({
  onOpen,
}: {
  /** Opens a published announcement in the reader detail. */
  onOpen: (uuid: string) => void;
}) {
  const { t, format } = useLocale();
  const qc = useQueryClient();

  const refresh = () => {
    void qc.invalidateQueries({
      queryKey: [...announcementKeys.all, "manage"],
    });
    void qc.invalidateQueries({ queryKey: announcementKeys.lists() });
  };
  const onError = (err: unknown) =>
    toast.error(isApiError(err) ? err.message : t("common.error_generic"));

  const publish = useMutation({
    mutationFn: (uuid: string) => announcementsService.publish(uuid),
    onSuccess: () => {
      toast.success(t("announcements.manage.published"));
      refresh();
    },
    onError,
  });
  const archive = useMutation({
    mutationFn: (uuid: string) => announcementsService.archive(uuid),
    onSuccess: () => {
      toast.success(t("announcements.manage.archived"));
      refresh();
    },
    onError,
  });
  const pin = useMutation({
    mutationFn: ({ uuid, pinned }: { uuid: string; pinned: boolean }) =>
      announcementsService.pin(uuid, pinned),
    onSuccess: refresh,
    onError,
  });
  const busy = publish.isPending || archive.isPending || pin.isPending;
  const publishOne = publish.mutate;
  const archiveOne = archive.mutate;
  const pinOne = pin.mutate;

  const columns = useMemo<ColumnDef<Announcement, unknown>[]>(
    () => [
      createColumn<Announcement>({
        accessorKey: "title",
        labelKey: "announcements.form.fields.title",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="flex items-center gap-2 font-medium">
            {row.original.pinned ? <Pin className="size-3.5" /> : null}
            {row.original.title}
          </span>
        ),
      }),
      createColumn<Announcement>({
        accessorKey: "status",
        labelKey: "announcements.manage.status",
        enableSorting: true,
        filterVariant: "faceted",
        param: "status",
        gridSecondary: true,
        filterOptions: ANNOUNCEMENT_STATUSES.map((value) => ({
          value,
          labelKey: `announcements.status.${value}`,
          label: value,
        })),
        cell: ({ row }) => (
          <StatusChip
            label={t(`announcements.status.${row.original.status}`)}
            tone={STATUS_TONE[row.original.status] ?? "default"}
          />
        ),
      }),
      createColumn<Announcement>({
        accessorKey: "pinned",
        labelKey: "announcements.form.fields.pinned",
        enableSorting: false,
        filterVariant: "boolean",
        param: "pinned",
        cell: ({ row }) =>
          row.original.pinned ? t("table.true") : t("table.false"),
      }),
      createColumn<Announcement>({
        accessorKey: "publish_at",
        labelKey: "announcements.form.fields.publish_at",
        enableSorting: true,
        filterVariant: "date-range",
        param: "publish",
        cell: ({ row }) =>
          row.original.publish_at
            ? format.dateTime(row.original.publish_at)
            : "—",
      }),
      createColumn<Announcement>({
        accessorKey: "created_at",
        labelKey: "announcements.manage.created_at",
        enableSorting: true,
        cell: ({ row }) =>
          row.original.created_at
            ? format.dateTime(row.original.created_at)
            : "—",
      }),
      createColumn<Announcement>({
        accessorKey: "updated_at",
        labelKey: "announcements.manage.updated_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.updated_at
            ? format.dateTime(row.original.updated_at)
            : "—",
      }),
      createColumn<Announcement>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const a = row.original;
          const actions: EntityRowAction[] = [];
          if (a.status === "published") {
            actions.push({
              id: "view",
              label: t("announcements.manage.view"),
              icon: Eye,
              onSelect: () => onOpen(a.uuid),
            });
          }
          if (a.status === "draft") {
            actions.push({
              id: "publish",
              label: t("announcements.form.publish"),
              icon: Send,
              disabled: busy,
              onSelect: () => publishOne(a.uuid),
            });
          }
          if (a.status !== "archived") {
            actions.push({
              id: "pin",
              label: a.pinned
                ? t("announcements.manage.unpin")
                : t("announcements.manage.pin"),
              icon: a.pinned ? PinOff : Pin,
              disabled: busy,
              onSelect: () => pinOne({ uuid: a.uuid, pinned: !a.pinned }),
            });
            actions.push({
              id: "archive",
              label: t("announcements.manage.archive"),
              icon: Archive,
              variant: "destructive",
              disabled: busy,
              onSelect: () => archiveOne(a.uuid),
            });
          }
          return <EntityRowActions actions={actions} />;
        },
      }),
    ],
    [archiveOne, busy, format, onOpen, pinOne, publishOne, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 20,
    persistKey: ANNOUNCEMENTS_MANAGE_PERSIST_KEY,
  });
  const params = listState.params as AnnouncementManageQuery;

  const list = useQuery({
    queryKey: announcementKeys.manage(params),
    queryFn: () => announcementsService.manage(params),
    placeholderData: (previous) => previous,
  });

  return (
    <EntityTable
      columns={columns}
      data={list.data?.items ?? []}
      getRowId={(row) => row.uuid}
      onRowClick={(row) => {
        if (row.status === "published") onOpen(row.uuid);
      }}
      isLoading={list.isLoading}
      isError={list.isError}
      onRetry={() => void list.refetch()}
      emptyTitle={t("announcements.list.empty")}
      emptyDescription=""
      rowCount={list.data?.total ?? 0}
      state={listState.tableState}
      features={{
        persistKey: ANNOUNCEMENTS_MANAGE_PERSIST_KEY,
        // No bulk endpoint for announcements.
        rowSelection: false,
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
