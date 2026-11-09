"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ArrowRightLeft, Eye, Pencil, Plus, ShoppingCart } from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useMemo, useState } from "react";

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
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { OrderActionDialog } from "@/features/orders/components/order-action-dialog";
import { OrderQueueChips } from "@/features/orders/components/order-queue-chips";
import {
  activeOrderSide,
  canEditDraft,
  orderActions,
  resolveOrderListAccess,
  type OrderAction,
} from "@/features/orders/lib/access";
import { orderActivityLabel } from "@/features/orders/lib/activity";
import {
  amountNumber,
  invoiceStatusTone,
  orderStatusTone,
} from "@/features/orders/lib/form";
import {
  ORDER_FILTER_STATUSES,
  ORDER_INVOICE_STATUSES,
} from "@/features/orders/lib/list-filters";
import {
  ORDERS_EXPORT_PATH,
  orderKeys,
  ordersService,
  type Order,
  type OrderListQuery,
  type OrderSide,
} from "@/features/orders/services/orders.service";
import { ExportMenu } from "@/features/io/components/export-menu";
import {
  warehouseKeys,
  warehouseService,
} from "@/features/warehouse/services/warehouse.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useCanSeePrices } from "@/hooks/use-can-see-prices";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const ORDER_PAGE_SIZE = 20;
export const ORDERS_PERSIST_KEY = "tenant-orders-v1";

/** The counterpart column: the buyer on incoming, the seller on outgoing. */
const PARTY_COLUMN = "party";

/** TEC-1054: pieces, and the meters of roll lines next to them. */
function OrderAmount({ order }: { order: Order }) {
  const { t, format } = useLocale();
  const meters = amountNumber(order.total_meters);
  return (
    <span className="whitespace-nowrap tabular-nums">
      {order.total_quantity > 0 || !order.total_meters
        ? format.number(order.total_quantity)
        : null}
      {order.total_meters ? (
        <span className="text-muted-foreground ms-1 text-xs">
          {t("orders.list.quantity_meters", {
            meters: format.number(meters, { maximumFractionDigits: 2 }),
          })}
        </span>
      ) : null}
    </span>
  );
}

/** TEC-1054 / TEC-1060: who last handled the order, and the step. */
function OrderHandler({ order }: { order: Order }) {
  const { t } = useLocale();
  const last = order.last_activity;
  if (!last?.user) return <span className="text-muted-foreground">—</span>;
  return (
    <span className="flex flex-col">
      <span className="whitespace-nowrap">{last.user.name}</span>
      <span className="text-muted-foreground text-xs">
        {orderActivityLabel(t, last.action)}
      </span>
    </span>
  );
}

/**
 * Tenant > Orders (TEC-170, TEC-374): incoming orders (the organization
 * sells) and outgoing orders (it buys) as tabs, over a server DataTable
 * with sort (order no, status, total, created), search, status / party /
 * total / created filters, row actions (view, edit draft, status moves),
 * export (csv, xlsx, pdf) and mobile cards.
 */
