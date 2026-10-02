"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Search, ShoppingCart, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { pricingService } from "@/features/catalog/services/pricing.service";
import {
  canEditDraft,
  resolveOrderListAccess,
} from "@/features/orders/lib/access";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import {
  NOTE_MAX,
  addLine,
  amountNumber,
  formIsValid,
  isRoll,
  linesFromOrder,
  toItemInputs,
  validateOrderForm,
  type OrderFormLine,
} from "@/features/orders/lib/form";
import {
  orderKeys,
  ordersService,
  type Order,
} from "@/features/orders/services/orders.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useDebounce } from "@/hooks/use-debounce";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Purchase price preview of a product from the pricing view (K8). */
function LinePrice({ productUuid }: { productUuid: string }) {
  const { format } = useLocale();
  const price = useQuery({
    queryKey: orderKeys.price(productUuid),
    queryFn: () => pricingService.getProduct(productUuid),
    staleTime: 60_000,
  });
  const rows = (price.data?.prices ?? []).filter((p) => p.purchase_price);
  if (rows.length === 0)
    return <span className="text-muted-foreground">—</span>;
  return (
    <span data-testid="line-price-preview">
      {rows
        .map((p) => format.currency(amountNumber(p.purchase_price), p.currency))
        .join(" · ")}
    </span>
  );
}

