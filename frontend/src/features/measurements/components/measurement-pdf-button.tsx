"use client";

import { FileDown } from "lucide-react";
import type { ComponentProps } from "react";

import { Button } from "@/components/ui/button";
import { useMeasurementPdf } from "@/features/measurements/hooks/use-measurement-pdf";
import { useLocale } from "@/providers/locale-provider";

/**
 * "PDF indir" of one measurement (TEC-298): only an accepted measurement
 * has a PDF; the render is polled until the file streams.
 */
export function MeasurementPdfButton({
  uuid,
  status,
  size,
  testId = "measurement-pdf",
}: {
  uuid: string;
  status: string;
  size?: ComponentProps<typeof Button>["size"];
  testId?: string;
}) {
  const { t } = useLocale();
  const pdf = useMeasurementPdf();
  return (
    <Button
      type="button"
      variant="outline"
      size={size}
      disabled={status !== "accepted" || pdf.isPending}
      onClick={() => pdf.mutate(uuid)}
      data-testid={testId}
    >
      <FileDown className="size-4" />
      {pdf.isPending
        ? t("measurements.pdf.preparing")
        : t("measurements.pdf.download")}
    </Button>
  );
}
