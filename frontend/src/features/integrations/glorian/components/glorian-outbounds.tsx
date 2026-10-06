"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { RotateCcw } from "lucide-react";
import { useMemo, useState } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { OutboundStateBadge } from "@/features/integrations/glorian/components/glorian-status";
import { glorianErrorText } from "@/features/integrations/glorian/lib/errors";
import { glorianKeys } from "@/features/integrations/glorian/lib/keys";
import {
  glorianService,
  type GlorianOutbound,
  type GlorianOutboundFilter,
  type GlorianOutboundState,
} from "@/features/integrations/glorian/services/glorian.service";
import { useFormatter, useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const OUTBOUND_STATES: GlorianOutboundState[] = [
  "held",
  "failed",
  "pending",
  "sent",
  "cancelled",
];

/** Only held and failed outbounds can be replayed (TEC-273). */
export function isReplayable(o: Pick<GlorianOutbound, "state">) {
  return o.state === "held" || o.state === "failed";
}

export const OUTBOUNDS_PERSIST_KEY = "platform-glorian-outbounds-v1";

/** The list opened on held outbounds before the state facet existed. */
const DEFAULT_STATE_FILTER = [{ id: "state", value: ["held"] }];

/**
 * Order outbounds (server list: CSV state, q on order no / reference,
 * updated range, sort, paging) with single and bulk replay.
 */
export function GlorianOutbounds({ canManage }: { canManage: boolean }) {
  const { t } = useLocale();
  const format = useFormatter();
  const queryClient = useQueryClient();
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const selectedIds = useMemo(
    () => Object.keys(rowSelection).filter((id) => rowSelection[id]),
    [rowSelection],
  );

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: [...glorianKeys.all, "outbounds"],
    });

  const replay = useMutation({
    mutationFn: (uuid: string) => glorianService.replayOutbound(uuid),
    onSuccess: async (o) => {
      appToast.success(
        t("integrations.glorian.outbounds.replay_queued", {
          order: o.order_no,
        }),
      );
      await invalidate();
    },
    onError: (error) => appToast.error(glorianErrorText(t, error)),
  });
  const replayOne = replay.mutate;
  const replayPending = replay.isPending;

  const replayBatch = useMutation({
    mutationFn: (uuids: string[]) => glorianService.replayOutbounds(uuids),
    onSuccess: async (res) => {
      setRowSelection({});
      appToast.success(
        t("integrations.glorian.outbounds.replay_batch_done", {
          queued: res.queued.length,
          skipped: res.skipped.length,
        }),
      );
      await invalidate();
    },
    onError: (error) => appToast.error(glorianErrorText(t, error)),
  });

  const columns = useMemo<ColumnDef<GlorianOutbound, unknown>[]>(
    () => [
      ...(canManage ? [createSelectColumnDef<GlorianOutbound>()] : []),
      createColumn<GlorianOutbound>({
        accessorKey: "order_no",
        labelKey: "integrations.glorian.outbounds.order",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-mono">{row.original.order_no}</span>
        ),
      }),
      createColumn<GlorianOutbound>({
        accessorKey: "external_reference",
        labelKey: "integrations.glorian.outbounds.reference",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="font-mono">{row.original.external_reference}</span>
        ),
      }),
      createColumn<GlorianOutbound>({
        accessorKey: "state",
        labelKey: "integrations.glorian.outbounds.state",
        enableSorting: true,
        filterVariant: "faceted",
        param: "state",
        gridSecondary: true,
        filterOptions: OUTBOUND_STATES.map((value) => ({
          value,
          labelKey: `integrations.glorian.outbound_state.${value}`,
          label: value,
        })),
        cell: ({ row }) => <OutboundStateBadge state={row.original.state} />,
      }),
      createColumn<GlorianOutbound>({
        accessorKey: "attempts",
        labelKey: "integrations.glorian.outbounds.attempts",
        enableSorting: true,
      }),
      createColumn<GlorianOutbound>({
        accessorKey: "updated_at",
        labelKey: "integrations.glorian.outbounds.updated",
        enableSorting: true,
        filterVariant: "date-range",
        param: "updated",
        cell: ({ row }) => (
          <span className="whitespace-nowrap">
            {format.dateTime(row.original.updated_at)}
          </span>
        ),
      }),
      createColumn<GlorianOutbound>({
        accessorKey: "created_at",
        labelKey: "integrations.glorian.outbounds.created",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="whitespace-nowrap">
            {format.dateTime(row.original.created_at)}
          </span>
        ),
      }),
      createColumn<GlorianOutbound>({
        id: "reason",
        labelKey: "integrations.glorian.outbounds.reason",
        enableSorting: false,
        cell: ({ row }) => {
          const o = row.original;
          return (
            <>
              {o.held_reason ? (
                <div>
                  {t(`integrations.glorian.held_reason.${o.held_reason}`)}
                </div>
              ) : null}
              {o.last_error ? (
                <div className="text-destructive break-all">{o.last_error}</div>
              ) : null}
            </>
          );
        },
      }),
      createColumn<GlorianOutbound>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const o = row.original;
          return (
            <div className="text-end" data-testid={`outbound-${o.uuid}`}>
              {canManage && isReplayable(o) ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => replayOne(o.uuid)}
                  disabled={replayPending}
                  data-testid={`replay-${o.uuid}`}
                >
                  <RotateCcw className="size-4" aria-hidden />
                  {t("integrations.glorian.outbounds.replay")}
                </Button>
              ) : null}
            </div>
          );
        },
      }),
    ],
    [canManage, format, replayOne, replayPending, t],
  );

  const listState = useServerListState({
    columns,
    // Backend default: created_at ascending (the replay order).
    initialSort: null,
    initialPageSize: 20,
    persistKey: OUTBOUNDS_PERSIST_KEY,
    initialColumnFilters: DEFAULT_STATE_FILTER,
  });
  const query = listState.params as GlorianOutboundFilter;

  const list = useQuery({
    queryKey: glorianKeys.outbounds(query),
    queryFn: () => glorianService.outboundsPage(query),
    placeholderData: (previous) => previous,
  });

  return (
    <div data-testid="outbounds">
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={glorianErrorText(t, list.error)}
        onRetry={() => void list.refetch()}
        emptyTitle={t("integrations.glorian.outbounds.empty")}
        emptyDescription=""
        rowCount={list.data?.total ?? 0}
        state={{
          ...listState.tableState,
          rowSelection,
          onRowSelectionChange: setRowSelection,
        }}
        features={{
          persistKey: OUTBOUNDS_PERSIST_KEY,
          rowSelection: canManage,
        }}
        toolbarExtra={
          <>
            {canManage && selectedIds.length ? (
              <Button
                type="button"
                size="sm"
                onClick={() => replayBatch.mutate(selectedIds.slice(0, 100))}
                disabled={replayBatch.isPending}
                data-testid="replay-selected"
              >
                <RotateCcw className="size-4" aria-hidden />
                {t("integrations.glorian.outbounds.replay_selected", {
                  count: selectedIds.length,
                })}
              </Button>
            ) : null}
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
    </div>
  );
}
