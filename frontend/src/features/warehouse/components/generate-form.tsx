"use client";

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";

import { AppForm, AppInput } from "@/components/forms";
import { FormFieldShell } from "@/components/forms/form-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AsyncCombobox } from "@/components/ui/async-combobox";
import {
  catalogService,
  type CatalogProduct,
} from "@/features/catalog/services/catalog.service";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  BATCH_MAX,
  generateBody,
  generateLinesSchema,
  type GenerateLinesValues,
} from "@/features/warehouse/lib/forms";
import { useDebounce } from "@/hooks/use-debounce";
import { useLocale } from "@/providers/locale-provider";

export type GenerateBody = ReturnType<typeof generateBody>;

/**
 * Reserve N new barcodes of one product (TEC-202, center only, K14): used
 * by a generate_new stock entry and by the barcode batches page. Rolls
 * need the length of each roll; the prefix defaults to the brand's.
 */
export function GenerateForm({
  submitLabel,
  onSubmit,
  pending,
}: {
  submitLabel: string;
  onSubmit: (body: GenerateBody) => Promise<unknown>;
  pending?: boolean;
}) {
  const { t } = useLocale();
  const [product, setProduct] = useState<CatalogProduct | null>(null);
  const [error, setError] = useState<string | null>(null);
  const isRoll = product?.unit_type === "roll_meter";

  return (
    <AppForm<GenerateLinesValues>
      key={isRoll ? "roll" : "piece"}
      schema={generateLinesSchema(t, isRoll) as never}
      defaultValues={{
        product_uuid: product?.uuid ?? "",
        quantity: "1",
        meters: "",
        prefix: "",
      }}
      onSubmit={async (v) => {
        setError(null);
        try {
          await onSubmit(generateBody(v, isRoll));
        } catch (err) {
          setError(warehouseErrorMessage(err, t, t("warehouse.form.error")));
        }
      }}
      className="space-y-4"
    >
      <ProductField product={product} onProduct={setProduct} />
      <div className="grid gap-4 sm:grid-cols-3">
        <AppInput
          name="quantity"
          inputMode="numeric"
          label={t("warehouse.fields.quantity")}
          description={t("warehouse.generate.quantity_hint", {
            max: BATCH_MAX,
          })}
          data-testid="generate-quantity"
        />
        {isRoll ? (
          <AppInput
            name="meters"
            inputMode="decimal"
            label={t("warehouse.fields.meters")}
            description={t("warehouse.generate.meters_hint")}
            data-testid="generate-meters"
            dir="ltr"
          />
        ) : null}
        <AppInput
          name="prefix"
          label={t("warehouse.fields.prefix")}
          description={t("warehouse.generate.prefix_hint")}
          data-testid="generate-prefix"
          dir="ltr"
        />
      </div>
      {error ? (
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      ) : null}
      <div className="flex justify-end">
        <Button type="submit" disabled={pending} data-testid="generate-submit">
          {submitLabel}
        </Button>
      </div>
    </AppForm>
  );
}

function ProductField({
  product,
  onProduct,
}: {
  product: CatalogProduct | null;
  onProduct: (p: CatalogProduct | null) => void;
}) {
  const { t } = useLocale();
  const {
    setValue,
    control,
    formState: { errors },
  } = useFormContext<GenerateLinesValues>();
  const value = useWatch({ control, name: "product_uuid" });
  const [q, setQ] = useState("");
  const query = useDebounce(q, 300);
  const products = useQuery({
    queryKey: ["warehouse", "catalog-products", query],
    queryFn: () =>
      catalogService.listProducts({
        q: query || undefined,
        active: true,
        limit: 50,
      }),
  });
  const items = products.data?.items ?? [];
  const options =
    product && !items.some((p) => p.uuid === product.uuid)
      ? [product, ...items]
      : items;
  const error = errors.product_uuid?.message as string | undefined;

  return (
    <FormFieldShell
      name="product_uuid"
      label={t("warehouse.fields.product")}
      error={error}
    >
      <div className="grid gap-2 sm:grid-cols-[14rem_1fr]">
        <Input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={t("warehouse.generate.product_search")}
          aria-label={t("warehouse.generate.product_search")}
          data-testid="generate-product-search"
        />
        <AsyncCombobox
          id="product_uuid"
          data-testid="generate-product"
          value={value ?? ""}
          aria-invalid={Boolean(error)}
          placeholder={t("warehouse.generate.pick_product")}
          options={options.map((p) => ({
            value: p.uuid,
            label: `${p.sku} · ${p.name}${
              p.unit_type === "roll_meter"
                ? ` (${t("warehouse.unit_type.roll_meter")})`
                : ""
            }`,
          }))}
          onValueChange={(next) => {
            const picked = options.find((p) => p.uuid === next) ?? null;
            setValue("product_uuid", next, { shouldValidate: true });
            onProduct(picked);
          }}
        />
      </div>
    </FormFieldShell>
  );
}
