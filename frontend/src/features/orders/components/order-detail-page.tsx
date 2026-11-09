"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  Building2,
  ClipboardList,
  History,
  Package,
  Pencil,
  Receipt,
  ScanBarcode,
  ShoppingCart,
  Trash2,
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
import { OrderInvoiceDialog } from "@/features/orders/components/order-invoice-dialog";
import {
  LinePickingActions,
  ShippingCard,
} from "@/features/orders/components/order-picking";
import {
  canAssignUnits,
  canEditDraft,
  canOpenPickList,
  lineAmount,
  lineFullyAssigned,
  orderActions,
  orderFullyAssigned,
  type OrderAction,
} from "@/features/orders/lib/access";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import {
  amountNumber,
  invoiceStatusTone,
  isRoll,
  orderStatusTone,
} from "@/features/orders/lib/form";
import {
  orderKeys,
  ordersService,
  type Order,
  type OrderItem,
} from "@/features/orders/services/orders.service";
import { OrderPlacementPanel } from "@/features/warehouse/components/order-placement-panel";
import { ScanInput } from "@/features/warehouse/components/scan-input";
import { scanKind, unitBarcode } from "@/features/warehouse/lib/scan";
import { warehouseService } from "@/features/warehouse/services/warehouse.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { useCanSeePrices } from "@/hooks/use-can-see-prices";

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

/**
 * Picks the line a scanned unit belongs to: the first line of the unit's
 * product that is not fully assigned (TEC-1032, legacy ScanOrderBarcode).
 */
export function lineForProduct(
  order: Order,
  productUuid: string,
): OrderItem | null {
  return (
    (order.items ?? []).find(
      (item) => item.product.uuid === productUuid && !lineFullyAssigned(item),
    ) ?? null
  );
}

/**
 * Order-level scan field (seller, preparing): the barcode resolves to its
 * product through the universal scan and is assigned to the first open line
 * of that product, so the operator scans box after box without finding the
 * row. Rolls are assigned whole; a cut goes through the line form.
 */
