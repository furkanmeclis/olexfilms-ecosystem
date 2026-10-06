"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  Building2,
  History,
  Package,
  Pencil,
  ScanBarcode,
  ShoppingCart,
  X,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState, type ReactNode } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  OrderActionDialog,
  useStoreOrder,
} from "@/features/orders/components/order-action-dialog";
import {
  canAssignUnits,
  canEditDraft,
  lineAmount,
  lineFullyAssigned,
  orderActions,
  orderFullyAssigned,
  type OrderAction,
} from "@/features/orders/lib/access";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import {
  amountNumber,
  isRoll,
  orderStatusTone,
} from "@/features/orders/lib/form";
import {
  orderKeys,
  ordersService,
  type Order,
  type OrderItem,
} from "@/features/orders/services/orders.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function Section({
  title,
  icon,
  testId,
  children,
}: {
  title: string;
  icon: ReactNode;
  testId: string;
  children: ReactNode;
}) {
  return (
    <Card data-testid={testId}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="text-muted-foreground">{icon}</span>
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm break-words">{children}</dd>
    </div>
  );
}

function newSplitKey(): string {
  return typeof crypto !== "undefined" && "randomUUID" in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

/** Barcode scan (or typed) assignment of a unit to a line (seller). */
function AssignForm({ order, item }: { order: Order; item: OrderItem }) {
  const { t } = useLocale();
  const store = useStoreOrder();
  const [barcode, setBarcode] = useState("");
  const [quantity, setQuantity] = useState("");
  const [meters, setMeters] = useState("");
  // One key per typed assignment: a retry of the same roll cut (TEC-184)
  // returns the earlier split instead of cutting the roll again.
  const [splitKey, setSplitKey] = useState(newSplitKey);
  const roll = isRoll(item.product.unit_type);
  const mutation = useMutation({
    mutationFn: () => {
      const q = Number(quantity);
      const m = meters.trim().replace(",", ".");
      return ordersService.assignUnit(order.uuid, item.uuid, {
        barcode: barcode.trim(),
        ...(!roll && quantity.trim() && Number.isInteger(q) && q > 0
          ? { quantity: q }
          : {}),
        ...(roll && m ? { meters: m, idempotency_key: splitKey } : {}),
      });
    },
    onSuccess: (updated) => {
      store(updated);
      setBarcode("");
      setQuantity("");
      setMeters("");
      setSplitKey(newSplitKey());
      if (updated.split) {
        toast.success(
          t("orders.assign.split_success", {
            barcode: updated.split.new_barcode,
            rest: updated.split.source_remaining_meters,
          }),
        );
      } else {
        toast.success(t("orders.assign.success"));
      }
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.assign.failed"))),
  });
  const bid = `barcode-${item.uuid}`;
  const qid = `qty-${item.uuid}`;
  const mid = `meters-${item.uuid}`;
  return (
    <form
      className="flex flex-wrap items-end gap-2"
      data-testid="assign-form"
      onSubmit={(e) => {
        e.preventDefault();
        if (barcode.trim()) mutation.mutate();
      }}
    >
      <div className="min-w-48 flex-1 space-y-1">
        <Label htmlFor={bid} className="text-xs">
          {t("orders.assign.barcode")}
        </Label>
        <Input
          id={bid}
          name="barcode"
          dir="ltr"
          autoComplete="off"
          value={barcode}
          maxLength={64}
          placeholder={t("orders.assign.barcode_placeholder")}
          onChange={(e) => {
            setBarcode(e.target.value);
            setSplitKey(newSplitKey());
          }}
        />
      </div>
      {roll ? (
        <div className="w-28 space-y-1">
          <Label htmlFor={mid} className="text-xs">
            {t("orders.assign.meters")}
          </Label>
          <Input
            id={mid}
            name="meters"
            inputMode="decimal"
            dir="ltr"
            value={meters}
            placeholder={t("orders.assign.meters_placeholder")}
            onChange={(e) => {
              setMeters(e.target.value);
              setSplitKey(newSplitKey());
            }}
          />
        </div>
      ) : null}
      {!roll ? (
        <div className="w-28 space-y-1">
          <Label htmlFor={qid} className="text-xs">
            {t("orders.assign.quantity")}
          </Label>
          <Input
            id={qid}
            name="quantity"
            inputMode="numeric"
            dir="ltr"
            value={quantity}
            placeholder="1"
            onChange={(e) => setQuantity(e.target.value)}
          />
        </div>
      ) : null}
      <Button
        type="submit"
        size="sm"
        disabled={!barcode.trim() || mutation.isPending}
        data-testid="assign-submit"
      >
        <ScanBarcode className="size-4" />
        {t("orders.assign.submit")}
      </Button>
    </form>
  );
}

export const ORDER_ITEMS_PERSIST_KEY = "tenant-order-items-v1";

/**
 * Lines of the order as a nested client-side table (TEC-374): sort, search
 * and the assigned units with the barcode form while the seller prepares.
 */
function Items({ order, assign }: { order: Order; assign: boolean }) {
  const { t, format } = useLocale();
  const store = useStoreOrder();
  const items = useMemo(() => order.items ?? [], [order.items]);
  const unassign = useMutation({
    mutationFn: (v: { item: string; unit: string }) =>
      ordersService.unassignUnit(order.uuid, v.item, v.unit),
    onSuccess: (updated) => store(updated),
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.assign.failed"))),
  });
  const unassignUnit = unassign.mutate;
  const unassignPending = unassign.isPending;
  const showAssigned = ![
    "draft",
    "submitted",
    "approved",
    "cancelled",
  ].includes(order.status);

  const columns = useMemo(() => {
    const cols: ColumnDef<OrderItem, unknown>[] = [
      createColumn<OrderItem>({
        id: "product",
        accessorFn: (item) => `${item.product.name} ${item.product.sku}`,
        labelKey: "orders.form.product",
        gridPrimary: true,
        cell: ({ row }) => (
          <div
            className="min-w-0"
            data-testid="order-item"
            data-sku={row.original.product.sku}
          >
            <div className="font-medium">{row.original.product.name}</div>
            <div className="text-muted-foreground text-xs" dir="ltr">
              {row.original.product.sku}
            </div>
          </div>
        ),
      }) as ColumnDef<OrderItem, unknown>,
      createColumn<OrderItem>({
        id: "amount",
        accessorFn: (item) => lineAmount(item),
        labelKey: "orders.items.amount",
        cell: ({ row }) =>
          isRoll(row.original.product.unit_type)
            ? `${row.original.meters ?? "0"} ${t("orders.unit.m")}`
            : `${row.original.quantity ?? 0} ${t("orders.unit.pcs")}`,
      }) as ColumnDef<OrderItem, unknown>,
      createColumn<OrderItem>({
        id: "unit_price",
        accessorFn: (item) => amountNumber(item.unit_price),
        labelKey: "orders.items.unit_price",
        cell: ({ row }) =>
          format.currency(
            amountNumber(row.original.unit_price),
            order.currency,
          ),
      }) as ColumnDef<OrderItem, unknown>,
      createColumn<OrderItem>({
        id: "line_total",
        accessorFn: (item) => amountNumber(item.line_total),
        labelKey: "orders.items.line_total",
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="font-semibold" data-testid="line-total">
            {format.currency(
              amountNumber(row.original.line_total),
              order.currency,
            )}
          </span>
        ),
      }) as ColumnDef<OrderItem, unknown>,
    ];
    if (showAssigned) {
      cols.push(
        createColumn<OrderItem>({
          id: "assigned",
          accessorFn: (item) => Number(item.assigned) || 0,
          labelKey: "orders.assign.assigned",
          cell: ({ row }) => {
            const item = row.original;
            const roll = isRoll(item.product.unit_type);
            return (
              <Badge
                variant={lineFullyAssigned(item) ? "success" : "warning"}
                data-testid="line-assigned"
              >
                {item.assigned} / {roll ? item.meters : item.quantity}
              </Badge>
            );
          },
        }) as ColumnDef<OrderItem, unknown>,
        createColumn<OrderItem>({
          id: "units",
          accessorFn: (item) => item.units.map((u) => u.barcode).join(" "),
          labelKey: "orders.items.units",
          enableSorting: false,
          cell: ({ row }) => {
            const item = row.original;
            return (
              <div className="min-w-48 space-y-2">
                {item.units.length > 0 ? (
                  <ul className="flex flex-wrap gap-2">
                    {item.units.map((u) => (
                      <li
                        key={u.unit_uuid}
                        className="bg-muted flex items-center gap-1 rounded px-2 py-1 font-mono text-xs"
                        dir="ltr"
                        data-testid="assigned-unit"
                      >
                        {u.barcode}
                        {u.quantity && u.quantity > 1 ? ` ×${u.quantity}` : ""}
                        {u.meters ? ` ${u.meters} m` : ""}
                        {assign ? (
                          <button
                            type="button"
                            className="text-muted-foreground hover:text-destructive ms-1"
                            aria-label={t("orders.assign.remove")}
                            data-testid="unassign"
                            disabled={unassignPending}
                            onClick={() =>
                              unassignUnit({
                                item: item.uuid,
                                unit: u.unit_uuid,
                              })
                            }
                          >
                            <X className="size-3" />
                          </button>
                        ) : null}
                      </li>
                    ))}
                  </ul>
                ) : null}
                {assign && !lineFullyAssigned(item) ? (
                  <AssignForm order={order} item={item} />
                ) : null}
              </div>
            );
          },
        }) as ColumnDef<OrderItem, unknown>,
      );
    }
    return cols;
  }, [assign, format, order, showAssigned, t, unassignPending, unassignUnit]);

  return (
    <Section
      title={t("orders.detail.items", { count: items.length })}
      icon={<Package className="size-4" />}
      testId="order-items"
    >
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(item) => item.uuid}
        manual={CLIENT_SIDE_MANUAL}
        emptyTitle={t("orders.form.lines_empty")}
        features={{
          persistKey: ORDER_ITEMS_PERSIST_KEY,
          rowSelection: false,
          pagination: items.length > 20,
        }}
      />
      <dl className="mt-4 space-y-1 border-t pt-3 text-sm">
        <div className="flex justify-between gap-2">
          <dt className="text-muted-foreground">
            {t("orders.detail.subtotal")}
          </dt>
          <dd>
            {format.currency(amountNumber(order.subtotal), order.currency)}
          </dd>
        </div>
        <div className="flex justify-between gap-2">
          <dt className="text-muted-foreground">{t("orders.detail.tax")}</dt>
          <dd>
            {format.currency(amountNumber(order.tax_total), order.currency)}
          </dd>
        </div>
        <div className="flex justify-between gap-2 font-semibold">
          <dt>{t("orders.detail.total")}</dt>
          <dd data-testid="order-total">
            {format.currency(amountNumber(order.total), order.currency)}
          </dd>
        </div>
      </dl>
    </Section>
  );
}

