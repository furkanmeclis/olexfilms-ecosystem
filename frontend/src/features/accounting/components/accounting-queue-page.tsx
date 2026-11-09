"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { CheckCheck, FileCheck2, ListChecks, Undo2 } from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { EntityTable, useServerListState } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { Money } from "@/features/accounting/components/shared";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import {
  QUEUE_IN_TRANSIT,
  QUEUE_TO_INVOICE,
} from "@/features/accounting/lib/home-role";
import {
  accountingService,
  type AccountingQueueReturn,
} from "@/features/accounting/services/accounting.service";
import { OrderInvoiceDialog } from "@/features/orders/components/order-invoice-dialog";
import { invoiceStatusTone, orderStatusTone } from "@/features/orders/lib/form";
import {
  orderKeys,
  ordersService,
  type Order,
  type OrderListQuery,
} from "@/features/orders/services/orders.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const QUEUE_TABS = ["orders", "transit", "returns"] as const;
export type QueueTab = (typeof QUEUE_TABS)[number];

export function queueTabOf(raw: string | null | undefined): QueueTab {
  return (QUEUE_TABS as readonly string[]).includes(raw ?? "")
    ? (raw as QueueTab)
    : "orders";
}

/**
 * TEC-1070: accounting queue (legacy olexfilms-warehouse "Muhasebe
 * kuyruğu"). Received sales waiting for an invoice mark (TEC-1030; the
 * mark opens the order's invoice dialog), sales on the road that become
 * invoiceable when the buyer receives them, and received returns waiting
 * to be marked processed.
 */
export function AccountingQueuePage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const router = useRouter();
  const search = useSearchParams();
  const access = useAccountingAccess(slug);
  const tab = queueTabOf(search?.get("tab"));
  const summary = useQuery({
    queryKey: accountingKeys.queueSummary(access.orgUuid),
    queryFn: () => accountingService.queueSummary(),
    enabled: access.canRead && access.orgUuid !== "",
  });
  const counts: Record<QueueTab, number | undefined> = {
    orders: summary.data?.orders_to_invoice,
    transit: summary.data?.orders_in_transit,
    returns: summary.data?.returns_pending,
  };
  const title = t("accounting.queue.title");

  return (
    <div className="space-y-6">
      <PageHeader
        title={title}
        icon={<ListChecks className="size-6" />}
        description={t("accounting.queue.description")}
        breadcrumbs={[
          {
            label: t("layout.breadcrumb_home"),
            href: routes.tenant.home(slug),
          },
          { label: title },
        ]}
      />
      {!access.canRead ? (
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("accounting.forbidden")}
        />
      ) : (
        <>
          <div className="flex flex-wrap gap-2" role="tablist">
            {QUEUE_TABS.map((id) => (
              <Button
                key={id}
                type="button"
                role="tab"
                aria-selected={tab === id}
                variant={tab === id ? "default" : "outline"}
                data-testid={`queue-tab-${id}`}
                onClick={() =>
                  router.replace(
                    `${routes.tenant.accounting.queue(slug)}?tab=${id}`,
                  )
                }
              >
                {t(`accounting.queue.tabs.${id}`)}
                {typeof counts[id] === "number" ? (
                  <span className="tabular-nums opacity-80">
                    ({counts[id]})
                  </span>
                ) : null}
              </Button>
            ))}
          </div>
          {tab === "returns" ? (
            <QueueReturnsTable slug={slug} />
          ) : (
            <QueueOrdersTable key={tab} slug={slug} tab={tab} />
          )}
        </>
      )}
    </div>
  );
}

