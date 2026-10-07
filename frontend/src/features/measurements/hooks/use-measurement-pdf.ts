"use client";

import { useMutation } from "@tanstack/react-query";

import { measurementsService } from "@/features/measurements/services/measurements.service";
import { isApiError } from "@/lib/api";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** "PDF indir": waits for the render (202 polling) and saves the file. */
export function useMeasurementPdf() {
  const { t } = useLocale();
  return useMutation({
    mutationFn: (uuid: string) => measurementsService.downloadPdf(uuid),
    onSuccess: ({ blob, filename }) => triggerBrowserDownload(blob, filename),
    onError: (error) => {
      if (isApiError(error) && error.code === "MEASUREMENT_VIN_PENDING") {
        appToast.error(t("measurements.pdf.vin_pending"));
      } else if (
        isApiError(error) &&
        error.code === "MEASUREMENT_NOT_NORMALIZED"
      ) {
        appToast.error(t("measurements.pdf.not_normalized"));
      } else {
        appToast.error(t("measurements.pdf.failed"));
      }
    },
  });
}
