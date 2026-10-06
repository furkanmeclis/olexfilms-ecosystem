"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ArrowLeftRight, ArrowRightLeft, Eye, Plus, Undo2 } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ReasonDialog } from "@/features/transfers/components/transfer-reason-dialog";
import {
  TRANSFER_DIRECTIONS,
  TRANSFER_KINDS,
  TRANSFER_STATUSES,
  transferErrorMessage,
  transferStatusTone,
  transitionAsksReason,
  transitionButtons,
  transitionIsDestructive,
} from "@/features/transfers/lib/transfers";
import {
  transferKeys,
  transfersService,
  type StockTransfer,
  type StockTransferStatus,
  type TransferDirection,
  type TransferListQuery,
} from "@/features/transfers/services/transfers.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const TRANSFER_PAGE_SIZE = 20;
export const TRANSFERS_PERSIST_KEY = "tenant-stock-transfers-v1";

const ALL = "all";

/**
 * Tenant > Transfers (TEC-197, TEC-374): stock transfer requests of the
 * active organization as the giver (outgoing), the receiver (incoming) or
 * the common parent (approval) as tabs, over a server DataTable with sort
 * (no, status, created), search (no, sender, receiver), kind / status /
 * organization / created filters, row actions (view and the moves the
 * server allows) and mobile cards.
 */