function Parties({ order }: { order: Order }) {
  const { t, format } = useLocale();
  const rate = order.rate_snapshot;
  return (
    <Section
      title={t("orders.detail.summary")}
      icon={<Building2 className="size-4" />}
      testId="order-parties"
    >
      <dl className="grid gap-3 sm:grid-cols-2">
        <Field label={t("orders.detail.seller")}>{order.seller.name}</Field>
        <Field label={t("orders.detail.buyer")}>{order.buyer.name}</Field>
        <Field label={t("orders.detail.role")}>
          {t(`orders.role.${order.role}`)}
        </Field>
        <Field label={t("orders.detail.currency")}>
          <span dir="ltr">{order.currency}</span>
        </Field>
        <Field label={t("orders.detail.rate")}>
          {rate ? (
            <span dir="ltr" data-testid="order-rate">
              1 {rate.base} = {rate.rate} {rate.quote} ({rate.rate_date},{" "}
              {rate.source.toUpperCase()})
            </span>
          ) : (
            <span className="text-muted-foreground" data-testid="order-rate">
              {t("orders.detail.rate_pending")}
            </span>
          )}
        </Field>
        <Field label={t("orders.detail.created_at")}>
          {format.dateTime(order.created_at)}
        </Field>
        {order.approved_at ? (
          <Field label={t("orders.detail.approved_at")}>
            {format.dateTime(order.approved_at)}
          </Field>
        ) : null}
        {order.shipped_at ? (
          <Field label={t("orders.detail.shipped_at")}>
            {format.dateTime(order.shipped_at)}
          </Field>
        ) : null}
        {order.note ? (
          <Field label={t("orders.form.note")}>{order.note}</Field>
        ) : null}
        {order.cancel_reason ? (
          <Field label={t("orders.detail.cancel_reason")}>
            {order.cancel_reason}
          </Field>
        ) : null}
      </dl>
    </Section>
  );
}

