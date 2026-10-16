"use client";

import { History, Replace, Scissors } from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ReclassifyDialog } from "@/features/stock/components/reclassify-dialog";
import { SplitDialog } from "@/features/stock/components/split-dialog";
import {
  canRequestReclassification,
  canSplitRoll,
  type OperableUnit,
} from "@/features/stock/lib/stock-ops";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * Unit operations (TEC-763) on the scan card and the unit history page:
 * open the history, request a product fix (stock.write, not for fixed
 * barcodes) and split a roll (stock.write, roll_meter with meters left).
 * The backend checks that the unit is on hand at the active organization.
 */
export function UnitOperations({
  slug,
  unit,
  showHistory = false,
}: {
  slug: string;
  unit: OperableUnit;
  showHistory?: boolean;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const [dialog, setDialog] = useState<"reclass" | "split" | null>(null);
  const canWrite = can(Permission.StockWrite);
  const reclass = canWrite && canRequestReclassification(unit);
  const split = canWrite && canSplitRoll(unit);
  const history = showHistory && can(Permission.StockRead);

  if (!reclass && !split && !history) return null;

  return (
    <div className="flex flex-wrap gap-2" data-testid="unit-operations">
      {history ? (
        <Button asChild size="sm" variant="outline">
          <Link
            href={routes.tenant.stock.unit(slug, unit.barcode)}
            data-testid="unit-history-link"
          >
            <History className="size-4" />
            {t("stock.unit.history")}
          </Link>
        </Button>
      ) : null}
      {reclass && unit.product ? (
        <>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setDialog("reclass")}
            data-testid="unit-reclassify"
          >
            <Replace className="size-4" />
            {t("stock.reclass.action")}
          </Button>
          <ReclassifyDialog
            slug={slug}
            barcode={unit.barcode}
            product={unit.product}
            open={dialog === "reclass"}
            onOpenChange={(open) => setDialog(open ? "reclass" : null)}
          />
        </>
      ) : null}
      {split && unit.remaining_meters ? (
        <>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setDialog("split")}
            data-testid="unit-split"
          >
            <Scissors className="size-4" />
            {t("stock.split.action")}
          </Button>
          <SplitDialog
            slug={slug}
            barcode={unit.barcode}
            remaining={unit.remaining_meters}
            open={dialog === "split"}
            onOpenChange={(open) => setDialog(open ? "split" : null)}
          />
        </>
      ) : null}
    </div>
  );
}

/**
 * The scan card's unit operations (TEC-763): history link, product fix
 * and roll split; the tenant slug comes from the route.
 */
export function ScanUnitOperations({ unit }: { unit: OperableUnit }) {
  const params = useParams<{ slug?: string }>();
  const slug = String(params?.slug ?? "");
  if (!slug) return null;
  return <UnitOperations slug={slug} unit={unit} showHistory />;
}
