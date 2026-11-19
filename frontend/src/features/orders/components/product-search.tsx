"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import { Package, Plus, Ruler, Search } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  catalogService,
  type CatalogProduct,
} from "@/features/catalog/services/catalog.service";
import {
  isRoll,
  lineKey,
  productMeasures,
  type PickedProduct,
  type SaleMeasure,
} from "@/features/orders/lib/form";
import { orderKeys } from "@/features/orders/services/orders.service";
import { useDebounce } from "@/hooks/use-debounce";
import { useLocale } from "@/providers/locale-provider";

/** Products per page of the order product picker (TEC-1052). */
export const PRODUCT_PAGE_SIZE = 20;

/** TEC-1239: the order form view of a catalog product. */
export function pickedProduct(p: CatalogProduct): PickedProduct {
  const roll = p.roll;
  return {
    uuid: p.uuid,
    sku: p.sku,
    name: p.name,
    unit_type: p.unit_type,
    sellable_by_meter: roll?.sellable_by_meter === true,
    box_meters: roll?.default_meters ?? null,
    min_cut: roll?.min_cut_meters ?? null,
    usage_credits: p.usage_credits ?? null,
  };
}

/**
 * Brand catalog product picker of the order forms (TEC-170, TEC-1024).
 * TEC-1052: the active products are listed without typing (by name, paged
 * with "show more"); a search narrows the list. TEC-1239: a product sold
 * by the meter offers +Box and +Meter (one line per product and measure,
 * K10); every other product keeps the single add button.
 */
export function ProductSearch({
  onPick,
  picked,
}: {
  onPick: (p: PickedProduct, measure: SaleMeasure) => void;
  /** Line keys (product + measure) already on the form. */
  picked: Set<string>;
}) {
  const { t } = useLocale();
  const [q, setQ] = useState("");
  const term = useDebounce(q.trim(), 300);
  const products = useInfiniteQuery({
    queryKey: orderKeys.products(term),
    queryFn: ({ pageParam }) =>
      catalogService.listProducts({
        ...(term ? { q: term } : {}),
        active: true,
        limit: PRODUCT_PAGE_SIZE,
        offset: pageParam,
      }),
    initialPageParam: 0,
    getNextPageParam: (last) => {
      const next = last.offset + last.items.length;
      return last.items.length > 0 && next < last.total ? next : undefined;
    },
  });
  const items = products.data?.pages.flatMap((p) => p.items) ?? [];
  const total = products.data?.pages[0]?.total ?? 0;
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
      {products.isLoading ? (
        <p className="text-muted-foreground text-sm">{t("orders.loading")}</p>
      ) : items.length === 0 ? (
        <p className="text-muted-foreground text-sm" data-testid="product-none">
          {t("orders.form.product_none")}
        </p>
      ) : (
        <>
          <ul
            className="max-h-96 divide-y overflow-y-auto rounded-lg border"
            data-testid="product-options"
          >
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
                      : p.roll?.sellable_by_meter
                        ? t("orders.unit.box_and_meters")
                        : t("orders.unit.pieces")}
                  </div>
                </div>
                <div className="flex shrink-0 flex-wrap justify-end gap-1">
                  {productMeasures(pickedProduct(p)).map((measure, _, all) => {
                    const added = picked.has(
                      lineKey({
                        product_uuid: p.uuid,
                        unit_type: p.unit_type,
                        sale_measure: measure,
                      }),
                    );
                    const both = all.length > 1;
                    return (
                      <Button
                        key={measure}
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={added}
                        data-testid={
                          both ? `product-add-${measure}` : "product-add"
                        }
                        onClick={() => onPick(pickedProduct(p), measure)}
                      >
                        {both ? (
                          measure === "meter" ? (
                            <Ruler className="size-4" />
                          ) : (
                            <Package className="size-4" />
                          )
                        ) : (
                          <Plus className="size-4" />
                        )}
                        {added
                          ? t("orders.form.product_added")
                          : both
                            ? t(`orders.form.product_add_${measure}`)
                            : t("orders.form.product_add")}
                      </Button>
                    );
                  })}
                </div>
              </li>
            ))}
          </ul>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p
              className="text-muted-foreground text-xs"
              data-testid="product-count"
            >
              {t("orders.form.product_count", {
                shown: items.length,
                total,
              })}
            </p>
            {products.hasNextPage ? (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={products.isFetchingNextPage}
                data-testid="product-more"
                onClick={() => void products.fetchNextPage()}
              >
                {t("orders.form.product_more")}
              </Button>
            ) : null}
          </div>
        </>
      )}
    </div>
  );
}
