"use client";

import { useQuery } from "@tanstack/react-query";
import { Plus, Search } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { isRoll } from "@/features/orders/lib/form";
import { orderKeys } from "@/features/orders/services/orders.service";
import { useDebounce } from "@/hooks/use-debounce";
import { useLocale } from "@/providers/locale-provider";

/** Brand catalog product search of the order forms (TEC-170, TEC-1024). */
export function ProductSearch({
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