function OrderScanBar({ order }: { order: Order }) {
  const { t } = useLocale();
  const store = useStoreOrder();
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const onScan = async (code: string) => {
    if (scanKind(code) === "location") {
      setError(t("orders.scan.not_unit"));
      return;
    }
    const barcode = unitBarcode(code);
    setBusy(true);
    setError(null);
    try {
      const scan = await warehouseService.scan(barcode);
      const product = scan.product;
      if (!product) {
        setError(t("orders.scan.not_unit"));
        return;
      }
      const item = lineForProduct(order, product.uuid);
      if (!item) {
        setError(t("orders.scan.no_line", { product: product.name }));
        return;
      }
      const updated = await ordersService.assignUnit(order.uuid, item.uuid, {
        barcode,
      });
      store(updated);
      toast.success(t("orders.assign.success"));
    } catch (err) {
      setError(orderErrorMessage(err, t, t("orders.assign.failed")));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Card data-testid="order-scan-bar">
      <CardContent className="space-y-2 pt-6">
        <ScanInput
          expect="unit"
          id="order-scan"
          label={t("orders.scan.label")}
          hint={t("orders.scan.hint")}
          busy={busy}
          onScan={onScan}
        />
        {error ? (
          <p
            role="alert"
            className="text-destructive text-sm"
            data-testid="order-scan-error"
          >
            {error}
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}

export const ORDER_ITEMS_PERSIST_KEY = "tenant-order-items-v1";

/**
 * Lines of the order as a nested client-side table (TEC-374): sort, search
 * and the assigned units with the barcode form while the seller prepares.
 */
function Items({ order, assign }: { order: Order; assign: boolean }) {
  const { t, format } = useLocale();
  // TEC-1050: price columns and totals only with a price grant.
  const canSeePrices = useCanSeePrices();
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
  // TEC-1030: the seller sees which lines it invoiced once received.
  const showInvoice = order.status === "received" && order.role === "seller";

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
    ];
    const priceCols: ColumnDef<OrderItem, unknown>[] = [
      createColumn<OrderItem>({
        id: "unit_price",
        accessorFn: (item) => amountNumber(item.unit_price),
        labelKey: "orders.items.unit_price",
        cell: ({ row }) => (
          <span className="inline-flex flex-wrap items-center gap-1">
            {format.currency(
              amountNumber(row.original.unit_price),
              order.currency,
            )}
            {row.original.price_missing ? (
              <Badge
                variant="outline"
                className="border-amber-500 text-amber-700 dark:text-amber-400"
                data-testid="line-price-missing"
              >
                {t("orders.items.price_missing")}
              </Badge>
            ) : null}
          </span>
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
    if (canSeePrices) cols.push(...priceCols);
    if (showInvoice) {
      cols.push(
        createColumn<OrderItem>({
          id: "invoice",
          accessorFn: (item) => item.invoice_no ?? "",
          labelKey: "orders.items.invoice",
          cell: ({ row }) =>
            row.original.invoiced ? (
              <Badge
                variant="success"
                data-testid="line-invoiced"
                title={row.original.invoice_no ?? undefined}
              >
                <span dir="ltr">{row.original.invoice_no}</span>
              </Badge>
            ) : (
              <Badge variant="outline" data-testid="line-invoice-pending">
                {t("orders.invoice.line_pending")}
              </Badge>
            ),
        }) as ColumnDef<OrderItem, unknown>,
      );
    }
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
                  <>
                    <LinePickingActions order={order} item={item} suggest />
                    <AssignForm order={order} item={item} />
                  </>
                ) : null}
              </div>
            );
          },
        }) as ColumnDef<OrderItem, unknown>,
      );
    }
    return cols;
  }, [
    assign,
    canSeePrices,
    format,
    order,
    showAssigned,
    showInvoice,
    t,
    unassignPending,
    unassignUnit,
  ]);

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
      {canSeePrices ? (
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
      ) : null}
    </Section>
  );
}

/**
 * TEC-1030: the seller's manual invoice records on a received order, the
 * "mark invoiced" entry (einvoice.manage) and the undo of a record.
 */
