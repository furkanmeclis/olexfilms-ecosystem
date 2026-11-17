"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Banknote } from "lucide-react";
import { useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { PageHeader } from "@/components/layout/page-header";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  type BrandCurrencyChange,
  brandCurrencyKeys,
  brandCurrencyService,
  type BrandPriceConversion,
  type PlatformBrand,
} from "@/features/brand-currency/services/brand-currency.service";
import { isApiError } from "@/lib/api";
import { useAuthStore } from "@/lib/auth/session-store";
import { useCurrencyList } from "@/lib/currency";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type OpenOrdersOption = "block" | "reprice";

/** Rows of a preview list shown before "and N more". */
const PREVIEW_ROWS = 50;

/**
 * Platform > Settings > Brands (TEC-1214). Each brand's price and order
 * currency; a platform admin previews a change (missing prices, open
 * orders, rates) and applies it, and converts the price list to another
 * currency as draft prices first. Organization (ledger) currencies and
 * existing records do not change.
 */
export function BrandCurrencyPage() {
  const { t } = useLocale();
  const isAdmin = useAuthStore((s) => Boolean(s.user?.isSuperAdmin));
  const brands = useQuery({
    queryKey: brandCurrencyKeys.list,
    queryFn: () => brandCurrencyService.list(),
  });
  const currencies = useCurrencyList(isAdmin);

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("rates.brand_currency.title")}
        description={t("rates.brand_currency.description")}
        icon={<Banknote className="size-6" />}
      />
      {brands.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{t("rates.toast.failed")}</AlertDescription>
        </Alert>
      ) : null}
      {(brands.data?.items ?? []).map((brand) => (
        <BrandCurrencyCard
          key={brand.uuid}
          brand={brand}
          local={brands.data?.local_currency ?? ""}
          isAdmin={isAdmin}
          currencies={currencies.data ?? []}
        />
      ))}
    </div>
  );
}

function errorText(error: unknown, fallback: string) {
  return isApiError(error) ? error.message : fallback;
}

