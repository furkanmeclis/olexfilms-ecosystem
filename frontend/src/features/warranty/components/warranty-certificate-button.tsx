"use client";

import { Loader2, ShieldCheck } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  certificateFilename,
  fetchCertificate,
  isNoActiveWarranty,
  type CertificateClient,
  type WaitOptions,
} from "@/features/warranty/lib/certificate";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Queues the certificate, waits for the export job and downloads the PDF;
 * also the "Warranty PDF" row action of the warranty list (TEC-378).
 */
export async function downloadWarrantyCertificate({
  client,
  locale,
  waitOptions,
  t,
}: {
  client: CertificateClient;
  locale?: string;
  waitOptions?: WaitOptions;
  t: (key: string) => string;
}): Promise<void> {
  try {
    const { job, file } = await fetchCertificate(client, locale, waitOptions);
    triggerBrowserDownload(file.blob, certificateFilename(file, job));
    appToast.success(t("warranty.certificate.ready"));
  } catch (error) {
    appToast.error(
      isNoActiveWarranty(error)
        ? t("warranty.certificate.none")
        : t("warranty.certificate.failed"),
    );
  }
}

/**
 * "Warranty PDF" (TEC-188): queues the certificate of a service, waits for
 * the export job and downloads the PDF in the user language. The client
 * decides the realm (panel or portal).
 */
export function WarrantyCertificateButton({
  client,
  locale,
  waitOptions,
  testId = "warranty-pdf",
}: {
  client: CertificateClient;
  locale?: string;
  waitOptions?: WaitOptions;
  testId?: string;
}) {
  const { t } = useLocale();
  const [busy, setBusy] = useState(false);

  const onClick = async () => {
    setBusy(true);
    try {
      await downloadWarrantyCertificate({ client, locale, waitOptions, t });
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
      data-testid={testId}
    >
      {busy ? (
        <Loader2 className="size-4 animate-spin" />
      ) : (
        <ShieldCheck className="size-4" />
      )}
      {busy
        ? t("warranty.certificate.preparing")
        : t("warranty.certificate.download")}
    </Button>
  );
}