function InvoicesCard({
  order,
  canManage,
}: {
  order: Order;
  canManage: boolean;
}) {
  const { t, format } = useLocale();
  const store = useStoreOrder();
  const [open, setOpen] = useState(false);
  const remove = useMutation({
    mutationFn: (invoice: string) =>
      ordersService.deleteInvoice(order.uuid, invoice),
    onSuccess: (updated) => {
      store(updated);
      toast.success(t("orders.invoice.removed"));
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.invoice.remove_failed"))),
  });
  const invoices = order.invoices ?? [];
  const remaining = (order.items ?? []).some((it) => !it.invoiced);
  return (
    <Section
      title={t("orders.invoice.section")}
      icon={<Receipt className="size-4" />}
      testId="order-invoices"
    >
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <StatusChip
            label={t(`orders.invoice.status.${order.invoice_status}`)}
            tone={invoiceStatusTone(order.invoice_status)}
          />
          {canManage && remaining ? (
            <Button
              type="button"
              size="sm"
              onClick={() => setOpen(true)}
              data-testid="order-mark-invoiced"
            >
              <Receipt className="size-4" />
              {t("orders.invoice.mark")}
            </Button>
          ) : null}
        </div>
        {invoices.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("orders.invoice.empty")}
          </p>
        ) : (
          <ul className="divide-y text-sm">
            {invoices.map((inv) => (
              <li
                key={inv.uuid}
                className="flex items-start justify-between gap-2 py-2"
                data-testid="order-invoice"
              >
                <div className="min-w-0 space-y-0.5">
                  <div className="font-medium" dir="ltr">
                    {inv.invoice_no}
                  </div>
                  <div className="text-muted-foreground text-xs">
                    {format.date(inv.invoice_date)} ·{" "}
                    {t("orders.invoice.items_count", {
                      count: inv.item_uuids.length,
                    })}
                    {inv.created_by
                      ? ` · ${t("orders.invoice.recorded_by", { name: inv.created_by })}`
                      : ""}
                  </div>
                  {inv.note ? (
                    <div className="text-muted-foreground text-xs">
                      {inv.note}
                    </div>
                  ) : null}
                </div>
                {canManage ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={t("orders.invoice.remove")}
                    title={t("orders.invoice.remove")}
                    disabled={remove.isPending}
                    data-testid="order-invoice-remove"
                    onClick={() => {
                      if (window.confirm(t("orders.invoice.remove_confirm")))
                        remove.mutate(inv.uuid);
                    }}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>
      <OrderInvoiceDialog
        order={order}
        open={open}
        onClose={() => setOpen(false)}
      />
    </Section>
  );
}

function Parties({ order }: { order: Order }) {
  const { t, format } = useLocale();
  // TEC-1050: currency and the frozen rate only with a price grant.
  const canSeePrices = useCanSeePrices();
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
        {order.seller_warehouse ? (
          <Field label={t("orders.detail.seller_warehouse")}>
            <span data-testid="order-seller-warehouse">
              {order.seller_warehouse.name}
            </span>
          </Field>
        ) : null}
        <Field label={t("orders.detail.role")}>
          {t(`orders.role.${order.role}`)}
        </Field>
        {canSeePrices ? (
          <>
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
                <span
                  className="text-muted-foreground"
                  data-testid="order-rate"
                >
                  {t("orders.detail.rate_pending")}
                </span>
              )}
            </Field>
          </>
        ) : null}
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
        {order.delivered_at ? (
          <Field label={t("orders.detail.delivered_at")}>
            {format.dateTime(order.delivered_at)}
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
 * assignment, ready, ship, receive and cancel. A received order bought by
 * a center or distributor shows the shelving panel (TEC-765).
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
          <div className="flex flex-wrap items-center gap-2">
            {order.created_by_seller ? (
              <Badge variant="secondary" data-testid="created-by-seller">
                {t("orders.detail.created_by_seller")}
              </Badge>
            ) : null}
            {order.status === "received" && order.role === "seller" ? (
              <StatusChip
                label={t(`orders.invoice.status.${order.invoice_status}`)}
                tone={invoiceStatusTone(order.invoice_status)}
              />
            ) : null}
            <StatusChip
              label={order.status_label}
              tone={orderStatusTone(order.status)}
            />
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
  const pickList = canOpenPickList(can, order);

  return (
    <div className="space-y-6">
      {header}
      {actions.length > 0 || canEditDraft(can, order) || pickList ? (
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
          {pickList ? (
            <Button asChild variant="outline">
              <Link
                href={routes.tenant.orders.pickList(slug, order.uuid)}
                data-testid="order-pick-list"
              >
                <ClipboardList className="size-4" />
                {t("orders.picking.pick_list")}
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
      {assign && !fully ? <OrderScanBar order={order} /> : null}
      <OrderPlacementPanel slug={slug} order={order} />
      {/* TEC-1052 (5.10): while preparing, the lines (barcode assignment)
          take the full width and the side cards move below them. */}
      <div
        className={
          assign
            ? "grid gap-6"
            : "grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]"
        }
        data-testid="order-detail-layout"
      >
        <Items order={order} assign={assign} />
        <div
          className={
            assign
              ? "grid items-start gap-6 lg:grid-cols-2 2xl:grid-cols-3"
              : "space-y-6"
          }
        >
          <Parties order={order} />
          {order.status === "received" && order.role === "seller" ? (
            <InvoicesCard
              order={order}
              canManage={can(permissions.einvoice.manage)}
            />
          ) : null}
          <ShippingCard
            order={order}
            canShip={can(permissions.orders.ship)}
            canReceive={can(permissions.orders.receive)}
          />
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
