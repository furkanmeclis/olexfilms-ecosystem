"use client";

import { useQuery } from "@tanstack/react-query";
import { Info } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { CarPartPicker } from "@/features/services/components/car-part-picker";
import {
  BODY_PARTS,
  WINDOW_PARTS,
  availablePartsOf,
} from "@/features/services/lib/car-parts";
import {
  serviceWizardKeys,
  type Service,
} from "@/features/services/services/service-wizard.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const ALL_KNOWN = [...BODY_PARTS, ...WINDOW_PARTS];

export type PartsStepProps = {
  service: Service;
  selected: readonly string[];
  onChange: (parts: string[]) => void;
  onBack: () => void;
  onNext: () => void;
};

/**
 * Step 2: package and parts. The pickable parts are the available_parts of
 * the brand's active product categories (TEC-145), optionally narrowed to
 * one category. The selection is applied to each item added in the stock
 * step, limited to that product's category (TEC-179 applied_parts).
 */
export function PartsStep({
  service,
  selected,
  onChange,
  onBack,
  onNext,
}: PartsStepProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canReadCatalog = can(permissions.catalog.read);
  const [category, setCategory] = useState<string | null>(null);

  const categories = useQuery({
    queryKey: serviceWizardKeys.categories,
    queryFn: () => catalogService.listCategories({ active: true, limit: 100 }),
    enabled: canReadCatalog,
    staleTime: 60_000,
  });

  if (canReadCatalog && categories.isLoading) return <Loading />;
  if (canReadCatalog && categories.isError) {
    return (
      <ErrorState
        title={t("services.parts.load_failed")}
        onRetry={() => void categories.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  const list = (categories.data?.items ?? []).filter(
    (c) => (c.available_parts ?? []).length > 0,
  );
  const chosen = list.find((c) => c.uuid === category) ?? null;
  const available = !canReadCatalog
    ? ALL_KNOWN
    : availablePartsOf(chosen ? [chosen] : list);
  const editable = service.items_editable;
  const itemCount = service.items?.length ?? 0;

  return (
    <div className="space-y-6" data-testid="parts-step">
      {list.length > 1 ? (
        <div
          className="flex flex-wrap items-center gap-2"
          role="group"
          aria-label={t("services.parts.category")}
        >
          <span className="text-sm font-medium">
            {t("services.parts.category")}
          </span>
          <Button
            type="button"
            size="sm"
            variant={chosen ? "outline" : "default"}
            aria-pressed={!chosen}
            onClick={() => setCategory(null)}
          >
            {t("services.parts.all_categories")}
          </Button>
          {list.map((c) => (
            <Button
              key={c.uuid}
              type="button"
              size="sm"
              variant={chosen?.uuid === c.uuid ? "default" : "outline"}
              aria-pressed={chosen?.uuid === c.uuid}
              data-category={c.uuid}
              onClick={() => setCategory(c.uuid)}
            >
              {c.name}
            </Button>
          ))}
        </div>
      ) : null}

      {canReadCatalog && available.length === 0 ? (
        <Alert>
          <Info />
          <AlertDescription>
            {t("services.parts.none_available")}
          </AlertDescription>
        </Alert>
      ) : (
        <CarPartPicker
          available={available}
          selected={selected}
          onChange={onChange}
          disabled={!editable}
        />
      )}

      <p
        className="text-muted-foreground text-sm"
        aria-live="polite"
        data-testid="parts-count"
      >
        {t("services.parts.selected_count", { count: selected.length })}
      </p>
      <Alert>
        <Info />
        <AlertDescription>
          {itemCount > 0
            ? t("services.parts.hint_existing_items")
            : t("services.parts.hint")}
        </AlertDescription>
      </Alert>

      <div className="flex flex-wrap justify-between gap-2">
        <Button type="button" variant="outline" onClick={onBack}>
          {t("services.wizard.back")}
        </Button>
        <Button type="button" data-testid="parts-next" onClick={onNext}>
          {t("services.wizard.next")}
        </Button>
      </div>
    </div>
  );
}