function QueueOrdersTable({
  slug,
  tab,
}: {
  slug: string;
  tab: "orders" | "transit";
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const access = useAccountingAccess(slug);
  const client = useQueryClient();
  const canInvoice = tab === "orders" && can(Permission.EinvoiceManage);
  const [invoicing, setInvoicing] = useState<string | null>(null);
  const detail = useQuery({
    queryKey: orderKeys.detail(invoicing ?? ""),
    queryFn: () => ordersService.get(invoicing ?? ""),
    enabled: invoicing !== null,
  });

  const columns = useMemo(
    () =>
      [
        createColumn<Order>({
          accessorKey: "order_no",
          labelKey: "orders.list.columns.order_no",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.orders.detail(slug, row.original.uuid)}
              className="font-mono font-medium hover:underline"
              dir="ltr"
              data-testid="queue-order-row"
              onClick={(e) => e.stopPropagation()}
            >
              {row.original.order_no}
            </Link>
          ),
        }),
        createColumn<Order>({
          id: "buyer",
          accessorFn: (o) => o.buyer.name,
          labelKey: "orders.list.columns.buyer",
          enableSorting: false,
          gridSecondary: true,
        }),
        createColumn<Order>({
          accessorKey: "status",
          labelKey: "orders.list.columns.status",
          enableSorting: true,
          cell: ({ row }) => (
            <StatusChip
              label={row.original.status_label}
              tone={orderStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<Order>({
          accessorKey: "invoice_status",
          labelKey: "orders.invoice.column",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(`orders.invoice.status.${row.original.invoice_status}`)}
              tone={invoiceStatusTone(row.original.invoice_status)}
            />
          ),
        }),
        createColumn<Order>({
          id: "moved_at",
          accessorFn: (o) =>
            tab === "orders" ? o.received_at : (o.shipped_at ?? o.created_at),
          labelKey:
            tab === "orders"
              ? "accounting.queue.columns.received_at"
              : "accounting.queue.columns.shipped_at",
          enableSorting: false,
          cell: ({ getValue }) => {
            const v = getValue() as string | null;
            return v ? format.dateTime(v) : "—";
          },
        }),
        createColumn<Order>({
          id: "total",
          accessorKey: "total",
          labelKey: "orders.list.columns.total",
          enableSorting: true,
          meta: { cellClassName: "text-end", headerClassName: "text-end" },
          // TEC-1050: total is null without a price grant.
          cell: ({ row }) =>
            row.original.total != null ? (
              <Money
                amount={row.original.total}
                currency={row.original.currency}
              />
            ) : null,
        }),
        ...(canInvoice
          ? [
              createColumn<Order>({
                id: "actions",
                labelKey: "common.actions",
                enableSorting: false,
                cell: ({ row }) => (
                  <Button
                    size="sm"
                    variant="outline"
                    data-testid="queue-mark-invoiced"
                    onClick={(e) => {
                      e.stopPropagation();
                      setInvoicing(row.original.uuid);
                    }}
                  >
                    <FileCheck2 className="size-3.5" />
                    {t("accounting.queue.mark_invoiced")}
                  </Button>
                ),
              }),
            ]
          : []),
      ] as ColumnDef<Order, unknown>[],
    [canInvoice, format, slug, t, tab],
  );

  const listState = useServerListState({
    columns,
    initialSort: tab === "orders" ? "created_at" : "-created_at",
    initialPageSize: 20,
    persistKey: `tenant-accounting-queue-${tab}-v1`,
  });
  const params: OrderListQuery = useMemo(
    () => ({
      ...listState.params,
      side: "seller",
      ...(tab === "orders" ? QUEUE_TO_INVOICE : QUEUE_IN_TRANSIT),
    }),
    [listState.params, tab],
  );
  const list = useQuery({
    queryKey: orderKeys.list(params),
    queryFn: () => ordersService.list(params),
  });

  return (
    <>
      {tab === "transit" ? (
        <p className="text-muted-foreground text-sm">
          {t("accounting.queue.transit_hint")}
        </p>
      ) : null}
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.orders.detail(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("accounting.queue.orders_empty")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: `tenant-accounting-queue-${tab}-v1`,
          rowSelection: false,
        }}
      />
      {invoicing && detail.data ? (
        <OrderInvoiceDialog
          order={detail.data}
          open
          onClose={() => {
            setInvoicing(null);
            void client.invalidateQueries({ queryKey: orderKeys.all });
            void client.invalidateQueries({
              queryKey: accountingKeys.queueSummary(access.orgUuid),
            });
          }}
        />
      ) : null}
    </>
  );
}

