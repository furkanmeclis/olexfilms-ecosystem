"use client";

import { FileWarning } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { importsService } from "@/features/io/services/imports.service";
import type { ExportJobScope } from "@/features/io/types";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Set once the error report endpoint answered 404: the button stays hidden
 * for the rest of the session (the endpoint is rolled out by TEC-757).
 */
let reportEndpointMissing = false;

type ImportErrorReportButtonProps = {
  jobUuid: string;
  scope?: ExportJobScope;
  size?: "sm" | "default";
};

/** Downloads the XLSX of the rows that are not imported, with their errors. */
export function ImportErrorReportButton({
  jobUuid,
  scope = "platform",
  size = "sm",
}: ImportErrorReportButtonProps) {
  const { t } = useLocale();
  const [hidden, setHidden] = useState(reportEndpointMissing);
  const [pending, setPending] = useState(false);

  if (hidden) return null;

  const download = async () => {
    setPending(true);
    try {
      const report = await importsService.errorReport(jobUuid, scope);
      if (report.kind === "missing") {
        reportEndpointMissing = true;
        setHidden(true);
        return;
      }
      triggerBrowserDownload(
        report.blob,
        report.filename ?? `import-errors-${jobUuid.slice(0, 8)}.xlsx`,
      );
    } catch {
      appToast.error(t("imports.toast.error_report_failed"));
    } finally {
      setPending(false);
    }
  };

  return (
    <Button
      type="button"
      variant="outline"
      size={size}
      disabled={pending}
      onClick={() => void download()}
      data-testid="import-error-report"
    >
      <FileWarning className="size-4" />
      {t("imports.error_report")}
    </Button>
  );
}
