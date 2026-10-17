"use client";

import { useQuery } from "@tanstack/react-query";
import { Warehouse as WarehouseIcon } from "lucide-react";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  activeWarehouseKey,
  resolveActiveWarehouse,
} from "@/features/warehouse/lib/active-warehouse";
import {
  warehouseKeys,
  warehouseService,
} from "@/features/warehouse/services/warehouse.service";
import { useLocalStorage } from "@/hooks/use-local-storage";
import { useLocale } from "@/providers/locale-provider";

/**
 * The caller's warehouse context (TEC-769): GET /v1/warehouse/my-warehouses
 * plus the panel's local pick. A user without memberships may write in
 * every warehouse; a member only in its member warehouses (the backend
 * enforces the same, WAREHOUSE_NOT_MEMBER).
 */
export function useActiveWarehouse(slug: string, enabled: boolean) {
  const [stored, setStored] = useLocalStorage<string | null>(
    activeWarehouseKey(slug),
    null,
  );
  const mine = useQuery({
    queryKey: warehouseKeys.mine,
    queryFn: () => warehouseService.myWarehouses(),
    enabled,
    staleTime: 60_000,
    retry: false,
  });
  return {
    context: mine.data,
    activeUuid: resolveActiveWarehouse(mine.data, stored),
    setActive: (uuid: string) => setStored(uuid),
  };
}

/** Active warehouse picker of the warehouse page header. */
export function ActiveWarehouseSelect({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { context, activeUuid, setActive } = useActiveWarehouse(slug, true);
  const writable = (context?.items ?? []).filter((w) => w.member);
  if (!context || writable.length === 0) return null;
  return (
    <div className="flex items-center gap-2" data-testid="active-warehouse">
      <WarehouseIcon className="text-muted-foreground size-4" aria-hidden />
      <Select value={activeUuid ?? ""} onValueChange={setActive}>
        <SelectTrigger
          className="h-9 w-56"
          aria-label={t("warehouse.active.label")}
          data-testid="active-warehouse-select"
        >
          <SelectValue placeholder={t("warehouse.active.placeholder")} />
        </SelectTrigger>
        <SelectContent>
          {writable.map((w) => (
            <SelectItem key={w.uuid} value={w.uuid}>
              {w.code} · {w.name}
              {w.is_default ? ` (${t("warehouse.active.default")})` : ""}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {context.restricted ? (
        <span
          className="text-muted-foreground hidden text-xs xl:inline"
          data-testid="active-warehouse-restricted"
        >
          {t("warehouse.active.restricted")}
        </span>
      ) : null}
    </div>
  );
}
