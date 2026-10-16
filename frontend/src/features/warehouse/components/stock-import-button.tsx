"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { FileSpreadsheet } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { ImportWizard } from "@/features/io/components/import-wizard";
import type { ImportJob } from "@/features/io/types";
import {
  warehouseKeys,
  warehouseService,
  type StockEntryListQuery,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * Warehouse > Stock entries: "Import from Excel" (TEC-759). Opens the
 * import wizard on the stock import (opening stock, bulk barcodes) of the
 * active center; the result step links the import entry it recorded.
 * The caller shows it only when `canImportStock` allows.
 */
export function StockImportButton({ slug }: { slug: string }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button
        type="button"
        variant="outline"
        onClick={() => setOpen(true)}
        data-testid="stock-import-open"
      >
        <FileSpreadsheet className="size-4" />
        {t("warehouse.entries.import")}
      </Button>
      <ImportWizard
        resource="tenant.stock.units"
        scope="tenant"
        open={open}
        onOpenChange={setOpen}
        title={t("warehouse.entries.import_title")}
        jobsHref={routes.tenant.imports.root(slug)}
        onComplete={() =>
          void qc.invalidateQueries({ queryKey: ["warehouse", "entries"] })
        }
        resultAction={(job) => <ImportedEntryLink slug={slug} job={job} />}
      />
    </>
  );
}

/** Link to the stock entry an applied import recorded (mode import). */
function ImportedEntryLink({ slug, job }: { slug: string; job: ImportJob }) {
  const { t } = useLocale();
  const query: StockEntryListQuery = {
    limit: 1,
    offset: 0,
    mode: "import",
    import_job_uuid: job.uuid,
  };
  const entries = useQuery({
    queryKey: [...warehouseKeys.entries(query), job.status],
    queryFn: () => warehouseService.listEntries(query),
  });
  const entry = entries.data?.items[0];
  if (!entry) return null;
  return (
    <Button asChild size="sm">
      <Link
        href={routes.tenant.warehouse.entry(slug, entry.uuid)}
        data-testid="import-result-entry"
      >
        {t("warehouse.entries.open_import_entry")}
      </Link>
    </Button>
  );
}
