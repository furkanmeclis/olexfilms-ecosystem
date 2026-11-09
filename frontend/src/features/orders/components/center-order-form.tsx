"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { NewRecipientDialog } from "@/features/accounts/components/new-recipient-dialog";
import { PlotterToggle } from "@/features/orders/components/plotter-toggle";
import { ProductSearch } from "@/features/orders/components/product-search";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import {
  NOTE_MAX,
  addLine,
  formIsValid,
  isRoll,
  previewLine,
  toItemInputs,
  validateOrderForm,
  type OrderFormLine,
} from "@/features/orders/lib/form";
import {
  orderKeys,
  ordersService,
  type OrderQuoteItem,
} from "@/features/orders/services/orders.service";
import {
  warehouseKeys,
  warehouseService,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * TEC-1024: the center opens an order for a dealer directly under it. The
 * order starts approved and goes straight to the warehouse flow; prices
 * never block (a typed unit price wins, else the dealer's purchase price,
 * else 0 flagged as missing).
 *
 * TEC-1028: the order falls into a warehouse of the center, the primary one
 * unless another active warehouse is picked here. Without warehouse.read the
 * picker stays hidden and the server applies the primary warehouse.
 *
 * TEC-1052: a user holding orders.ship can mark that he prepares the order
 * himself; then no warehouse staff is told of it.
 *
 * TEC-1064: with pricing.sale.read every line shows the list price (order
 * currency, USD base), the discount (the dealer's first-order discount on
 * its first order, else its standard one; editable), the discounted unit
 * price and the line total; the summary shows the total in the order
 * currency and at the current rate in TRY. A line can be a gift (0). The
 * server recomputes every amount.
 */
export function CenterOrderForm({
  slug,
  centerUuid,
}: {
  slug: string;
  centerUuid: string;
}) {
  const { t, format } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const [dealer, setDealer] = useState<ComboboxOption | null>(null);
  const [newRecipient, setNewRecipient] = useState(false); // TEC-1062
  const [lines, setLines] = useState<OrderFormLine[]>([]);
  const [note, setNote] = useState("");
  const [warehouse, setWarehouse] = useState<string | null>(null);
  // TEC-1052: the creator prepares the order himself (orders.ship): no
  // warehouse staff is notified.
  const { can } = usePermission();
  const canShip = can(permissions.orders.ship);
  const canSeePrices = can(permissions.pricing.saleRead);
  const [selfPrepare, setSelfPrepare] = useState(false);
  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
    staleTime: 5 * 60_000,
    retry: false,
  });
  const activeWarehouses = useMemo(
    () => (warehouses.data?.items ?? []).filter((w) => w.active),
    [warehouses.data?.items],
  );
  // The primary warehouse is preselected until the user picks one.
  const warehouseValue =
    warehouse ??
    activeWarehouses.find((w) => w.is_primary)?.uuid ??
    activeWarehouses[0]?.uuid ??
    "";
  const [touched, setTouched] = useState(false);
  const errors = useMemo(() => validateOrderForm(lines), [lines]);
  const valid = formIsValid(errors) && dealer !== null;
  const picked = useMemo(
    () => new Set(lines.map((l) => l.product_uuid)),
    [lines],
  );

  // TEC-1064: the dealer's prices, default discount and the TRY rate.
  const productUuids = useMemo(
    () => lines.map((l) => l.product_uuid).sort(),
    [lines],
  );
  const quote = useQuery({
    queryKey: orderKeys.quote(dealer?.value ?? "", productUuids),
    queryFn: () =>
      ordersService.quote({
        buyer_org_uuid: dealer?.value ?? "",
        product_uuids: productUuids,
      }),
    enabled: canSeePrices && dealer !== null,
    placeholderData: keepPreviousData,
    staleTime: 30_000,
    retry: false,
  });
  const quoteData =
    dealer !== null && canSeePrices && quote.data?.prices_visible
      ? quote.data
      : undefined;
  const pricesVisible = quoteData !== undefined;
  const quoteBy = useMemo(
    () =>
      new Map<string, OrderQuoteItem>(
        (quoteData?.items ?? []).map((i) => [i.product_uuid, i]),
      ),
    [quoteData?.items],
  );
  const currency = quoteData?.currency ?? "";
  const defaultDiscount = quoteData?.discount_pct ?? null;

  const previews = useMemo(
    () =>
      new Map(
        lines.map((line) => {
          const q = quoteBy.get(line.product_uuid);
          const list = q ? (q.list_unit_price ?? null) : undefined;
          const typed = (line.discount_pct ?? "").trim();
          return [
            line.product_uuid,
            {
              loaded: q !== undefined,
              list: list ?? null,
              discount: typed !== "" ? typed : defaultDiscount,
              ...previewLine({
                list,
                discount: typed !== "" ? typed : defaultDiscount,
                manual: list === null ? line.unit_price : undefined,
                gift: line.is_gift,
                amount: line.amount,
              }),
            },
          ];
        }),
      ),
    [lines, quoteBy, defaultDiscount],
  );
  const total = useMemo(
    () =>
      Math.round(
        [...previews.values()].reduce((s, p) => s + (p.total ?? 0), 0) * 100,
      ) / 100,
    [previews],
  );
  const tryRate = quoteData?.try_rate ? Number(quoteData.try_rate) : null;
  const showTry = pricesVisible && currency !== "TRY" && lines.length > 0;

  // Labels of every dealer the search returned, to name the picked one.
  const names = useRef(new Map<string, string>());
  const loadDealers = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      const items = await qc.fetchQuery({
        queryKey: orderKeys.childDealers(centerUuid, q),
        queryFn: () => ordersService.listChildDealers(centerUuid, q),
        staleTime: 30_000,
      });
      for (const o of items) names.current.set(o.uuid, o.name);
      return items.map((o) => ({ value: o.uuid, label: o.name }));
    },
    [centerUuid, qc],
  );

  const create = useMutation({
    mutationFn: () =>
      ordersService.create({
        buyer_org_uuid: dealer?.value ?? "",
        ...(warehouseValue ? { seller_warehouse_uuid: warehouseValue } : {}),
        ...(canShip && selfPrepare ? { self_prepare: true } : {}),
        items: toItemInputs(lines, true),
        ...(note.trim() ? { note: note.trim() } : {}),
      }),
    onSuccess: (order) => {
      qc.setQueryData(orderKeys.detail(order.uuid), order);
      void qc.invalidateQueries({ queryKey: ["orders", "list"] });
      void qc.invalidateQueries({ queryKey: ["orders", "quote"] });
      toast.success(t("orders.center_form.created", { no: order.order_no }));
      if ((order.items ?? []).some((i) => i.price_missing)) {
        toast.warning(t("orders.center_form.price_missing_warning"));
      }
      router.push(routes.tenant.orders.detail(slug, order.uuid));
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.form.save_failed"))),
  });

  const update = (uuid: string, patch: Partial<OrderFormLine>) =>
    setLines((ls) =>
      ls.map((l) => (l.product_uuid === uuid ? { ...l, ...patch } : l)),
    );

  const money = (v: number | null) =>
    v === null ? "—" : format.currency(v, currency || "USD");
  const discountInfo = (() => {
    if (!quoteData) return null;
    if (quoteData.discount_pct === null) {
      return t("orders.center_form.discount_none");
    }
    return quoteData.first_order && quoteData.first_order_discount_pct !== null
      ? t("orders.center_form.discount_first_order", {
          pct: quoteData.discount_pct,
        })
      : t("orders.center_form.discount_standard", {
          pct: quoteData.discount_pct,
        });
  })();

  // TEC-1052 (5.10): dealer, warehouse and summary sit beside the product
  // picker; the lines get the full width below.
  return (
    <div className="space-y-6">
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
        <div className="space-y-6">
          <Card>
            <CardContent className="space-y-2 pt-6">
              <Label htmlFor="center-order-dealer">
                {t("orders.center_form.dealer")}
              </Label>
              <AsyncCombobox
                id="center-order-dealer"
                data-testid="center-order-dealer"
                value={dealer?.value}
                initialOptions={dealer ? [dealer] : undefined}
                loadOptions={loadDealers}
                placeholder={t("orders.center_form.dealer_placeholder")}
                searchPlaceholder={t("orders.center_form.dealer_search")}
                emptyText={t("orders.center_form.dealer_none")}
                aria-invalid={(touched && dealer === null) || undefined}
                onValueChange={(value) =>
                  setDealer(
                    value
                      ? { value, label: names.current.get(value) ?? value }
                      : null,
                  )
                }
              />
              <Button
                type="button"
                variant="link"
                size="sm"
                className="h-auto px-0"
                data-testid="center-order-new-recipient"
                onClick={() => setNewRecipient(true)}
              >
                {t("organizations.recipients.new")}
              </Button>
              <NewRecipientDialog
                open={newRecipient}
                onOpenChange={setNewRecipient}
                onCreated={(r) => setDealer({ value: r.uuid, label: r.name })}
              />
              {touched && dealer === null ? (
                <p
                  className="text-destructive text-xs"
                  data-testid="dealer-error"
                >
                  {t("orders.center_form.dealer_required")}
                </p>
              ) : null}
              {discountInfo ? (
                <p
                  className="text-muted-foreground text-xs"
                  data-testid="center-order-discount-info"
                >
                  {discountInfo}
                </p>
              ) : null}
              {activeWarehouses.length > 0 ? (
                <div className="space-y-2 pt-2">
                  <Label htmlFor="center-order-warehouse">
                    {t("orders.center_form.warehouse")}
                  </Label>
                  <Select value={warehouseValue} onValueChange={setWarehouse}>
                    <SelectTrigger
                      id="center-order-warehouse"
                      className="w-full"
                      data-testid="center-order-warehouse"
                    >
                      <SelectValue
                        placeholder={t(
                          "orders.center_form.warehouse_placeholder",
                        )}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {activeWarehouses.map((w) => (
                        <SelectItem
                          key={w.uuid}
                          value={w.uuid}
                          data-testid="center-order-warehouse-option"
                        >
                          {w.is_primary
                            ? t("orders.center_form.warehouse_primary", {
                                name: w.name,
                              })
                            : w.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <p className="text-muted-foreground text-xs">
                    {t("orders.center_form.warehouse_hint")}
                  </p>
                </div>
              ) : null}
              {canShip ? (
                <div className="flex items-start gap-2 pt-2">
                  <Checkbox
                    id="center-order-self-prepare"
                    checked={selfPrepare}
                    onCheckedChange={(v) => setSelfPrepare(v === true)}
                    data-testid="center-order-self-prepare"
                  />
                  <div className="space-y-1">
                    <Label
                      htmlFor="center-order-self-prepare"
                      className="text-sm"
                    >
                      {t("orders.center_form.self_prepare")}
                    </Label>
                    <p className="text-muted-foreground text-xs">
                      {t("orders.center_form.self_prepare_hint")}
                    </p>
                  </div>
                </div>
              ) : null}
            </CardContent>
          </Card>
          <Card data-testid="order-summary">
            <CardHeader>
              <CardTitle className="text-base">
                {t("orders.form.summary")}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
              {pricesVisible ? (
                <div className="space-y-1" data-testid="center-order-totals">
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="text-muted-foreground">
                      {t("orders.center_form.total", { currency })}
                    </span>
                    <span
                      className="text-base font-semibold tabular-nums"
                      dir="ltr"
                      data-testid="center-order-total"
                    >
                      {money(total)}
                    </span>
                  </div>
                  {showTry ? (
                    tryRate !== null ? (
                      <div
                        className="text-muted-foreground flex items-baseline justify-between gap-2 text-xs"
                        data-testid="center-order-total-try"
                      >
                        <span>
                          {t("orders.center_form.total_try", {
                            currency,
                            rate: format.number(tryRate, {
                              maximumFractionDigits: 4,
                            }),
                            date: quoteData?.rate_date
                              ? format.date(quoteData.rate_date)
                              : "",
                          })}
                        </span>
                        <span className="tabular-nums" dir="ltr">
                          {format.currency(
                            Math.round(total * tryRate * 100) / 100,
                            "TRY",
                          )}
                        </span>
                      </div>
                    ) : (
                      <p className="text-muted-foreground text-xs">
                        {t("orders.center_form.total_try_unavailable")}
                      </p>
                    )
                  ) : null}
                </div>
              ) : null}
              <p className="text-muted-foreground text-xs">
                {t("orders.center_form.price_rule")}
              </p>
              <div className="space-y-1.5">
                <Label htmlFor="order-note">{t("orders.form.note")}</Label>
                <Textarea
                  id="order-note"
                  value={note}
                  rows={3}
                  maxLength={NOTE_MAX}
                  onChange={(e) => setNote(e.target.value)}
                />
              </div>
              <Button
                type="button"
                className="w-full"
                disabled={create.isPending}
                data-testid="center-order-create"
                onClick={() => {
                  setTouched(true);
                  if (valid) create.mutate();
                }}
              >
                {t("orders.center_form.create")}
              </Button>
            </CardContent>
          </Card>
        </div>
        <Card>
          <CardContent className="pt-6">
            <ProductSearch
              picked={picked}
              onPick={(p) =>
                setLines((ls) =>
                  addLine(ls, p).map((l) =>
                    l.product_uuid === p.uuid && l.unit_price === undefined
                      ? { ...l, unit_price: "", discount_pct: "" }
                      : l,
                  ),
                )
              }
            />
          </CardContent>
        </Card>
      </div>
      <Card data-testid="order-lines">
        <CardHeader>
          <CardTitle className="text-base">
            {t("orders.form.lines_title", { count: lines.length })}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {lines.length === 0 ? (
            <p
              className={
                touched && errors.form === "empty"
                  ? "text-destructive text-sm"
                  : "text-muted-foreground text-sm"
              }
              data-testid="lines-empty"
            >
              {t("orders.form.lines_empty")}
            </p>
          ) : (
            lines.map((line) => {
              const err = errors.lines[line.product_uuid];
              const id = `line-${line.product_uuid}`;
              const pv = previews.get(line.product_uuid);
              const gift = line.is_gift === true;
              // The typed price is offered when the prices are hidden or
              // the dealer has no list price for the product.
              const manualPrice =
                !pricesVisible || (pv?.loaded === true && pv.list === null);
              return (
                <div
                  key={line.product_uuid}
                  className="grid gap-3 rounded-lg border p-3 sm:grid-cols-2 lg:grid-cols-[minmax(0,3fr)_minmax(6rem,1fr)_minmax(6rem,1fr)_minmax(6rem,1fr)_minmax(8rem,1.2fr)_auto_auto] lg:items-end"
                  data-testid="order-line"
                  data-sku={line.sku}
                >
                  <div className="min-w-0 space-y-1 sm:col-span-2 lg:col-span-1">
                    <div className="font-medium">{line.name}</div>
                    <div className="text-muted-foreground text-xs" dir="ltr">
                      {line.sku}
                    </div>
                    <PlotterToggle
                      id={`${id}-plotter`}
                      checked={line.plotter_cut === true}
                      onChange={(v) =>
                        update(line.product_uuid, { plotter_cut: v })
                      }
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor={id} className="text-xs">
                      {isRoll(line.unit_type)
                        ? t("orders.form.meters")
                        : t("orders.form.quantity")}
                    </Label>
                    <Input
                      id={id}
                      name="amount"
                      inputMode={isRoll(line.unit_type) ? "decimal" : "numeric"}
                      dir="ltr"
                      value={line.amount}
                      aria-invalid={
                        (touched &&
                          Boolean(err) &&
                          err !== "price_invalid" &&
                          err !== "discount_invalid") ||
                        undefined
                      }
                      onChange={(e) =>
                        update(line.product_uuid, { amount: e.target.value })
                      }
                    />
                  </div>
                  <div className="space-y-1">
                    <span className="text-muted-foreground block text-xs">
                      {t("orders.center_form.list_price", {
                        currency: currency || "—",
                      })}
                    </span>
                    <div
                      className="flex h-9 items-center tabular-nums"
                      dir="ltr"
                      data-testid="line-list-price"
                    >
                      {pricesVisible && pv?.list != null
                        ? money(Number(pv.list))
                        : "—"}
                    </div>
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor={`${id}-discount`} className="text-xs">
                      {t("orders.center_form.discount")}
                    </Label>
                    <Input
                      id={`${id}-discount`}
                      name="discount_pct"
                      inputMode="decimal"
                      dir="ltr"
                      disabled={gift || !pricesVisible || manualPrice}
                      value={line.discount_pct ?? ""}
                      placeholder={
                        defaultDiscount !== null ? defaultDiscount : "0"
                      }
                      data-testid="line-discount"
                      aria-invalid={
                        (touched && err === "discount_invalid") || undefined
                      }
                      onChange={(e) =>
                        update(line.product_uuid, {
                          discount_pct: e.target.value,
                        })
                      }
                    />
                  </div>
                  <div className="space-y-1">
                    {manualPrice ? (
                      <>
                        <Label htmlFor={`${id}-price`} className="text-xs">
                          {t("orders.center_form.unit_price")}
                        </Label>
                        <Input
                          id={`${id}-price`}
                          name="unit_price"
                          inputMode="decimal"
                          dir="ltr"
                          disabled={gift}
                          value={line.unit_price ?? ""}
                          placeholder={t(
                            "orders.center_form.unit_price_placeholder",
                          )}
                          data-testid="line-unit-price"
                          aria-invalid={
                            (touched && err === "price_invalid") || undefined
                          }
                          onChange={(e) =>
                            update(line.product_uuid, {
                              unit_price: e.target.value,
                            })
                          }
                        />
                      </>
                    ) : (
                      <>
                        <span className="text-muted-foreground block text-xs">
                          {t("orders.center_form.discounted_price")}
                        </span>
                        <div
                          className="flex h-9 items-center tabular-nums"
                          dir="ltr"
                          data-testid="line-discounted-price"
                        >
                          {money(pv?.unit ?? null)}
                        </div>
                      </>
                    )}
                    {pricesVisible ? (
                      <div
                        className="text-muted-foreground text-xs tabular-nums"
                        data-testid="line-total"
                      >
                        {t("orders.center_form.line_total", {
                          total: money(pv?.total ?? null),
                        })}
                      </div>
                    ) : null}
                  </div>
                  <div className="flex items-center gap-2 lg:h-9">
                    <Switch
                      id={`${id}-gift`}
                      checked={gift}
                      data-testid="line-gift"
                      onCheckedChange={(v) =>
                        update(line.product_uuid, { is_gift: v === true })
                      }
                    />
                    <Label htmlFor={`${id}-gift`} className="text-xs">
                      {t("orders.center_form.gift")}
                    </Label>
                  </div>
                  <div className="flex items-start justify-end">
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      aria-label={t("orders.form.remove")}
                      data-testid="line-remove"
                      onClick={() =>
                        setLines((ls) =>
                          ls.filter(
                            (l) => l.product_uuid !== line.product_uuid,
                          ),
                        )
                      }
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                  {touched && err ? (
                    <p
                      className="text-destructive text-xs sm:col-span-2 lg:col-span-7"
                      data-testid="line-error"
                    >
                      {t(`orders.form.errors.${err}`)}
                    </p>
                  ) : null}
                </div>
              );
            })
          )}
          {touched && errors.form === "too_many" ? (
            <p className="text-destructive text-sm" data-testid="form-error">
              {t("orders.form.errors.too_many")}
            </p>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