export function OrdersListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  // TEC-1050: no total column, filter or card amount without a price grant.
  const canSeePrices = useCanSeePrices();
  const router = useRouter();
  const org = useActiveOrganization(slug);
  const access = resolveOrderListAccess(can, org?.type);
  const [pickedSide, setSide] = useState<OrderSide | null>(null);
  const side = activeOrderSide(pickedSide, org?.type);
  const [action, setAction] = useState<{
    order: Order;
    action: OrderAction;
  } | null>(null);

  // Center and distributor sell to organizations of their scope: the buyer
  // filter offers them on the incoming tab (TEC-373 buyer_org_uuid).
  const canFilterParty =
    can(permissions.orders.read) &&
    side === "seller" &&
    can(permissions.organizations.tenantRead) &&
    (org?.type === "center" || org?.type === "distributor");
  const organizations = useQuery({
    queryKey: orderKeys.organizations,
    queryFn: () => ordersService.listOrganizations(),
    enabled: canFilterParty,
    staleTime: 5 * 60_000,
  });
  // TEC-1054: the seller warehouse filter lists the own warehouses.
  const canFilterWarehouse =
    side === "seller" && can(permissions.warehouse.read);
  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
    enabled: canFilterWarehouse,
    staleTime: 5 * 60_000,
    retry: false,
  });
  const warehouseOptions = useMemo(
    () =>
      (warehouses.data?.items ?? []).map((w) => ({
        value: w.uuid,
        label: w.name,
      })),
    [warehouses.data?.items],
  );
  const partyOptions = useMemo(
    () =>
      (organizations.data ?? []).map((o) => ({ value: o.uuid, label: o.name })),
    [organizations.data],
  );

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
              data-testid="order-row"
              data-uuid={row.original.uuid}
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.order_no}
            </Link>
          ),
        }),
        createColumn<Order>({
          accessorKey: "status",
          labelKey: "orders.list.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: ORDER_FILTER_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `orders.status.${value}`,
          })),
          param: "status",
          cell: ({ row }) => (
            <StatusChip
              label={row.original.status_label}
              tone={orderStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<Order>({
          id: PARTY_COLUMN,
          accessorFn: (o) => (side === "seller" ? o.buyer.uuid : o.seller.uuid),
          labelKey:
            side === "seller"
              ? "orders.list.columns.buyer"
              : "orders.list.columns.seller",
          enableSorting: false,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: partyOptions,
          enableColumnFilter: canFilterParty && partyOptions.length > 0,
          param: side === "seller" ? "buyer_org_uuid" : "seller_org_uuid",
          cell: ({ row }) =>
            side === "seller"
              ? row.original.buyer.name
              : row.original.seller.name,
        }),
        // TEC-1054: the seller warehouse is the seller's own business.
        ...(side === "seller"
          ? [
              createColumn<Order>({
                id: "seller_warehouse",
                accessorFn: (o) => o.seller_warehouse?.uuid ?? "",
                labelKey: "orders.list.columns.seller_warehouse",
                enableSorting: false,
                filterVariant: "faceted",
                filterOptions: warehouseOptions,
                enableColumnFilter:
                  canFilterWarehouse && warehouseOptions.length > 0,
                param: "seller_warehouse",
                cell: ({ row }) =>
                  row.original.seller_warehouse?.name ?? (
                    <span className="text-muted-foreground">—</span>
                  ),
              }),
            ]
          : []),
        createColumn<Order>({
          id: "total_quantity",
          accessorFn: (o) => o.total_quantity,
          labelKey: "orders.list.columns.quantity",
          enableSorting: false,
          meta: { cellClassName: "text-end", headerClassName: "text-end" },
          cell: ({ row }) => <OrderAmount order={row.original} />,
        }),
        createColumn<Order>({
          id: "handled_by",
          accessorFn: (o) => o.last_activity?.user?.name ?? "",
          labelKey: "orders.list.columns.handled_by",
          enableSorting: false,
          cell: ({ row }) => <OrderHandler order={row.original} />,
        }),
        createColumn<Order>({
          accessorKey: "invoice_status",
          labelKey: "orders.invoice.column",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: ORDER_INVOICE_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `orders.invoice.status.${value}`,
          })),
          param: "invoice_status",
          // TEC-1030: meaningful on the seller's received orders.
          cell: ({ row }) =>
            row.original.status === "received" &&
            row.original.role === "seller" ? (
              <StatusChip
                label={t(
                  `orders.invoice.status.${row.original.invoice_status}`,
                )}
                tone={invoiceStatusTone(row.original.invoice_status)}
              />
            ) : null,
        }),
        ...(canSeePrices
          ? [
              createColumn<Order>({
                id: "total",
                accessorFn: (o) => amountNumber(o.total),
                labelKey: "orders.list.columns.total",
                enableSorting: true,
                filterVariant: "number-range",
                param: "total",
                meta: {
                  cellClassName: "text-end",
                  headerClassName: "text-end",
                },
                cell: ({ row }) => (
                  <span className="flex flex-col items-end whitespace-nowrap tabular-nums">
                    <span>
                      {format.currency(
                        amountNumber(row.original.total),
                        row.original.currency,
                      )}
                    </span>
                    {/* TEC-1054: today's rate in lira; a TRY order needs none. */}
                    {canSeePrices &&
                    row.original.currency !== "TRY" &&
                    row.original.live_total_try ? (
                      <span
                        className="text-muted-foreground text-xs"
                        data-testid="order-live-try"
                      >
                        {t("orders.list.live_try", {
                          amount: format.currency(
                            amountNumber(row.original.live_total_try),
                            "TRY",
                          ),
                        })}
                      </span>
                    ) : null}
                  </span>
                ),
              }),
            ]
          : []),
        createColumn<Order>({
          accessorKey: "created_at",
          labelKey: "orders.list.columns.created_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<Order>({
          accessorKey: "approved_at",
          labelKey: "orders.detail.approved_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.approved_at
              ? format.dateTime(row.original.approved_at)
              : "—",
        }),
        createColumn<Order>({
          accessorKey: "shipped_at",
          labelKey: "orders.detail.shipped_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.shipped_at
              ? format.dateTime(row.original.shipped_at)
              : "—",
        }),
        createColumn<Order>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const order = row.original;
            const items: EntityRowAction[] = [
              {
                id: "view",
                label: t("common.view"),
                icon: Eye,
                onSelect: () =>
                  router.push(routes.tenant.orders.detail(slug, order.uuid)),
              },
            ];
            if (canEditDraft(can, order)) {
              items.push({
                id: "edit",
                label: t("orders.actions.edit"),
                icon: Pencil,
                onSelect: () =>
                  router.push(routes.tenant.orders.edit(slug, order.uuid)),
              });
            }
            // Marking ready needs every line assigned: the detail page
            // shows the lines, so the list leaves that move to it.
            for (const move of orderActions(order)) {
              if (move.kind === "mark_ready") continue;
              items.push({
                id: move.kind,
                label: t(`orders.actions.${move.kind}.button`),
                icon: ArrowRightLeft,
                variant: move.destructive ? "destructive" : "default",
                onSelect: () => setAction({ order, action: move }),
              });
            }
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<Order, unknown>[],
    [
      can,
      canFilterParty,
      canFilterWarehouse,
      canSeePrices,
      format,
      partyOptions,
      warehouseOptions,
      router,
      side,
      slug,
      t,
      setAction,
    ],
  );

  // Column meta drives the params: status / party (CSV), total
  // (total_min/_max), created (created_from/_to).
  // TEC-1024: ?status=a,b (the warehouse preparation queue link) seeds the
  // status filter.
  const statusParam = useSearchParams()?.get("status") ?? "";
  const initialColumnFilters = useMemo(() => {
    const allowed = new Set<string>(ORDER_FILTER_STATUSES);
    const value = statusParam.split(",").filter((v) => allowed.has(v));
    return value.length ? [{ id: "status", value }] : undefined;
  }, [statusParam]);
  const listState = useServerListState({
    columns,
    initialColumnFilters,
    initialSort: "-created_at",
    initialPageSize: ORDER_PAGE_SIZE,
    persistKey: ORDERS_PERSIST_KEY,
  });
  const params: OrderListQuery = useMemo(
    () => ({ ...listState.params, side }),
    [listState.params, side],
  );

  const list = useQuery({
    queryKey: orderKeys.list(params),
    queryFn: () => ordersService.list(params),
    enabled: access.canRead,
  });
  const total = list.data?.total ?? 0;

  // Export uses the same tab, filters, search and sort as the list.
  const exportQuery = useMemo(
    () => ({
      ...listState.filterParams,
      side,
      q: params.q,
      sort: params.sort,
    }),
    [listState.filterParams, params.q, params.sort, side],
  );

  const { onColumnFiltersChange, setPagination } = listState;
  const pickSide = (next: OrderSide) => {
    setSide(next);
    // The party filter means buyer on one tab and seller on the other.
    onColumnFiltersChange((prev) => prev.filter((f) => f.id !== PARTY_COLUMN));
    setPagination((prev) => ({ ...prev, pageIndex: 0 }));
  };

  const title = t("orders.list.title");
  const header = (
    <PageHeader
      title={title}
      icon={<ShoppingCart className="size-6" />}
      description={t("orders.list.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        access.canCreate || access.canCreateForDealer ? (
          <Button asChild>
            <Link
              href={routes.tenant.orders.create(slug)}
              data-testid="new-order"
            >
              <Plus className="size-4" />
              {t("orders.nav_new")}
            </Link>
          </Button>
        ) : null
      }
    />
  );

  if (!access.canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("orders.list.forbidden")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      {access.sides.length > 1 ? (
        <div
          className="flex gap-2"
          role="tablist"
          aria-label={t("orders.list.title")}
        >
          {access.sides.map((s) => (
            <Button
              key={s}
              type="button"
              role="tab"
              aria-selected={side === s}
              variant={side === s ? "default" : "outline"}
              data-testid={`side-${s}`}
              onClick={() => pickSide(s)}
            >
              {s === "seller"
                ? t("orders.list.incoming")
                : t("orders.list.outgoing")}
            </Button>
          ))}
        </div>
      ) : null}
      {side === "seller" ? (
        <OrderQueueChips
          side={side}
          statusFilter={
            (listState.columnFilters.find((f) => f.id === "status")?.value as
              string[] | undefined) ?? []
          }
          onPick={(statuses) => {
            onColumnFiltersChange((prev) => [
              ...prev.filter((f) => f.id !== "status"),
              ...(statuses.length ? [{ id: "status", value: statuses }] : []),
            ]);
            setPagination((prev) => ({ ...prev, pageIndex: 0 }));
          }}
        />
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
        emptyTitle={t("orders.list.empty_title")}
        emptyDescription={
          listState.columnFilters.length > 0 || params.q
            ? t("orders.list.empty_filtered")
            : t("orders.list.empty_description")
        }
        rowCount={total}
        state={listState.tableState}
        features={{
          persistKey: ORDERS_PERSIST_KEY,
          rowSelection: false,
          viewMode: true,
        }}
        renderGridItem={(order) => (
          <div className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <Link
                href={routes.tenant.orders.detail(slug, order.uuid)}
                className="font-mono font-semibold hover:underline"
                dir="ltr"
                onClick={(event) => event.stopPropagation()}
              >
                {order.order_no}
              </Link>
              <StatusChip
                label={order.status_label}
                tone={orderStatusTone(order.status)}
              />
            </div>
            <p className="text-sm">
              {side === "seller" ? order.buyer.name : order.seller.name}
            </p>
            <div className="text-muted-foreground flex justify-between gap-2 text-xs">
              <span>{order.seller_warehouse?.name ?? ""}</span>
              <span>{order.last_activity?.user?.name ?? ""}</span>
            </div>
            <div className="text-muted-foreground flex justify-between gap-2 text-xs">
              <span>{format.dateTime(order.created_at)}</span>
              {canSeePrices && order.total != null ? (
                <span className="text-foreground font-medium tabular-nums">
                  {format.currency(amountNumber(order.total), order.currency)}
                </span>
              ) : null}
            </div>
          </div>
        )}
        toolbarExtra={
          <>
            <ExportMenu
              exportPath={ORDERS_EXPORT_PATH}
              query={exportQuery}
              formats={["xlsx", "csv", "pdf"]}
              jobsHref={routes.tenant.exports.root(slug)}
            />
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
      <OrderActionDialog
        key={action ? `${action.order.uuid}-${action.action.kind}` : "closed"}
        order={action?.order ?? null}
        action={action?.action ?? null}
        onClose={() => setAction(null)}
      />
    </div>
  );
}