function HistoryList({ order }: { order: Order }) {
  const { t, format } = useLocale();
  const rows = [...(order.history ?? [])].sort((a, b) =>
    a.created_at < b.created_at ? 1 : -1,
  );
  return (
    <Section
      title={t("orders.detail.history")}
      icon={<History className="size-4" />}
      testId="order-history"
    >
      {rows.length === 0 ? (
        <p className="text-muted-foreground text-sm">—</p>
      ) : (
        <ol className="space-y-3">
          {rows.map((h, i) => (
            <li
              key={`${h.created_at}-${i}`}
              className="border-s-2 ps-3"
              data-testid="history-row"
            >
              <div className="text-sm font-medium">
                {h.from_status
                  ? `${t(`orders.status.${h.from_status}`)} → `
                  : ""}
                {t(`orders.status.${h.to_status}`)}
              </div>
              <div className="text-muted-foreground text-xs">
                {format.dateTime(h.created_at)}
              </div>
              {h.reason ? <div className="text-sm">{h.reason}</div> : null}
            </li>
          ))}
        </ol>
      )}
    </Section>
  );
}

/**
 * Tenant > Orders > detail (TEC-170): lines, totals, frozen rate and the
 * status history, with the actions the caller's side may take (server
 * available_transitions): submit, approve / reject, preparing with barcode
 * assignment, ready, ship, receive and cancel.
 */