function QueueReturnsTable({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const access = useAccountingAccess(slug);
  const client = useQueryClient();
  const [processed, setProcessed] = useState(false);
  const [marking, setMarking] = useState<AccountingQueueReturn | null>(null);
  const [note, setNote] = useState("");
  const refresh = () =>
    client.invalidateQueries({ queryKey: accountingKeys.all(access.orgUuid) });
  const mark = useMutation({
    mutationFn: (r: AccountingQueueReturn) =>
      accountingService.markReturnProcessed(r.uuid, note.trim() || undefined),
    onSuccess: () => {
      appToast.success(t("accounting.queue.marked"));
      setMarking(null);
      setNote("");
      void refresh();
    },
    onError: () => appToast.error(t("accounting.queue.mark_failed")),
  });
  const unmark = useMutation({
    mutationFn: (r: AccountingQueueReturn) =>
      accountingService.unmarkReturnProcessed(r.uuid),
    onSuccess: () => {
      appToast.success(t("accounting.queue.unmarked"));
      void refresh();
    },
    onError: () => appToast.error(t("accounting.queue.mark_failed")),
  });

  const columns = useMemo(
    () =>
      [
        createColumn<AccountingQueueReturn>({
          accessorKey: "transfer_no",
          labelKey: "accounting.queue.columns.transfer_no",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono font-medium" dir="ltr">
              {row.original.transfer_no}
            </span>
          ),
        }),
        createColumn<AccountingQueueReturn>({
          id: "sender",
          accessorFn: (r) => r.sender.name,
          labelKey: "accounting.queue.columns.sender",
          enableSorting: false,
          gridSecondary: true,
        }),
        // TEC-1085: the order the return refers to.
        createColumn<AccountingQueueReturn>({
          id: "order",
          accessorFn: (r) => r.order?.order_no ?? "",
          labelKey: "orders.list.columns.order_no",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.order ? (
              <Link
                href={routes.tenant.orders.detail(
                  slug,
                  row.original.order.uuid,
                )}
                className="font-mono hover:underline"
                dir="ltr"
                data-testid="queue-return-order"
                onClick={(e) => e.stopPropagation()}
              >
                {row.original.order.order_no}
              </Link>
            ) : (
              "—"
            ),
        }),
        createColumn<AccountingQueueReturn>({
          accessorKey: "item_count",
          labelKey: "accounting.queue.columns.item_count",
          enableSorting: false,
          cell: ({ row }) => format.number(row.original.item_count),
        }),
        createColumn<AccountingQueueReturn>({
          accessorKey: "received_at",
          labelKey: "accounting.queue.columns.received_at",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.received_at
              ? format.dateTime(row.original.received_at)
              : "—",
        }),
        createColumn<AccountingQueueReturn>({
          id: "total",
          accessorFn: (r) => r.total,
          labelKey: "orders.list.columns.total",
          enableSorting: false,
          meta: { cellClassName: "text-end", headerClassName: "text-end" },
          cell: ({ row }) =>
            row.original.total ? (
              <Money
                amount={row.original.total}
                currency={row.original.currency}
              />
            ) : (
              "—"
            ),
        }),
        createColumn<AccountingQueueReturn>({
          accessorKey: "processed_at",
          labelKey: "accounting.queue.columns.processed_at",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.processed_at ? (
              <div className="space-y-0.5">
                <div>{format.dateTime(row.original.processed_at)}</div>
                {row.original.note ? (
                  <div className="text-muted-foreground text-xs">
                    {row.original.note}
                  </div>
                ) : null}
              </div>
            ) : (
              <StatusChip
                label={t("accounting.queue.waiting")}
                tone="warning"
              />
            ),
        }),
        ...(access.canWrite
          ? [
              createColumn<AccountingQueueReturn>({
                id: "actions",
                labelKey: "common.actions",
                enableSorting: false,
                cell: ({ row }) =>
                  row.original.processed_at ? (
                    <Button
                      size="sm"
                      variant="ghost"
                      data-testid="queue-unmark-return"
                      disabled={unmark.isPending}
                      onClick={() => unmark.mutate(row.original)}
                    >
                      <Undo2 className="size-3.5" />
                      {t("accounting.queue.unmark")}
                    </Button>
                  ) : (
                    <Button
                      size="sm"
                      variant="outline"
                      data-testid="queue-mark-return"
                      onClick={() => setMarking(row.original)}
                    >
                      <CheckCheck className="size-3.5" />
                      {t("accounting.queue.mark_processed")}
                    </Button>
                  ),
              }),
            ]
          : []),
      ] as ColumnDef<AccountingQueueReturn, unknown>[],
    [access.canWrite, format, slug, t, unmark],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-received_at",
    initialPageSize: 20,
    persistKey: "tenant-accounting-queue-returns-v1",
  });
  const params = useMemo(
    () => ({ ...listState.params, processed }),
    [listState.params, processed],
  );
  const list = useQuery({
    queryKey: accountingKeys.queueReturns(access.orgUuid, params),
    queryFn: () => accountingService.listQueueReturns(params),
    enabled: access.orgUuid !== "",
  });

  return (
    <>
      <div className="flex gap-2">
        {[false, true].map((value) => (
          <Button
            key={String(value)}
            type="button"
            size="sm"
            variant={processed === value ? "secondary" : "ghost"}
            aria-pressed={processed === value}
            data-testid={`queue-returns-${value ? "processed" : "waiting"}`}
            onClick={() => setProcessed(value)}
          >
            {value
              ? t("accounting.queue.filter_processed")
              : t("accounting.queue.filter_waiting")}
          </Button>
        ))}
      </div>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("accounting.queue.returns_empty")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: "tenant-accounting-queue-returns-v1",
          rowSelection: false,
        }}
      />
      <Dialog
        open={marking !== null}
        onOpenChange={(open) => {
          if (!open) setMarking(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("accounting.queue.mark_processed")}</DialogTitle>
            <DialogDescription>
              {marking
                ? `${marking.transfer_no} · ${marking.sender.name}`
                : null}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="queue-note">{t("accounting.queue.note")}</Label>
            <Textarea
              id="queue-note"
              value={note}
              maxLength={1000}
              placeholder={t("accounting.queue.note_placeholder")}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setMarking(null)}>
              {t("common.cancel")}
            </Button>
            <Button
              data-testid="queue-mark-confirm"
              disabled={mark.isPending}
              onClick={() => marking && mark.mutate(marking)}
            >
              {t("accounting.queue.mark_processed")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