function CurrencySelect({
  id,
  value,
  options,
  onChange,
  label,
}: {
  id: string;
  value: string;
  options: string[];
  onChange: (value: string) => void;
  label: string;
}) {
  return (
    <label className="space-y-1 text-sm" htmlFor={id}>
      <span className="text-muted-foreground">{label}</span>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger id={id} className="w-36" data-testid={id}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((code) => (
            <SelectItem key={code} value={code}>
              {code}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </label>
  );
}

function CheckboxField({
  id,
  checked,
  onChange,
  label,
}: {
  id: string;
  checked: boolean;
  onChange: (value: boolean) => void;
  label: string;
}) {
  return (
    <label className="flex items-center gap-2 text-sm" htmlFor={id}>
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(v) => onChange(v === true)}
      />
      {label}
    </label>
  );
}

function MoreRows({ total, shown }: { total: number; shown: number }) {
  const { t } = useLocale();
  if (total <= shown) return null;
  return (
    <li className="text-muted-foreground text-xs">
      {t("rates.brand_currency.more_rows", { count: total - shown })}
    </li>
  );
}

function BrandCurrencyCard({
  brand,
  local,
  isAdmin,
  currencies,
}: {
  brand: PlatformBrand;
  local: string;
  isAdmin: boolean;
  currencies: string[];
}) {
  const { t } = useLocale();
  const others = currencies.filter((c) => c !== brand.currency);
  return (
    <Card data-testid={`brand-currency-${brand.slug}`}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {brand.name}
          <Badge variant="outline" dir="ltr">
            {brand.currency}
          </Badge>
        </CardTitle>
        <CardDescription>
          {t("rates.brand_currency.current", {
            currency: brand.currency,
            local: local || "—",
          })}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {isAdmin ? (
          <>
            <ChangeSection brand={brand} others={others} />
            <ConvertSection brand={brand} others={others} />
          </>
        ) : (
          <p className="text-muted-foreground text-sm">
            {t("rates.brand_currency.admin_only")}
          </p>
        )}
      </CardContent>
    </Card>
  );
}

function ChangeSection({
  brand,
  others,
}: {
  brand: PlatformBrand;
  others: string[];
}) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const [target, setTarget] = useState("");
  const [openOrders, setOpenOrders] = useState<OpenOrdersOption>("block");
  const [allowMissing, setAllowMissing] = useState(false);
  const [preview, setPreview] = useState<{
    key: string;
    out: BrandCurrencyChange;
  } | null>(null);
  const [confirming, setConfirming] = useState(false);
  const to = target || others[0] || "";
  const body = {
    currency: to,
    open_orders: openOrders,
    allow_missing_prices: allowMissing,
  };
  // A preview belongs to the options it was made with.
  const key = JSON.stringify(body);
  const current = preview?.key === key ? preview.out : null;

  const run = useMutation({
    mutationFn: (vars: { dryRun: boolean; key: string }) =>
      brandCurrencyService.changeCurrency(brand.uuid, {
        ...(JSON.parse(vars.key) as typeof body),
        dry_run: vars.dryRun,
      }),
    onSuccess: async (out, vars) => {
      setPreview({ key: vars.key, out });
      if (out.applied) {
        setConfirming(false);
        setPreview(null);
        await queryClient.invalidateQueries({
          queryKey: brandCurrencyKeys.all,
        });
        appToast.success(
          t("rates.brand_currency.applied", {
            from: out.from,
            to: out.to,
            count: out.repriced_orders,
          }),
        );
      }
    },
    onError: (error) => {
      setConfirming(false);
      appToast.error(errorText(error, t("rates.toast.failed")));
    },
  });

  const rateLine = (
    rate: BrandCurrencyChange["rate_to_local"],
    missingKey: string,
  ) =>
    rate
      ? t("rates.brand_currency.rate_line", {
          base: rate.base,
          quote: rate.quote,
          rate: format.number(Number(rate.rate), { maximumFractionDigits: 6 }),
          date: rate.rate_date,
        })
      : t(missingKey);

  return (
    <section className="space-y-3" aria-labelledby={`change-${brand.slug}`}>
      <h3 id={`change-${brand.slug}`} className="font-semibold">
        {t("rates.brand_currency.change_title")}
      </h3>
      <p className="text-muted-foreground text-sm">
        {t("rates.brand_currency.change_description")}
      </p>
      <div className="flex flex-wrap items-end gap-4">
        <CurrencySelect
          id={`brand-currency-target-${brand.slug}`}
          label={t("rates.brand_currency.target")}
          value={to}
          options={others}
          onChange={setTarget}
        />
        <label className="space-y-1 text-sm">
          <span className="text-muted-foreground">
            {t("rates.brand_currency.open_orders")}
          </span>
          <Select
            value={openOrders}
            onValueChange={(v) => setOpenOrders(v as OpenOrdersOption)}
          >
            <SelectTrigger className="w-56">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="block">
                {t("rates.brand_currency.open_orders_block")}
              </SelectItem>
              <SelectItem value="reprice">
                {t("rates.brand_currency.open_orders_reprice")}
              </SelectItem>
            </SelectContent>
          </Select>
        </label>
        <CheckboxField
          id={`brand-currency-allow-missing-${brand.slug}`}
          checked={allowMissing}
          onChange={setAllowMissing}
          label={t("rates.brand_currency.allow_missing")}
        />
        <Button
          type="button"
          variant="outline"
          disabled={!to || run.isPending}
          onClick={() => run.mutate({ dryRun: true, key })}
          data-testid={`brand-currency-preview-${brand.slug}`}
        >
          {t("rates.brand_currency.preview")}
        </Button>
      </div>

      {current ? (
        <div className="space-y-3 rounded-md border p-3 text-sm">
          <ul className="space-y-1">
            <li>
              {t("rates.brand_currency.summary", {
                from: current.from,
                to: current.to,
                products: current.active_products,
                missing: current.missing_price_count,
                orders: current.open_order_count,
              })}
            </li>
            <li>
              {rateLine(
                current.rate_to_local,
                "rates.brand_currency.rate_to_local_missing",
              )}
            </li>
            {current.open_order_count > 0 ? (
              <li>
                {rateLine(
                  current.conversion_rate,
                  "rates.brand_currency.conversion_rate_missing",
                )}
              </li>
            ) : null}
          </ul>

          {current.blockers.length ? (
            <Alert variant="destructive">
              <AlertTitle>
                {t("rates.brand_currency.blockers_title")}
              </AlertTitle>
              <AlertDescription>
                <ul className="list-disc ps-5">
                  {current.blockers.map((b) => (
                    <li key={b}>{t(`rates.brand_currency.blocker.${b}`)}</li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          ) : (
            <p className="text-sm">{t("rates.brand_currency.no_blockers")}</p>
          )}

          {current.missing_price_count > 0 ? (
            <div className="space-y-1">
              <h4 className="font-medium">
                {t("rates.brand_currency.missing_prices", {
                  count: current.missing_price_count,
                  currency: current.to,
                })}
              </h4>
              <ul
                className="max-h-48 space-y-0.5 overflow-y-auto"
                data-testid="brand-currency-missing"
              >
                {current.missing_prices.slice(0, PREVIEW_ROWS).map((p) => (
                  <li key={p.product_uuid}>
                    <span className="font-mono text-xs" dir="ltr">
                      {p.sku}
                    </span>{" "}
                    {p.name}
                  </li>
                ))}
                <MoreRows
                  total={current.missing_price_count}
                  shown={Math.min(PREVIEW_ROWS, current.missing_prices.length)}
                />
              </ul>
            </div>
          ) : null}

          {current.open_order_count > 0 ? (
            <div className="space-y-1">
              <h4 className="font-medium">
                {t("rates.brand_currency.open_orders_list", {
                  count: current.open_order_count,
                })}
              </h4>
              <ul
                className="max-h-48 space-y-0.5 overflow-y-auto"
                data-testid="brand-currency-orders"
              >
                {current.open_orders.slice(0, PREVIEW_ROWS).map((o) => (
                  <li key={o.uuid}>
                    <span className="font-mono text-xs" dir="ltr">
                      {o.order_no}
                    </span>{" "}
                    {o.seller_name} → {o.buyer_name} ·{" "}
                    {t(`orders.status.${o.status}`)} ·{" "}
                    {format.currency(Number(o.total), o.currency)}
                  </li>
                ))}
                <MoreRows
                  total={current.open_order_count}
                  shown={Math.min(PREVIEW_ROWS, current.open_orders.length)}
                />
              </ul>
            </div>
          ) : null}

          <Button
            type="button"
            disabled={current.blockers.length > 0 || run.isPending}
            onClick={() => setConfirming(true)}
            data-testid={`brand-currency-apply-${brand.slug}`}
          >
            {t("rates.brand_currency.apply", { to: current.to })}
          </Button>
        </div>
      ) : null}

      <ConfirmDialog
        open={confirming}
        title={t("rates.brand_currency.apply_confirm_title", {
          brand: brand.name,
        })}
        description={t("rates.brand_currency.apply_confirm_body", {
          from: brand.currency,
          to,
        })}
        isPending={run.isPending}
        onConfirm={() => run.mutate({ dryRun: false, key })}
        onCancel={() => setConfirming(false)}
      />
    </section>
  );
}

function ConvertSection({
  brand,
  others,
}: {
  brand: PlatformBrand;
  others: string[];
}) {
  const { t, format } = useLocale();
  const [target, setTarget] = useState("");
  const [overwrite, setOverwrite] = useState(false);
  const [preview, setPreview] = useState<BrandPriceConversion | null>(null);
  const [confirming, setConfirming] = useState(false);
  const to = target || others[0] || "";
  const current =
    preview && preview.target_currency === to && preview.overwrite === overwrite
      ? preview
      : null;

  const run = useMutation({
    mutationFn: (dryRun: boolean) =>
      brandCurrencyService.convertPrices(brand.uuid, {
        target_currency: to,
        overwrite,
        dry_run: dryRun,
      }),
    onSuccess: (out) => {
      setPreview(out.applied ? null : out);
      if (out.applied) {
        setConfirming(false);
        appToast.success(
          t("rates.brand_currency.converted", {
            create: out.create,
            update: out.update,
            skip: out.skip,
            currency: out.target_currency,
          }),
        );
      }
    },
    onError: (error) => {
      setConfirming(false);
      appToast.error(errorText(error, t("rates.toast.failed")));
    },
  });

  const money = (v: string | null | undefined, currency: string) =>
    v == null ? "—" : format.currency(Number(v), currency);

  return (
    <section className="space-y-3" aria-labelledby={`convert-${brand.slug}`}>
      <h3 id={`convert-${brand.slug}`} className="font-semibold">
        {t("rates.brand_currency.convert_title")}
      </h3>
      <p className="text-muted-foreground text-sm">
        {t("rates.brand_currency.convert_description", {
          currency: brand.currency,
        })}
      </p>
      <div className="flex flex-wrap items-end gap-4">
        <CurrencySelect
          id={`brand-convert-target-${brand.slug}`}
          label={t("rates.brand_currency.target")}
          value={to}
          options={others}
          onChange={setTarget}
        />
        <CheckboxField
          id={`brand-convert-overwrite-${brand.slug}`}
          checked={overwrite}
          onChange={setOverwrite}
          label={t("rates.brand_currency.overwrite")}
        />
        <Button
          type="button"
          variant="outline"
          disabled={!to || run.isPending}
          onClick={() => run.mutate(true)}
          data-testid={`brand-convert-preview-${brand.slug}`}
        >
          {t("rates.brand_currency.preview")}
        </Button>
      </div>

      {current ? (
        <div className="space-y-3 rounded-md border p-3 text-sm">
          <p>
            {t("rates.brand_currency.rate_line", {
              base: current.rate.base,
              quote: current.rate.quote,
              rate: format.number(Number(current.rate.rate), {
                maximumFractionDigits: 6,
              }),
              date: current.rate.rate_date,
            })}
          </p>
          <p data-testid="brand-convert-summary">
            {t("rates.brand_currency.convert_summary", {
              total: current.total,
              create: current.create,
              update: current.update,
              skip: current.skip,
            })}
          </p>
          <ul className="max-h-60 space-y-0.5 overflow-y-auto">
            {current.rows.slice(0, PREVIEW_ROWS).map((r) => (
              <li key={r.product_uuid}>
                <span className="font-mono text-xs" dir="ltr">
                  {r.sku}
                </span>{" "}
                {r.name}:{" "}
                {money(
                  r.source.sale_to_distributor_price,
                  current.source_currency,
                )}{" "}
                →{" "}
                {money(
                  r.converted.sale_to_distributor_price,
                  current.target_currency,
                )}{" "}
                <Badge variant="outline">
                  {t(`rates.brand_currency.action.${r.action}`)}
                </Badge>
              </li>
            ))}
            <MoreRows
              total={current.total}
              shown={Math.min(PREVIEW_ROWS, current.rows.length)}
            />
          </ul>
          <Button
            type="button"
            disabled={current.create + current.update === 0 || run.isPending}
            onClick={() => setConfirming(true)}
            data-testid={`brand-convert-apply-${brand.slug}`}
          >
            {t("rates.brand_currency.convert_apply")}
          </Button>
        </div>
      ) : null}

      <ConfirmDialog
        open={confirming}
        title={t("rates.brand_currency.convert_confirm_title")}
        description={t("rates.brand_currency.convert_confirm_body", {
          from: brand.currency,
          to,
        })}
        isPending={run.isPending}
        onConfirm={() => run.mutate(false)}
        onCancel={() => setConfirming(false)}
      />
    </section>
  );
}
