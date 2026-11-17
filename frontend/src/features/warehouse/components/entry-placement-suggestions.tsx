"use client";

import { useQuery } from "@tanstack/react-query";
import { Lightbulb } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  warehouseService,
  type StockEntrySuggestedLocation,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * TEC-1230: location suggestions for the unplaced lines of a draft entry —
 * where the warehouse already keeps the product, then empty locations.
 * A click places the selected (or all unplaced) lines there.
 */
export function EntryPlacementSuggestions({
  entryUuid,
  unplaced,
  busy,
  onPick,
}: {
  entryUuid: string;
  unplaced: number;
  busy: boolean;
  onPick: (locationUuid: string) => void;
}) {
  const { t } = useLocale();
  const query = useQuery({
    queryKey: [
      "warehouse",
      "entry",
      entryUuid,
      "placement-suggestions",
      unplaced,
    ],
    queryFn: () => warehouseService.placementSuggestions(entryUuid),
    enabled: unplaced > 0,
    staleTime: 30_000,
  });
  const data = query.data;
  if (!data || unplaced === 0) return null;
  const withStock = data.products.filter((p) => p.locations.length > 0);
  if (withStock.length === 0 && data.empty_locations.length === 0) return null;

  const chip = (l: StockEntrySuggestedLocation) => (
    <Button
      key={l.uuid}
      type="button"
      size="sm"
      variant="outline"
      disabled={busy}
      onClick={() => onPick(l.uuid)}
      data-testid={`entry-suggestion-${l.uuid}`}
      dir="ltr"
    >
      {l.full_code ?? l.code}
      {l.quantity != null ? (
        <span className="text-muted-foreground ms-1 tabular-nums">
          ({l.quantity})
        </span>
      ) : null}
    </Button>
  );

  return (
    <div
      className="space-y-2 rounded-md border p-3 lg:col-span-2"
      data-testid="entry-placement-suggestions"
    >
      <p className="flex items-center gap-2 text-sm font-medium">
        <Lightbulb className="size-4" />
        {t("warehouse.entry.suggestions.title")}
      </p>
      {withStock.map((p) => (
        <div key={p.product.uuid} className="flex flex-wrap items-center gap-2">
          <span className="text-muted-foreground text-xs">
            {t("warehouse.entry.suggestions.same_product", {
              product: p.product.name,
            })}
          </span>
          {p.locations.map(chip)}
        </div>
      ))}
      {data.empty_locations.length > 0 ? (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-muted-foreground text-xs">
            {t("warehouse.entry.suggestions.empty")}
          </span>
          {data.empty_locations.map(chip)}
        </div>
      ) : null}
    </div>
  );
}