function ProductSearch({
  onPick,
  picked,
}: {
  onPick: (p: {
    uuid: string;
    sku: string;
    name: string;
    unit_type: string;
  }) => void;
  picked: Set<string>;
}) {
  const { t } = useLocale();
  const [q, setQ] = useState("");
  const term = useDebounce(q.trim(), 300);
  const products = useQuery({
    queryKey: orderKeys.products(term),
    queryFn: () =>
      catalogService.listProducts({ q: term, active: true, limit: 10 }),
    enabled: term.length >= 2,
  });
  const items = products.data?.items ?? [];
  return (
    <div className="space-y-2">
      <Label htmlFor="order-product-search">{t("orders.form.product")}</Label>
      <div className="relative">
        <Search className="text-muted-foreground pointer-events-none absolute start-3 top-1/2 size-4 -translate-y-1/2" />
        <Input
          id="order-product-search"
          type="search"
          className="ps-9"
          value={q}
          maxLength={100}
          placeholder={t("orders.form.product_placeholder")}
          onChange={(e) => setQ(e.target.value)}
        />
      </div>
      {term.length < 2 ? (
        <p className="text-muted-foreground text-xs">
          {t("orders.form.product_hint")}
        </p>
      ) : products.isLoading ? (
        <p className="text-muted-foreground text-sm">{t("orders.loading")}</p>
      ) : items.length === 0 ? (
        <p className="text-muted-foreground text-sm" data-testid="product-none">
          {t("orders.form.product_none")}
        </p>
      ) : (
        <ul className="divide-y rounded-lg border">
          {items.map((p) => (
            <li
              key={p.uuid}
              className="flex items-center justify-between gap-2 p-2"
              data-testid="product-option"
              data-sku={p.sku}
            >
              <div className="min-w-0">
                <div className="truncate font-medium">{p.name}</div>
                <div className="text-muted-foreground text-xs" dir="ltr">
                  {p.sku} ·{" "}
                  {isRoll(p.unit_type)
                    ? t("orders.unit.meters")
                    : t("orders.unit.pieces")}
                </div>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={picked.has(p.uuid)}
                data-testid="product-add"
                onClick={() => onPick(p)}
              >
                <Plus className="size-4" />
                {picked.has(p.uuid)
                  ? t("orders.form.product_added")
                  : t("orders.form.product_add")}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function OrderForm({
  slug,
  initial,
  canSeePrices,
}: {
  slug: string;
  initial: Order | null;
  canSeePrices: boolean;
}) {
  const { t, format } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const [lines, setLines] = useState<OrderFormLine[]>(() =>
    initial ? linesFromOrder(initial) : [],
  );
  const [note, setNote] = useState("");
  const [saved, setSaved] = useState<Order | null>(initial);
  const [touched, setTouched] = useState(false);
  const errors = useMemo(() => validateOrderForm(lines), [lines]);
  const valid = formIsValid(errors);
  const picked = useMemo(
    () => new Set(lines.map((l) => l.product_uuid)),
    [lines],
  );
  const serverLine = (productUuid: string) =>
    saved?.items?.find((i) => i.product.uuid === productUuid);

  const save = async (): Promise<Order> => {
    const items = toItemInputs(lines);
    const order = saved
      ? await ordersService.replaceItems(saved.uuid, items)
      : await ordersService.create({
          items,
          ...(note.trim() ? { note: note.trim() } : {}),
        });
    qc.setQueryData(orderKeys.detail(order.uuid), order);
    void qc.invalidateQueries({ queryKey: ["orders", "list"] });
    return order;
  };

  const saveDraft = useMutation({
    mutationFn: save,
    onSuccess: (order) => {
      const first = !saved;
      setSaved(order);
      toast.success(t("orders.form.saved", { no: order.order_no }));
      if (first) router.replace(routes.tenant.orders.edit(slug, order.uuid));
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.form.save_failed"))),
  });

  const submit = useMutation({
    mutationFn: async () => {
      const order = await save();
      setSaved(order);
      return ordersService.transition(order.uuid, "submitted");
    },
    onSuccess: (order) => {
      qc.setQueryData(orderKeys.detail(order.uuid), order);
      toast.success(t("orders.form.submitted", { no: order.order_no }));
      router.push(routes.tenant.orders.detail(slug, order.uuid));
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.form.save_failed"))),
  });

  const busy = saveDraft.isPending || submit.isPending;
  const run = (m: typeof saveDraft | typeof submit) => {
    setTouched(true);
    if (!valid) return;
    m.mutate();
  };
  const update = (uuid: string, amount: string) =>
    setLines((ls) =>
      ls.map((l) => (l.product_uuid === uuid ? { ...l, amount } : l)),
    );

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
      <div className="space-y-6">
        <Card>
          <CardContent className="pt-6">
            <ProductSearch
              picked={picked}
              onPick={(p) => setLines((ls) => addLine(ls, p))}
            />
          </CardContent>
        </Card>
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
                const server = serverLine(line.product_uuid);
                const id = `line-${line.product_uuid}`;
                return (
                  <div
                    key={line.product_uuid}
                    className="grid gap-3 rounded-lg border p-3 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_auto]"
                    data-testid="order-line"
                    data-sku={line.sku}
                  >
                    <div className="min-w-0 space-y-1">
                      <div className="font-medium">{line.name}</div>
                      <div className="text-muted-foreground text-xs" dir="ltr">
                        {line.sku}
                      </div>
                      <div className="text-xs">
                        <span className="text-muted-foreground me-1">
                          {server
                            ? t("orders.form.price_snapshot")
                            : t("orders.form.price_preview")}
                        </span>
                        {server ? (
                          <span data-testid="line-price">
                            {format.currency(
                              amountNumber(server.unit_price),
                              saved?.currency ?? "",
                            )}{" "}
                            ={" "}
                            {format.currency(
                              amountNumber(server.line_total),
                              saved?.currency ?? "",
                            )}
                          </span>
                        ) : canSeePrices ? (
                          <LinePrice productUuid={line.product_uuid} />
                        ) : (
                          <span className="text-muted-foreground">
                            {t("orders.form.price_on_save")}
                          </span>
                        )}
                      </div>
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
                        inputMode={
                          isRoll(line.unit_type) ? "decimal" : "numeric"
                        }
                        dir="ltr"
                        value={line.amount}
                        aria-invalid={(touched && Boolean(err)) || undefined}
                        onChange={(e) =>
                          update(line.product_uuid, e.target.value)
                        }
                      />
                      {touched && err ? (
                        <p
                          className="text-destructive text-xs"
                          data-testid="line-error"
                        >
                          {t(`orders.form.errors.${err}`)}
                        </p>
                      ) : null}
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
      <div className="space-y-6">
        <Card data-testid="order-summary">
          <CardHeader>
            <CardTitle className="text-base">
              {t("orders.form.summary")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            {saved ? (
              <dl className="space-y-1">
                <div className="flex justify-between gap-2">
                  <dt className="text-muted-foreground">
                    {t("orders.detail.order_no")}
                  </dt>
                  <dd className="font-mono" dir="ltr">
                    {saved.order_no}
                  </dd>
                </div>
                <div className="flex justify-between gap-2">
                  <dt className="text-muted-foreground">
                    {t("orders.detail.total")}
                  </dt>
                  <dd className="font-semibold" data-testid="draft-total">
                    {format.currency(amountNumber(saved.total), saved.currency)}
                  </dd>
                </div>
              </dl>
            ) : null}
            <p className="text-muted-foreground text-xs">
              {t("orders.form.price_rule")}
            </p>
            {!saved ? (
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
            ) : null}
            <div className="flex flex-col gap-2">
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                data-testid="save-draft"
                onClick={() => run(saveDraft)}
              >
                {t("orders.form.save_draft")}
              </Button>
              <Button
                type="button"
                disabled={busy}
                data-testid="submit-order"
                onClick={() => run(submit)}
              >
                {t("orders.form.submit")}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

/**
 * Tenant > Orders > New / edit draft (TEC-170): the buyer picks products
 * of the brand catalog and amounts; prices are resolved by the server
 * (K8) and shown once the draft is saved, then frozen at approval (K7).
 */
export function OrderFormPage({ slug, uuid }: { slug: string; uuid?: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const access = resolveOrderListAccess(can, org?.type);
  const order = useQuery({
    queryKey: orderKeys.detail(uuid ?? ""),
    queryFn: () => ordersService.get(uuid ?? ""),
    enabled: Boolean(uuid) && access.canCreate,
  });

  const title = uuid ? t("orders.form.edit_title") : t("orders.form.title");
  const header = (
    <PageHeader
      title={title}
      icon={<ShoppingCart className="size-6" />}
      description={t("orders.form.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("orders.list.title"),
          href: access.canRead ? routes.tenant.orders.list(slug) : undefined,
        },
        { label: title },
      ]}
      actions={
        order.data ? (
          <Badge variant="secondary">
            <span dir="ltr">{order.data.order_no}</span>
          </Badge>
        ) : null
      }
    />
  );

  if (!access.canCreate) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("orders.form.forbidden")}
        />
      </div>
    );
  }
  if (uuid && order.isLoading) return <Loading />;
  if (uuid && (order.isError || !order.data)) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void order.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }
  if (order.data && !canEditDraft(can, order.data)) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("orders.form.not_editable")}
          description={t("errors.codes.ORDER_NOT_EDITABLE")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <OrderForm
        slug={slug}
        initial={order.data ?? null}
        canSeePrices={can(permissions.pricing.purchaseRead)}
      />
    </div>
  );
}
