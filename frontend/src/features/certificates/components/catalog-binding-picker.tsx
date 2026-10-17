"use client";

import { useCallback, useState } from "react";

import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { useLocale } from "@/providers/locale-provider";

/** Brand-scoped catalog search; selected labels survive subsequent search pages. */
export function CatalogBindingPicker({
  id,
  resource,
  value,
  onChange,
}: {
  id: string;
  resource: "categories" | "products";
  value: string[];
  onChange: (value: string[]) => void;
}) {
  const { t } = useLocale();
  const [catalog, setCatalog] = useState<ComboboxOption[]>([]);
  const loadOptions = useCallback(
    async (q: string) => {
      const params = { q, limit: 20, offset: 0 };
      const page =
        resource === "categories"
          ? await catalogService.listCategories(params)
          : await catalogService.listProducts(params);
      const options = page.items.map((item) => ({
        value: item.uuid,
        label: item.name,
      }));
      setCatalog((prev) => [
        ...new Map([...prev, ...options].map((o) => [o.value, o])).values(),
      ]);
      return options;
    },
    [resource],
  );

  return (
    <div className="space-y-2">
      <AsyncCombobox
        id={id}
        aria-label={t(
          `certificates.fields.${resource === "categories" ? "category_uuids" : "product_uuids"}`,
        )}
        loadOptions={loadOptions}
        selectedValues={value}
        hideSelected
        onValueChange={(uuid) => {
          if (uuid && !value.includes(uuid)) onChange([...value, uuid]);
        }}
      />
      {value.map((uuid) => {
        const label = catalog.find((item) => item.value === uuid)?.label;
        return (
          <div
            key={uuid}
            className="flex items-center justify-between gap-2 rounded-md border px-2 py-1 text-sm"
          >
            <span>{label}</span>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              aria-label={`${t("common.delete")} ${label ?? ""}`}
              onClick={() => onChange(value.filter((item) => item !== uuid))}
            >
              {t("common.delete")}
            </Button>
          </div>
        );
      })}
    </div>
  );
}