export function TransfersListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const qc = useQueryClient();
  const canRead =
    can(Permission.TransfersRequest) || can(Permission.TransfersApprove);
  const canCreate = can(Permission.TransfersRequest);
  const canListOrgs = canRead && can(Permission.OrganizationsRead);
  const [direction, setDirection] = useState<TransferDirection | typeof ALL>(
    ALL,
  );
  const [reasonFor, setReasonFor] = useState<{
    transfer: StockTransfer;
    status: StockTransferStatus;
  } | null>(null);

  // Organization filter options: the scope's organizations (a parent's
  // children) and the siblings a requester may send to.
  const organizations = useQuery({
    queryKey: transferKeys.organizations,
    queryFn: () => transfersService.listOrganizations(),
    enabled: canListOrgs,
    staleTime: 5 * 60_000,
  });
  const siblings = useQuery({
    queryKey: transferKeys.targets,
    queryFn: () => transfersService.targets(),
    enabled: canCreate,
    staleTime: 5 * 60_000,
  });
  const orgOptions = useMemo(() => {
    const seen = new Map<string, string>();
    for (const o of [
      ...(organizations.data ?? []),
      ...(siblings.data?.items ?? []),
    ]) {
      if (!seen.has(o.uuid)) seen.set(o.uuid, o.name);
    }
    return [...seen].map(([value, label]) => ({ value, label }));
  }, [organizations.data, siblings.data]);

  const move = useMutation({
    mutationFn: (v: {
      transfer: StockTransfer;
      status: StockTransferStatus;
      reason?: string;
    }) => transfersService.transition(v.transfer.uuid, v.status, v.reason),
    onSuccess: (updated: StockTransfer) => {
      qc.setQueryData(transferKeys.detail(updated.uuid), updated);
      void qc.invalidateQueries({ queryKey: transferKeys.all });
      toast.success(t(`transfers.action_done.${updated.status}`));
      setReasonFor(null);
    },
    onError: (err) => {
      toast.error(transferErrorMessage(err, t, t("transfers.detail.error")));
    },
  });
  const runMove = move.mutate;
  const movePending = move.isPending;

  const columns = useMemo(
    () =>
      [
        createColumn<StockTransfer>({
          accessorKey: "transfer_no",
          labelKey: "transfers.columns.no",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <div>
              <Link
                href={routes.tenant.transfers.detail(slug, row.original.uuid)}
                className="font-mono text-xs font-medium hover:underline"
                dir="ltr"
                data-testid="transfer-row"
                data-uuid={row.original.uuid}
                onClick={(event) => event.stopPropagation()}
              >
                {row.original.transfer_no}
              </Link>
              <div className="text-muted-foreground text-xs">
                {t(`transfers.role.${row.original.role}`)}
              </div>
            </div>
          ),
        }),
        createColumn<StockTransfer>({
          accessorKey: "kind",
          labelKey: "transfers.kind.label",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: TRANSFER_KINDS.map((value) => ({
            value,
            label: value,
            labelKey: `transfers.kind.${value}`,
          })),
          param: "kind",
          cell: ({ row }) => t(`transfers.kind.${row.original.kind}`),
        }),
        createColumn<StockTransfer>({
          accessorKey: "status",
          labelKey: "transfers.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: TRANSFER_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `transfers.status.${value}`,
          })),
          param: "status",
          cell: ({ row }) => (
            <StatusChip
              label={t(`transfers.status.${row.original.status}`)}
              tone={transferStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<StockTransfer>({
          id: "sender",
          accessorFn: (r) => r.sender.name,
          labelKey: "transfers.columns.sender",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => row.original.sender.name,
        }),
        createColumn<StockTransfer>({
          id: "receiver",
          accessorFn: (r) => r.receiver.name,
          labelKey: "transfers.columns.receiver",
          enableSorting: false,
          cell: ({ row }) => row.original.receiver.name,
        }),
        // Filter only: sender or receiver is one of the organizations
        // (TEC-373 organization_uuid); the parties show in their columns.
        createColumn<StockTransfer>({
          id: "organization",
          accessorFn: () => "",
          labelKey: "transfers.columns.organization",
          enableSorting: false,
          enableHiding: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: orgOptions,
          enableColumnFilter: orgOptions.length > 0,
          param: "organization_uuid",
          cell: () => null,
        }),
        createColumn<StockTransfer>({
          accessorKey: "item_count",
          labelKey: "transfers.columns.units",
          enableSorting: false,
          cell: ({ row }) => format.number(row.original.item_count),
        }),
        createColumn<StockTransfer>({
          accessorKey: "total",
          labelKey: "transfers.detail.total",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.total ? (
              <span dir="ltr" className="whitespace-nowrap">
                {row.original.total} {row.original.currency}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<StockTransfer>({
          accessorKey: "created_at",
          labelKey: "transfers.columns.created",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<StockTransfer>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const transfer = row.original;
            const items: EntityRowAction[] = [
              {
                id: "view",
                label: t("common.view"),
                icon: Eye,
                onSelect: () =>
                  router.push(
                    routes.tenant.transfers.detail(slug, transfer.uuid),
                  ),
              },
            ];
            for (const status of transitionButtons(transfer)) {
              items.push({
                id: status,
                label: t(`transfers.action.${status}`),
                icon: ArrowRightLeft,
                variant: transitionIsDestructive(status)
                  ? "destructive"
                  : "default",
                disabled: movePending,
                onSelect: () =>
                  transitionAsksReason(status)
                    ? setReasonFor({ transfer, status })
                    : runMove({ transfer, status }),
              });
            }
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<StockTransfer, unknown>[],
    [format, movePending, orgOptions, router, runMove, slug, t],
  );

  // Column meta drives the params: kind / status / organization (CSV),
  // created (created_from/_to).
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: TRANSFER_PAGE_SIZE,
    persistKey: TRANSFERS_PERSIST_KEY,
  });
  const params: TransferListQuery = useMemo(
    () => ({
      ...listState.params,
      ...(direction === ALL ? {} : { direction }),
    }),
    [direction, listState.params],
  );

  const list = useQuery({
    queryKey: transferKeys.list(params),
    queryFn: () => transfersService.list(params),
    enabled: canRead,
  });
  const total = list.data?.total ?? 0;

  const { setPagination } = listState;
  const pickDirection = (next: TransferDirection | typeof ALL) => {
    setDirection(next);
    setPagination((prev) => ({ ...prev, pageIndex: 0 }));
  };

  const title = t("transfers.list.title");
  const header = (
    <PageHeader
      title={title}
      icon={<ArrowLeftRight className="size-6" />}
      description={t("transfers.list.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        canCreate ? (
          <div className="flex flex-wrap gap-2">
            <Button asChild variant="outline">
              <Link
                href={routes.tenant.transfers.createReturn(slug)}
                data-testid="transfer-new-return"
              >
                <Undo2 className="size-4" />
                {t("transfers.return.new")}
              </Link>
            </Button>
            <Button asChild>
              <Link
                href={routes.tenant.transfers.create(slug)}
                data-testid="transfer-new"
              >
                <Plus className="size-4" />
                {t("transfers.list.new")}
              </Link>
            </Button>
          </div>
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
          description={t("transfers.list.forbidden")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <div
        className="flex flex-wrap gap-2"
        role="tablist"
        aria-label={t("transfers.list.direction")}
      >
        {[ALL, ...TRANSFER_DIRECTIONS].map((d) => {
          const active = direction === d;
          return (
            <Button
              key={d}
              type="button"
              role="tab"
              size="sm"
              variant={active ? "default" : "outline"}
              aria-selected={active}
              data-direction={d}
              onClick={() => pickDirection(d as TransferDirection | typeof ALL)}
            >
              {t(`transfers.direction.${d}`)}
            </Button>
          );
        })}
      </div>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.transfers.detail(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("transfers.list.empty_title")}
        emptyDescription={t("transfers.list.empty_description")}
        rowCount={total}
        state={listState.tableState}
        features={{
          persistKey: TRANSFERS_PERSIST_KEY,
          rowSelection: false,
          viewMode: true,
        }}
        renderGridItem={(r) => (
          <div className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <Link
                href={routes.tenant.transfers.detail(slug, r.uuid)}
                className="font-mono text-sm font-semibold hover:underline"
                dir="ltr"
                onClick={(event) => event.stopPropagation()}
              >
                {r.transfer_no}
              </Link>
              <StatusChip
                label={t(`transfers.status.${r.status}`)}
                tone={transferStatusTone(r.status)}
              />
            </div>
            <p className="text-sm">
              {r.sender.name} → {r.receiver.name}
            </p>
            <div className="text-muted-foreground flex justify-between gap-2 text-xs">
              <span>
                {t(`transfers.kind.${r.kind}`)} ·{" "}
                {t(`transfers.role.${r.role}`)}
              </span>
              <span>{format.dateTime(r.created_at)}</span>
            </div>
          </div>
        )}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />
      <ReasonDialog
        key={reasonFor ? `${reasonFor.transfer.uuid}-${reasonFor.status}` : "x"}
        target={reasonFor?.status ?? null}
        pending={movePending}
        onClose={() => setReasonFor(null)}
        onConfirm={(reason) =>
          reasonFor &&
          runMove({
            transfer: reasonFor.transfer,
            status: reasonFor.status,
            reason: reason || undefined,
          })
        }
      />
    </div>
  );
}