export function OrderDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.orders.read);
  const [action, setAction] = useState<OrderAction | null>(null);
  const detail = useQuery({
    queryKey: orderKeys.detail(uuid),
    queryFn: () => ordersService.get(uuid),
    enabled: canRead && Boolean(uuid),
    retry: (count, err) =>
      !(isApiError(err) && (err.status === 404 || err.status === 403)) &&
      count < 2,
  });

  const order = detail.data;
  const title = order?.order_no ?? t("orders.detail.title");
  const header = (
    <PageHeader
      title={title}
      icon={<ShoppingCart className="size-6" />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("orders.list.title"),
          href: routes.tenant.orders.list(slug),
        },
        { label: title },
      ]}
      actions={
        order ? (
          <StatusChip
            label={order.status_label}
            tone={orderStatusTone(order.status)}
          />
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
          description={t("orders.list.forbidden")}
        />
      </div>
    );
  }
  if (detail.isLoading) return <Loading />;
  if (detail.isError || !order) {
    const notFound = isApiError(detail.error) && detail.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={
            notFound ? t("orders.detail.not_found") : t("common.error_generic")
          }
          onRetry={notFound ? undefined : () => void detail.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }

  const actions = orderActions(order);
  const assign = canAssignUnits(can, order);
  const fully = orderFullyAssigned(order);

  return (
    <div className="space-y-6">
      {header}
      {actions.length > 0 || canEditDraft(can, order) ? (
        <div className="flex flex-wrap gap-2" data-testid="order-actions">
          {canEditDraft(can, order) ? (
            <Button asChild variant="outline">
              <Link
                href={routes.tenant.orders.edit(slug, order.uuid)}
                data-testid="order-edit"
              >
                <Pencil className="size-4" />
                {t("orders.actions.edit")}
              </Link>
            </Button>
          ) : null}
          {actions.map((a) => (
            <Button
              key={a.kind}
              type="button"
              variant={a.destructive ? "destructive" : "default"}
              data-action={a.kind}
              disabled={a.kind === "mark_ready" && !fully}
              title={
                a.kind === "mark_ready" && !fully
                  ? t("orders.assign.not_complete")
                  : undefined
              }
              onClick={() => setAction(a)}
            >
              {t(`orders.actions.${a.kind}.button`)}
            </Button>
          ))}
        </div>
      ) : null}
      {assign ? (
        <p className="text-muted-foreground text-sm" data-testid="assign-hint">
          {fully ? t("orders.assign.complete") : t("orders.assign.hint")}
        </p>
      ) : null}
      <div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <Items order={order} assign={assign} />
        <div className="space-y-6">
          <Parties order={order} />
          <HistoryList order={order} />
        </div>
      </div>
      <OrderActionDialog
        order={order}
        action={action}
        onClose={() => setAction(null)}
      />
    </div>
  );
}
