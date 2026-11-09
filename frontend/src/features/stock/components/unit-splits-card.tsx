"use client";

import { useQuery } from "@tanstack/react-query";
import { Scissors } from "lucide-react";
import Link from "next/link";

import { StatusChip } from "@/components/common/status-chip";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { routes } from "@/config/routes";
import {
  stockKeys,
  stockService,
  type StockUnitRow,
} from "@/features/stock/services/stock.service";
import type { components } from "@/generated/api";
import { useLocale } from "@/providers/locale-provider";

type Schemas = components["schemas"];
export type UnitSplits = Schemas["StockUnitSplits"];
export type SplitFamilyStatus = Schemas["StockSplitFamilyStatus"];

/** One family member as the part list shows it (history or family list). */
export type SplitPartItem = {
  uuid: string;
  barcode: string;
  depth: number;
  /** Meters cut as this part; null for the roll itself. */
  meters: string | null;
  remainingMeters: string | null;
  familyStatus: SplitFamilyStatus;
  holderName: string | null;
  order: { uuid: string; order_no: string } | null;
};

export function familyStatusTone(
  status: SplitFamilyStatus,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "on_hand":
      return "success";
    case "sold":
    case "in_transit":
      return "warning";
    case "used":
      return "danger";
    default:
      return "default";
  }
}

/** History splits -> list items. */
export function splitPartItems(splits: UnitSplits): SplitPartItem[] {
  return splits.parts.map((p) => ({
    uuid: p.uuid,
    barcode: p.barcode,
    depth: p.depth,
    meters: p.meters,
    remainingMeters: p.remaining_meters,
    familyStatus: p.family_status,
    holderName: p.holder?.name ?? null,
    order: p.order,
  }));
}

/**
 * TEC-1078: the parts of a split family — barcode (<roll>-3M), meters cut
 * and left, where it is now (on hand, sold to …, used) and the order that
 * carried it. A nested part is indented by its depth.
 */
export function SplitPartList({
  items,
  slug,
}: {
  items: readonly SplitPartItem[];
  slug: string;
}) {
  const { t } = useLocale();
  return (
    <ul className="space-y-2" data-testid="split-parts">
      {items.map((p) => (
        <li
          key={p.uuid}
          className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm"
          style={{ paddingInlineStart: `${Math.max(p.depth - 1, 0)}rem` }}
          data-testid="split-part"
        >
          <Link
            href={routes.tenant.stock.unit(slug, p.barcode)}
            className="font-mono font-medium hover:underline"
            dir="ltr"
          >
            {p.barcode}
          </Link>
          {p.meters ? (
            <span className="tabular-nums">
              {t("stock.splits.meters", {
                meters: p.meters,
                remaining: p.remainingMeters ?? "0",
              })}
            </span>
          ) : p.remainingMeters ? (
            <span className="tabular-nums">
              {t("stock.splits.roll_left", { remaining: p.remainingMeters })}
            </span>
          ) : null}
          <StatusChip
            label={t(`stock.family_status.${p.familyStatus}`)}
            tone={familyStatusTone(p.familyStatus)}
          />
          {p.holderName && p.familyStatus !== "on_hand" ? (
            <span className="text-muted-foreground">{p.holderName}</span>
          ) : null}
          {p.order ? (
            <Link
              href={routes.tenant.orders.detail(slug, p.order.uuid)}
              className="text-muted-foreground font-mono text-xs hover:underline"
              dir="ltr"
            >
              {p.order.order_no}
            </Link>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

/**
 * Unit history "Parts" card (TEC-1078): the roll this unit was cut from
 * and every part cut from it. Hidden when the unit has neither.
 */
export function UnitSplitsCard({
  splits,
  slug,
}: {
  splits: UnitSplits;
  slug: string;
}) {
  const { t } = useLocale();
  if (!splits.source && splits.parts.length === 0) return null;
  return (
    <Card data-testid="unit-splits">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Scissors className="size-4" />
          {t("stock.splits.title", { count: splits.parts.length })}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        {splits.source ? (
          <p data-testid="split-source">
            <span className="text-muted-foreground">
              {t("stock.splits.source")}:{" "}
            </span>
            <Link
              href={routes.tenant.stock.unit(slug, splits.source.barcode)}
              className="font-mono hover:underline"
              dir="ltr"
            >
              {splits.source.barcode}
            </Link>{" "}
            <span className="text-muted-foreground tabular-nums">
              ({t("stock.splits.cut", { meters: splits.source.meters })})
            </span>
          </p>
        ) : null}
        {splits.parts.length > 0 ? (
          <SplitPartList items={splitPartItems(splits)} slug={slug} />
        ) : (
          <p className="text-muted-foreground">{t("stock.splits.empty")}</p>
        )}
      </CardContent>
    </Card>
  );
}

/** Family list rows -> list items (the roll itself at depth 0). */
export function familyPartItems(
  rows: readonly StockUnitRow[],
): SplitPartItem[] {
  return rows.map((r) => ({
    uuid: r.uuid,
    barcode: r.barcode,
    depth: r.family_depth ?? 0,
    meters: r.split_meters ?? null,
    remainingMeters: r.remaining_meters,
    familyStatus: r.family_status ?? "other",
    holderName: r.holder?.name ?? null,
    order: r.order ?? null,
  }));
}

/**
 * Stock list search (TEC-1078): searching a roll code shows the roll and
 * every part cut from it — on hand, sold to a dealer or used — above the
 * on-hand table (`family=<barcode>`). Nothing shows for a code without
 * parts.
 */
export function StockFamilyCard({
  orgUuid,
  barcode,
  slug,
}: {
  orgUuid: string;
  barcode: string;
  slug: string;
}) {
  const { t } = useLocale();
  const code = barcode.trim();
  const family = useQuery({
    queryKey: stockKeys.family(orgUuid, code),
    queryFn: () => stockService.listFamily(orgUuid, code),
    enabled: orgUuid !== "" && code.length >= 3,
    staleTime: 30_000,
  });
  const rows = family.data?.items ?? [];
  if (!rows.some((r) => (r.family_depth ?? 0) > 0)) return null;
  return (
    <Card data-testid="stock-family">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Scissors className="size-4" />
          {t("stock.family.title", { barcode: rows[0]?.barcode ?? code })}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        <p className="text-muted-foreground text-xs">
          {t("stock.family.description")}
        </p>
        <SplitPartList items={familyPartItems(rows)} slug={slug} />
      </CardContent>
    </Card>
  );
}
