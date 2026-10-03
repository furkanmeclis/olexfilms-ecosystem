"use client";

import { useMutation } from "@tanstack/react-query";
import { ScanLine } from "lucide-react";
import { useState } from "react";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ScanInput } from "@/features/warehouse/components/scan-input";
import { ScanResultCard } from "@/features/warehouse/components/scan-result-card";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  warehouseService,
  type WarehouseScanResult,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

type HistoryEntry = {
  id: number;
  code: string;
  result: WarehouseScanResult | null;
  error: string | null;
};

const HISTORY_MAX = 10;

/**
 * Warehouse > Scan (TEC-203): one field for every code — location QR,
 * unit QR or barcode, SKU, short code — and what it resolved to, with the
 * last scans of this session.
 */
export function ScanPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const access = useWarehouseAccess(slug);
  const [history, setHistory] = useState<HistoryEntry[]>([]);
  const [seq, setSeq] = useState(0);

  const scan = useMutation({
    mutationFn: (code: string) => warehouseService.scan(code),
  });

  const onScan = async (code: string) => {
    const id = seq + 1;
    setSeq(id);
    let entry: HistoryEntry;
    try {
      const result = await scan.mutateAsync(code);
      entry = { id, code, result, error: null };
    } catch (err) {
      entry = {
        id,
        code,
        result: null,
        error: warehouseErrorMessage(err, t, t("warehouse.scan.failed")),
      };
    }
    setHistory((prev) => [entry, ...prev].slice(0, HISTORY_MAX));
  };

  const [latest, ...older] = history;

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.scan.title")}
      description={t("warehouse.scan.description")}
      icon={<ScanLine className="size-6" />}
    >
      <Card>
        <CardContent className="pt-6">
          <ScanInput onScan={onScan} busy={scan.isPending} />
        </CardContent>
      </Card>

      {latest ? (
        latest.result ? (
          <ScanResultCard result={latest.result} />
        ) : (
          <Card data-testid="scan-error">
            <CardContent className="pt-6">
              <p className="font-mono text-sm" dir="ltr">
                {latest.code}
              </p>
              <p role="alert" className="text-destructive text-sm">
                {latest.error}
              </p>
            </CardContent>
          </Card>
        )
      ) : null}

      {older.length > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("warehouse.scan.history")}</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="divide-y text-sm" data-testid="scan-history">
              {older.map((h) => (
                <li key={h.id} className="flex items-center gap-3 py-1.5">
                  <span className="font-mono" dir="ltr">
                    {h.code}
                  </span>
                  <span className="text-muted-foreground">
                    {h.result
                      ? t(`warehouse.scan.type.${h.result.type}`)
                      : h.error}
                  </span>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      ) : null}
    </WarehouseShell>
  );
}
