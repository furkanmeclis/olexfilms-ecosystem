"use client";

import { useMutation } from "@tanstack/react-query";
import { AlertTriangle, Minus, Plus, Trash2 } from "lucide-react";
import { useCallback, useRef, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  FieldError,
  Money,
  NativeSelect,
} from "@/features/accounting/components/shared";
import {
  addLine,
  cartTotals,
  hasBarcode,
  isBelowCost,
  lineFromLookup,
  normalizeMoney,
  type CartLine,
} from "@/features/dealer-sales/lib/sales";
import {
  dealerSalesService,
  SALE_PAYMENT_METHODS,
  type ProductSale,
  type SalePaymentMethod,
} from "@/features/dealer-sales/services/dealer-sales.service";
import { serviceWizardService } from "@/features/services/services/service-wizard.service";
import { ScanInput } from "@/features/warehouse/components/scan-input";
import { unitBarcode } from "@/features/warehouse/lib/scan";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type QuickSaleFormProps = {
  /** Display currency before the first line (the dealer's). */
  currency: string;
  canSeePurchasePrice: boolean;
  onSaved?: (sale: ProductSale) => void;
};

/**
 * Quick sale cart (TEC-348). A scanned barcode (or a product picked by
 * search) is resolved through /v1/product-sales/lookup and added as a
 * line with the dealer's price (else the recommended one); a barcode is
 * added only once (a fixed barcode's quantity is raised on its line).
 * Customer is optional except for a cari sale.
 */
