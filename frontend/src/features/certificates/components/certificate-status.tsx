"use client";

import { AlertTriangle } from "lucide-react";

import { StatusChip } from "@/components/common/status-chip";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import type { Service } from "@/features/services/services/service-wizard.service";
import type {
  CertificateServiceWarning,
  CertificateStatus,
  WarningDecision,
} from "@/features/certificates/services/certificates.service";
import { localizedName } from "@/features/certificates/services/certificates.service";
import { useLocale } from "@/providers/locale-provider";

export type ServiceWithCertificateWarnings = Service & {
  certificate_warnings?: CertificateServiceWarning[];
};

export function certificateStatusTone(status: string) {
  if (status === "valid") return "success" as const;
  if (status === "pending") return "warning" as const;
  if (status === "rejected" || status === "revoked" || status === "expired") {
    return "danger" as const;
  }
  return "default" as const;
}

export function warningDecisionTone(decision: string) {
  if (decision === "approved") return "success" as const;
  if (decision === "pending_approval" || decision === "none") {
    return "warning" as const;
  }
  if (decision === "rejected") return "danger" as const;
  return "default" as const;
}

export function CertificateStatusChip({
  status,
}: {
  status: CertificateStatus | string;
}) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(`certificates.status.${status}`)}
      tone={certificateStatusTone(status)}
    />
  );
}

export function WarningDecisionChip({
  decision,
}: {
  decision: WarningDecision | string;
}) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(`certificates.warning_decisions.${decision}`)}
      tone={warningDecisionTone(decision)}
    />
  );
}

export function certificateWarnings(
  service: Service,
): CertificateServiceWarning[] {
  return (service as ServiceWithCertificateWarnings).certificate_warnings ?? [];
}

export function blockingCertificateWarnings(service: Service) {
  return certificateWarnings(service).filter((warning) =>
    ["none", "pending_approval", "rejected"].includes(warning.decision),
  );
}

export function CertificateWarningBand({
  service,
  compact = false,
  enabled = true,
}: {
  service: Service;
  compact?: boolean;
  enabled?: boolean;
}) {
  const { t, locale } = useLocale();
  const warnings = blockingCertificateWarnings(service);
  if (!enabled || !warnings.length) return null;

  const pending = warnings.some((w) => w.decision === "pending_approval");
  return (
    <Alert data-testid="certificate-warning-band">
      <AlertTriangle className="size-4" />
      <AlertTitle>
        {pending
          ? t("certificates.service.pending_title")
          : t("certificates.service.warning_title")}
      </AlertTitle>
      <AlertDescription className="space-y-2">
        <p>
          {pending
            ? t("certificates.service.pending_description")
            : t("certificates.service.warning_description")}
        </p>
        {compact ? null : (
          <ul className="list-inside list-disc">
            {warnings.map((warning) => (
              <li key={warning.uuid}>
                {localizedName(warning.type?.name, locale)} ·{" "}
                {[warning.user?.name, warning.user?.surname]
                  .filter(Boolean)
                  .join(" ") || t("certificates.fields.staff")}
              </li>
            ))}
          </ul>
        )}
      </AlertDescription>
    </Alert>
  );
}

export function completionBlockedReason(service: Service) {
  const warnings = blockingCertificateWarnings(service);
  if (!warnings.length) return null;
  if (warnings.some((w) => w.decision === "pending_approval")) {
    return "certificates.service.complete_pending";
  }
  return "certificates.service.complete_blocked";
}
