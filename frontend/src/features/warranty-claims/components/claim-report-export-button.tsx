"use client";

import { useMutation } from "@tanstack/react-query";
import { FileDown } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import {
  buildExportRequest,
  CLAIM_REPORT_EXPORT_FORMATS,
  tabReportKind,
  type ClaimReportTab,
} from "@/features/warranty-claims/lib/claim-reports";
import {
  claimReportsService,
  type ClaimReportExportFormat,
  type ClaimReportPeriod,
} from "@/features/warranty-claims/services/claim-reports.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Queues the active tab's report as a CSV / XLSX export job (TEC-338
 * endpoints). The job lands in the organization's central export list;
 * the confirmation links there.
 */
export function ClaimReportExportButton({
  slug,
  tab,
  period,
  disabled,
}: {
  slug: string;
  tab: ClaimReportTab;
  period: ClaimReportPeriod;
  disabled?: boolean;
}) {
  const { t, locale } = useLocale();
  const [format, setFormat] = useState<ClaimReportExportFormat>("xlsx");
  const [queued, setQueued] = useState(false);

  const request = useMutation({
    mutationFn: () =>
      claimReportsService.requestExport(
        tabReportKind(tab),
        buildExportRequest(tab, period, format, locale),
      ),
    onSuccess: () => {
      setQueued(true);
      appToast.success(t("warranty.claim_reports.export.queued"));
    },
    onError: (err) =>
      appToast.error(
        isApiError(err)
          ? err.message
          : t("warranty.claim_reports.export.failed"),
      ),
  });

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Select
        value={format}
        onValueChange={(value) => setFormat(value as ClaimReportExportFormat)}
      >
        <SelectTrigger
          id="claim-report-export-format"
          data-testid="claim-report-export-format"
          aria-label={t("warranty.claim_reports.export.format")}
          className="w-auto"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {CLAIM_REPORT_EXPORT_FORMATS.map((f) => (
            <SelectItem key={f} value={f}>
              {f.toUpperCase()}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        type="button"
        variant="outline"
        data-testid="claim-report-export"
        disabled={disabled || request.isPending}
        onClick={() => request.mutate()}
      >
        <FileDown className="size-4" />
        {t("warranty.claim_reports.export.button")}
      </Button>
      {queued ? (
        <Link
          href={routes.tenant.exports.root(slug)}
          data-testid="claim-report-export-link"
          className="text-primary text-sm underline-offset-4 hover:underline"
        >
          {t("warranty.claim_reports.export.open_list")}
        </Link>
      ) : null}
    </div>
  );
}
