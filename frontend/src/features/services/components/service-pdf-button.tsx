"use client";

import { FileDown, Loader2 } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { servicePdfClient } from "@/features/services/services/service-pdf.service";
import {
  fetchCertificate,
  type CertificateClient,
  type WaitOptions,
} from "@/features/warranty/lib/certificate";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * "PDF" on the service detail (TEC-196): queues the service PDF, waits for
 * the export job and downloads it in the user language. `client` swaps the
 * realm (the portal passes its own, TEC-241); the panel client is default.
 */
export function ServicePdfButton({
  serviceUuid,
  serviceNo,
  locale,
  waitOptions,
  client,
}: {
  client?: CertificateClient;
  serviceUuid: string;
  serviceNo: string;
  locale?: string;
  waitOptions?: WaitOptions;
}) {
  const { t } = useLocale();
  const [busy, setBusy] = useState(false);

  const onClick = async () => {
    setBusy(true);
    try {
      const { file } = await fetchCertificate(
        client ?? servicePdfClient(serviceUuid),
        locale,
        waitOptions,
      );
      triggerBrowserDownload(file.blob, file.filename ?? `${serviceNo}.pdf`);
      appToast.success(t("services.detail.pdf_ready"));
    } catch {
      appToast.error(t("services.detail.pdf_failed"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Button
      type="button"
      variant="outline"
      disabled={busy}
      aria-busy={busy}
      onClick={() => void onClick()}
      data-testid="service-pdf"
    >
      {busy ? (
        <Loader2 className="size-4 animate-spin" />
      ) : (
        <FileDown className="size-4" />
      )}
      {busy ? t("services.detail.pdf_preparing") : t("services.detail.pdf")}
    </Button>
  );
}
