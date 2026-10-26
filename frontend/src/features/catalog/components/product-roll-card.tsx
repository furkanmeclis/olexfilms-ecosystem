"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { catalogKeys } from "@/features/catalog/hooks/use-catalog-access";
import type { components } from "@/generated/api";
import { isApiError } from "@/lib/api";
import { platformRequest } from "@/lib/api/platform-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Product = components["schemas"]["CatalogProduct"];
type Roll = components["schemas"]["CatalogRollSettings"];

const FIELDS = [
  "default_meters",
  "width_cm",
  "min_cut_meters",
  "waste_pct",
] as const;

const toText = (v: number | null | undefined) =>
  v === null || v === undefined ? "" : String(v);

const toNumber = (v: string): number | null => {
  const s = v.trim().replace(",", ".");
  if (s === "") return null;
  const n = Number(s);
  return Number.isFinite(n) ? n : NaN;
};

/**
 * TEC-1008 roll defaults of a product: the default roll length a box
 * counted as a piece becomes in bulk conversion, the width, the minimum
 * cut and the waste allowance. Any product may hold rolls and pieces at
 * the same time (a unit is a roll when it carries meters).
 */
export function ProductRollCard({
  product,
  canWrite,
}: {
  product: Product;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [values, setValues] = useState<Record<(typeof FIELDS)[number], string>>(
    {
      default_meters: toText(product.roll.default_meters),
      width_cm: toText(product.roll.width_cm),
      min_cut_meters: toText(product.roll.min_cut_meters),
      waste_pct: toText(product.roll.waste_pct),
    },
  );
  const save = useMutation({
    mutationFn: (body: Roll) =>
      platformRequest<Product>(
        "PUT",
        `/v1/catalog/products/${encodeURIComponent(product.uuid)}/roll`,
        { body },
      ),
    onSuccess: (p) => {
      qc.setQueryData(catalogKeys.product(p.uuid), p);
      appToast.success(t("catalog.roll.saved"));
    },
    onError: (e: unknown) =>
      appToast.error(isApiError(e) ? e.message : t("catalog.roll.failed")),
  });
  const submit = () => {
    const body = {} as Roll;
    for (const f of FIELDS) {
      const n = toNumber(values[f]);
      if (Number.isNaN(n)) {
        appToast.error(t("catalog.roll.invalid"));
        return;
      }
      body[f] = n;
    }
    save.mutate(body);
  };
  return (
    <Card data-testid="product-roll">
      <CardHeader>
        <CardTitle>{t("catalog.roll.title")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-muted-foreground text-sm">
          {t("catalog.roll.hint")}
        </p>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {FIELDS.map((f) => (
            <div key={f} className="space-y-1.5">
              <Label htmlFor={`roll-${f}`}>{t(`catalog.roll.${f}`)}</Label>
              <Input
                id={`roll-${f}`}
                inputMode="decimal"
                dir="ltr"
                value={values[f]}
                disabled={!canWrite}
                onChange={(e) =>
                  setValues((v) => ({ ...v, [f]: e.target.value }))
                }
              />
            </div>
          ))}
        </div>
        {canWrite ? (
          <Button type="button" disabled={save.isPending} onClick={submit}>
            {t("catalog.roll.save")}
          </Button>
        ) : null}
      </CardContent>
    </Card>
  );
}
