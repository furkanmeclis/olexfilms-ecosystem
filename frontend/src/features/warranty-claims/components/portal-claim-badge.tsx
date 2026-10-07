"use client";

import { StatusChip } from "@/components/common/status-chip";
import {
  claimStatusTone,
  type PortalClaimStatus,
} from "@/features/warranty-claims/lib/claims";
import { useLocale } from "@/providers/locale-provider";

/**
 * Portal claim status of a warranty (TEC-339): only the status and the
 * date of the last change. The portal API carries no description or
 * photos, and this badge renders none even if a richer object is passed.
 */
export function PortalClaimBadge({ claim }: { claim: PortalClaimStatus }) {
  const { t, format } = useLocale();
  return (
    <div
      className="flex flex-wrap items-center gap-2 text-xs"
      data-testid="portal-claim-badge"
    >
      <span className="text-muted-foreground">
        {t("warranty.claims.portal.label")}
      </span>
      <StatusChip
        label={t(`warranty.claims.status.${claim.status}`)}
        tone={claimStatusTone(claim.status)}
      />
      <span className="text-muted-foreground">
        {format.date(claim.updated_at)}
      </span>
    </div>
  );
}
