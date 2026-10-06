"use client";

import { Badge } from "@/components/ui/badge";
import { useLocale } from "@/providers/locale-provider";

/** accepted / vin_pending ("tamamlanacak") badge. */
export function MeasurementStatusBadge({ status }: { status: string }) {
  const { t } = useLocale();
  return status === "vin_pending" ? (
    <Badge variant="warning">{t("measurements.status.vin_pending")}</Badge>
  ) : (
    <Badge variant="success">{t("measurements.status.accepted")}</Badge>
  );
}

/** The VIN, or the "to be completed" badge while it is missing. */
export function MeasurementVin({ vin }: { vin: string | null }) {
  const { t } = useLocale();
  return vin ? (
    <span className="font-mono text-xs" dir="ltr">
      {vin}
    </span>
  ) : (
    <Badge variant="warning" data-testid="vin-pending-badge">
      {t("measurements.vin.pending_badge")}
    </Badge>
  );
}
