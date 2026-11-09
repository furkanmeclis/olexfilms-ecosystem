"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Percent } from "lucide-react";
import { useCallback, useRef, useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import { discountValid, normalizeAmount } from "@/features/orders/lib/form";
import {
  orderKeys,
  ordersService,
  type BuyerDiscount,
} from "@/features/orders/services/orders.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** The editable form of one buyer's discounts. */
function DiscountForm({
  value,
  onSaved,
}: {
  value: BuyerDiscount;
  onSaved: (v: BuyerDiscount) => void;
}) {
  const { t } = useLocale();
  const [standard, setStandard] = useState(value.standard_discount_pct ?? "");
  const [first, setFirst] = useState(value.first_order_discount_pct ?? "");
  const valid = discountValid(standard) && discountValid(first);
  const save = useMutation({
    mutationFn: () => {
      const s = normalizeAmount(standard);
      const f = normalizeAmount(first);
      return ordersService.setDiscount(value.organization.uuid, {
        standard_discount_pct: s === "" ? null : s,
        first_order_discount_pct: f === "" ? null : f,
      });
    },
    onSuccess: (v) => {
      onSaved(v);
      toast.success(t("orders.discounts.saved"));
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.discounts.save_failed"))),
  });
  return (
    <div className="space-y-4" data-testid="dealer-discount-form">
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="discount-standard">
            {t("orders.discounts.standard")}
          </Label>
          <Input
            id="discount-standard"
            inputMode="decimal"
            dir="ltr"
            value={standard}
            placeholder="0"
            aria-invalid={!discountValid(standard) || undefined}
            data-testid="discount-standard"
            onChange={(e) => setStandard(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            {t("orders.discounts.standard_hint")}
          </p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="discount-first">
            {t("orders.discounts.first_order")}
          </Label>
          <Input
            id="discount-first"
            inputMode="decimal"
            dir="ltr"
            value={first}
            placeholder="—"
            aria-invalid={!discountValid(first) || undefined}
            data-testid="discount-first"
            onChange={(e) => setFirst(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            {t("orders.discounts.first_order_hint")}
          </p>
        </div>
      </div>
      {!valid ? (
        <p className="text-destructive text-xs" data-testid="discount-error">
          {t("orders.form.errors.discount_invalid")}
        </p>
      ) : null}
      <p
        className="text-muted-foreground text-sm"
        data-testid="discount-first-state"
      >
        {value.first_order
          ? t("orders.discounts.state_first")
          : t("orders.discounts.state_placed")}
      </p>
      <Button
        type="button"
        disabled={!valid || save.isPending}
        data-testid="discount-save"
        onClick={() => save.mutate()}
      >
        {t("orders.discounts.save")}
      </Button>
    </div>
  );
}

/**
 * TEC-1064 (RV-5.3): the discount the center (or a distributor) agreed
 * with an organization directly under it — the standard one and the one of
 * the first order. Sales and accounting (pricing.sale.write) see and edit
 * it; the order screens apply it.
 */
export function DealerDiscountsPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const org = useActiveOrganization(slug);
  const { can } = usePermission();
  const allowed = can(permissions.pricing.saleWrite);
  const [buyer, setBuyer] = useState<ComboboxOption | null>(null);
  const names = useRef(new Map<string, string>());
  const parentUuid = org?.uuid ?? "";
  const loadBuyers = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      const items = await qc.fetchQuery({
        queryKey: orderKeys.childBuyers(parentUuid, q),
        queryFn: () => ordersService.listChildBuyers(parentUuid, q),
        staleTime: 30_000,
      });
      for (const o of items) names.current.set(o.uuid, o.name);
      return items.map((o) => ({ value: o.uuid, label: o.name }));
    },
    [parentUuid, qc],
  );
  const discount = useQuery({
    queryKey: orderKeys.discount(buyer?.value ?? ""),
    queryFn: () => ordersService.getDiscount(buyer?.value ?? ""),
    enabled: allowed && buyer !== null,
    retry: false,
  });

  const title = t("orders.discounts.title");
  return (
    <div className="space-y-6">
      <PageHeader
        title={title}
        icon={<Percent className="size-6" />}
        description={t("orders.discounts.description")}
        breadcrumbs={[
          {
            label: t("layout.breadcrumb_home"),
            href: routes.tenant.home(slug),
          },
          { label: title },
        ]}
      />
      {!allowed ? (
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("orders.discounts.forbidden")}
        />
      ) : (
        <Card>
          <CardContent className="space-y-6 pt-6">
            <div className="max-w-md space-y-2">
              <Label htmlFor="discount-buyer">
                {t("orders.discounts.buyer")}
              </Label>
              <AsyncCombobox
                id="discount-buyer"
                data-testid="discount-buyer"
                value={buyer?.value}
                initialOptions={buyer ? [buyer] : undefined}
                loadOptions={loadBuyers}
                placeholder={t("orders.discounts.buyer_placeholder")}
                searchPlaceholder={t("orders.center_form.dealer_search")}
                emptyText={t("orders.discounts.buyer_none")}
                onValueChange={(value) =>
                  setBuyer(
                    value
                      ? { value, label: names.current.get(value) ?? value }
                      : null,
                  )
                }
              />
            </div>
            {buyer && discount.isError ? (
              <ErrorState
                title={t("orders.discounts.load_failed")}
                description={orderErrorMessage(
                  discount.error,
                  t,
                  t("orders.discounts.load_failed"),
                )}
              />
            ) : null}
            {buyer && discount.data ? (
              <DiscountForm
                key={discount.data.organization.uuid}
                value={discount.data}
                onSaved={(v) => {
                  qc.setQueryData(orderKeys.discount(v.organization.uuid), v);
                  void qc.invalidateQueries({ queryKey: ["orders", "quote"] });
                }}
              />
            ) : null}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