export function QuickSaleForm({
  currency,
  canSeePurchasePrice,
  onSaved,
}: QuickSaleFormProps) {
  const { t } = useLocale();
  const [lines, setLines] = useState<CartLine[]>([]);
  const linesRef = useRef<CartLine[]>([]);
  const pending = useRef(new Set<string>());
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [customer, setCustomer] = useState("");
  const [payment, setPayment] = useState<SalePaymentMethod>("cash");
  const [note, setNote] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});

  const update = (next: CartLine[]) => {
    linesRef.current = next;
    setLines(next);
  };

  const lookupError = useCallback(
    (error: unknown) => {
      if (isApiError(error) && error.code === "STOCK_UNAVAILABLE") {
        return t("dealer_sales.quick_sale.not_in_stock");
      }
      if (isApiError(error) && error.isNotFound) {
        return t("dealer_sales.quick_sale.not_found");
      }
      return isApiError(error) ? error.message : t("dealer_sales.toast.failed");
    },
    [t],
  );

  const addItem = useCallback(
    async (query: { barcode?: string; product_uuid?: string }) => {
      const key = query.barcode ?? query.product_uuid ?? "";
      if (query.barcode && hasBarcode(linesRef.current, query.barcode)) {
        setNotice(
          t("dealer_sales.quick_sale.already_added", {
            barcode: query.barcode,
          }),
        );
        return;
      }
      if (pending.current.has(key)) return;
      pending.current.add(key);
      setBusy(true);
      try {
        const item = await dealerSalesService.lookup(query);
        const res = addLine(linesRef.current, lineFromLookup(item));
        if (res.added) {
          update(res.lines);
          setNotice(null);
        } else {
          setNotice(
            t("dealer_sales.quick_sale.already_added", {
              barcode: item.barcode,
            }),
          );
        }
      } catch (error) {
        const message = lookupError(error);
        setNotice(message);
        appToast.error(message);
      } finally {
        pending.current.delete(key);
        setBusy(false);
      }
    },
    [lookupError, t],
  );

  const loadProducts = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      const page = await dealerSalesService.listPrices({
        q: q.trim() || undefined,
        limit: 10,
      });
      return page.items.map((p) => ({
        value: p.product_uuid,
        label: p.name,
        description: p.sku,
      }));
    },
    [],
  );

  const loadCustomers = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      const page = await serviceWizardService.listCustomers({
        q: q.trim() || undefined,
      });
      return page.items.map((c) => ({
        value: c.uuid,
        label: [c.name, c.surname].filter(Boolean).join(" "),
        description: c.phone ?? c.email ?? undefined,
      }));
    },
    [],
  );

  const setLine = (barcode: string, patch: Partial<CartLine>) =>
    update(
      linesRef.current.map((l) =>
        l.barcode === barcode ? { ...l, ...patch } : l,
      ),
    );

  const totals = cartTotals(lines);
  const totalCurrency = lines[0]?.currency ?? currency;

  const create = useMutation({
    mutationFn: () =>
      dealerSalesService.createSale({
        customer_uuid: customer || null,
        payment_method: payment,
        note: note.trim() || undefined,
        lines: lines.map((l) => ({
          barcode: l.barcode,
          quantity: l.quantity,
          unit_price: normalizeMoney(l.unitPrice) ?? l.unitPrice,
        })),
      }),
    onSuccess: (sale) => {
      update([]);
      setCustomer("");
      setPayment("cash");
      setNote("");
      setNotice(null);
      appToast.success(t("dealer_sales.quick_sale.saved"));
      onSaved?.(sale);
    },
    onError: (error) => {
      if (isApiError(error) && error.isValidation) {
        setErrors(error.fieldErrors());
      }
      appToast.error(lookupError(error));
    },
  });

  const submit = () => {
    const next: Record<string, string> = {};
    if (lines.length === 0) next.lines = t("dealer_sales.validation.lines");
    if (totals.invalid) next.lines = t("dealer_sales.validation.price");
    if (payment === "cari" && !customer) {
      next.customer_uuid = t("dealer_sales.validation.cari_customer");
    }
    setErrors(next);
    if (Object.keys(next).length > 0) return;
    create.mutate();
  };

  return (
    <div className="grid gap-6 lg:grid-cols-[2fr_1fr]">
      <div className="space-y-4">
        <ScanInput
          id="quick-sale-scan"
          label={t("dealer_sales.quick_sale.scan_label")}
          placeholder={t("dealer_sales.quick_sale.scan_placeholder")}
          busy={busy}
          onScan={(code) => addItem({ barcode: unitBarcode(code) })}
        />
        <div className="grid gap-1.5">
          <Label htmlFor="quick-sale-product">
            {t("dealer_sales.quick_sale.search_label")}
          </Label>
          <AsyncCombobox
            id="quick-sale-product"
            value=""
            loadOptions={loadProducts}
            placeholder={t("dealer_sales.quick_sale.search_placeholder")}
            onValueChange={(value) => {
              if (value) void addItem({ product_uuid: value });
            }}
          />
        </div>
        {notice ? (
          <p
            className="text-muted-foreground text-sm"
            role="status"
            data-testid="quick-sale-notice"
          >
            {notice}
          </p>
        ) : null}

        <ul
          className="divide-y rounded-md border"
          data-testid="quick-sale-lines"
        >
          {lines.length === 0 ? (
            <li className="text-muted-foreground p-4 text-center text-sm">
              {t("dealer_sales.quick_sale.empty_cart")}
            </li>
          ) : null}
          {lines.map((line) => (
            <li
              key={line.barcode}
              className="flex flex-wrap items-center gap-3 p-3"
              data-testid="quick-sale-line"
            >
              <div className="min-w-40 flex-1">
                <p className="font-medium">{line.name}</p>
                <p
                  className="text-muted-foreground font-mono text-xs"
                  dir="ltr"
                >
                  {line.barcode}
                </p>
              </div>
              {line.unitKind === "fixed" && line.available > 1 ? (
                <div className="flex items-center gap-1">
                  <Button
                    type="button"
                    size="icon"
                    variant="outline"
                    aria-label={t("dealer_sales.quick_sale.decrease")}
                    disabled={line.quantity <= 1}
                    onClick={() =>
                      setLine(line.barcode, { quantity: line.quantity - 1 })
                    }
                  >
                    <Minus className="size-4" />
                  </Button>
                  <span className="w-8 text-center tabular-nums">
                    {line.quantity}
                  </span>
                  <Button
                    type="button"
                    size="icon"
                    variant="outline"
                    aria-label={t("dealer_sales.quick_sale.increase")}
                    disabled={line.quantity >= line.available}
                    onClick={() =>
                      setLine(line.barcode, { quantity: line.quantity + 1 })
                    }
                  >
                    <Plus className="size-4" />
                  </Button>
                </div>
              ) : null}
              <div className="grid w-32 gap-1">
                <Label className="sr-only" htmlFor={`price-${line.barcode}`}>
                  {t("dealer_sales.fields.unit_price")}
                </Label>
                <Input
                  id={`price-${line.barcode}`}
                  inputMode="decimal"
                  dir="ltr"
                  value={line.unitPrice}
                  placeholder="0.00"
                  aria-invalid={
                    normalizeMoney(line.unitPrice) === null ? true : undefined
                  }
                  onChange={(e) =>
                    setLine(line.barcode, { unitPrice: e.target.value })
                  }
                />
              </div>
              {canSeePurchasePrice &&
              isBelowCost(
                normalizeMoney(line.unitPrice),
                line.purchasePrice,
              ) ? (
                <Badge variant="warning">
                  <AlertTriangle className="size-3" />
                  {t("dealer_sales.prices.below_cost")}
                </Badge>
              ) : null}
              <Button
                type="button"
                size="icon"
                variant="ghost"
                aria-label={t("dealer_sales.quick_sale.remove_line")}
                onClick={() =>
                  update(
                    linesRef.current.filter((l) => l.barcode !== line.barcode),
                  )
                }
              >
                <Trash2 className="size-4" />
              </Button>
            </li>
          ))}
        </ul>
        <FieldError id="quick-sale-lines-error" message={errors.lines} />
      </div>

      <div className="space-y-4 rounded-md border p-4">
        <div className="grid gap-1.5">
          <Label htmlFor="quick-sale-customer">
            {t("dealer_sales.fields.customer_optional")}
          </Label>
          <AsyncCombobox
            id="quick-sale-customer"
            value={customer}
            onValueChange={setCustomer}
            loadOptions={loadCustomers}
            clearable
            placeholder={t("dealer_sales.quick_sale.customer_placeholder")}
          />
          <FieldError
            id="quick-sale-customer-error"
            message={errors.customer_uuid}
          />
        </div>
        <NativeSelect
          id="quick-sale-payment"
          label={t("dealer_sales.fields.payment_method")}
          value={payment}
          onChange={(v) => setPayment(v as SalePaymentMethod)}
          options={SALE_PAYMENT_METHODS.map((m) => ({
            value: m,
            label: t(`dealer_sales.payment_methods.${m}`),
          }))}
        />
        <div className="grid gap-1.5">
          <Label htmlFor="quick-sale-note">
            {t("dealer_sales.fields.note")}
          </Label>
          <Textarea
            id="quick-sale-note"
            value={note}
            maxLength={5000}
            rows={2}
            onChange={(e) => setNote(e.target.value)}
          />
        </div>
        <dl className="space-y-1 text-sm">
          <div className="flex justify-between gap-2">
            <dt>{t("dealer_sales.fields.total")}</dt>
            <dd className="font-semibold" data-testid="quick-sale-total">
              <Money amount={totals.total} currency={totalCurrency} />
            </dd>
          </div>
          {canSeePurchasePrice ? (
            <div className="flex justify-between gap-2">
              <dt>{t("dealer_sales.fields.profit_preview")}</dt>
              <dd data-testid="quick-sale-profit">
                {totals.profit !== null ? (
                  <Money amount={totals.profit} currency={totalCurrency} />
                ) : (
                  "—"
                )}
              </dd>
            </div>
          ) : null}
          {canSeePurchasePrice && totals.profitIncomplete ? (
            <p className="text-muted-foreground text-xs">
              {t("dealer_sales.quick_sale.profit_incomplete")}
            </p>
          ) : null}
        </dl>
        <Button
          type="button"
          className="w-full"
          disabled={create.isPending || lines.length === 0}
          onClick={submit}
          data-testid="quick-sale-submit"
        >
          {t("dealer_sales.quick_sale.submit")}
        </Button>
      </div>
    </div>
  );
}
