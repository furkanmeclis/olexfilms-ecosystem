"use client";

import { FileSpreadsheet } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { ImportWizard } from "@/features/io/components/import-wizard";
import { warehouseLocationConfirmCopy } from "@/features/warehouse/lib/import";
import {
  bffUrl,
  locationLabelsPath,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

export function LocationImportButton({
  warehouseUuid,
  onComplete,
}: {
  warehouseUuid: string;
  onComplete: () => void;
}) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => setOpen(true)}
      >
        <FileSpreadsheet className="size-4" />
        {t("warehouse.import.title")}
      </Button>
      <ImportWizard
        resource="tenant.warehouse.locations"
        scope="tenant"
        open={open}
        onOpenChange={setOpen}
        paths={{
          upload: `/v1/warehouse/warehouses/${warehouseUuid}/locations/import`,
          sample: `/v1/warehouse/warehouses/${warehouseUuid}/locations/import/sample`,
        }}
        fixedDefaults={{ warehouse_uuid: warehouseUuid }}
        confirmCopy={warehouseLocationConfirmCopy}
        resultLink={(job) =>
          (job.preview_summary?.counts?.applied ?? 0) +
            (job.preview_summary?.counts?.exists ?? 0) >
          0
            ? {
                href: bffUrl(locationLabelsPath({ warehouse: warehouseUuid })),
                label: t("warehouse.import.print_labels"),
              }
            : null
        }
        onComplete={onComplete}
        title={t("warehouse.import.title")}
      />
    </>
  );
}
