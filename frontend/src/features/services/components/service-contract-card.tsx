"use client";

import { useQuery } from "@tanstack/react-query";
import { FileDown, FileSignature, Loader2 } from "lucide-react";
import { useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import {
  contractSigningKeys,
  contractSigningService,
  type ContractStatus,
} from "@/features/contracts/services/contract-signing.service";
import type { Service } from "@/features/services/services/service-wizard.service";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Poll interval while an executed contract's PDF is being rendered. */
export const CONTRACT_PDF_POLL_MS = 5000;

const tone = (status: ContractStatus) =>
  status === "executed"
    ? "success"
    : status === "voided"
      ? "danger"
      : status === "pending"
        ? "warning"
        : "default";

/**
 * Intake contract on the service detail (TEC-291): status, number and the
 * executed PDF. Until the PDF is stored the button spins as "hazırlanıyor"
 * and the contract is polled.
 */
export function ServiceContractCard({
  contract,
}: {
  contract: NonNullable<Service["contract"]>;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.contracts.read);
  const executed = contract.status === "executed";
  const [busy, setBusy] = useState(false);

  const poll = useQuery({
    queryKey: contractSigningKeys.detail(contract.uuid),
    queryFn: () => contractSigningService.get(contract.uuid),
    enabled: canRead && executed && !contract.pdf_ready,
    refetchInterval: (q) =>
      q.state.data?.pdf_ready ? false : CONTRACT_PDF_POLL_MS,
  });
  const pdfReady = contract.pdf_ready || Boolean(poll.data?.pdf_ready);

  const download = async () => {
    setBusy(true);
    try {
      const { blob, filename } = await contractSigningService.downloadPdf(
        contract.uuid,
      );
      triggerBrowserDownload(
        blob,
        filename ?? `contract-${contract.contract_no}.pdf`,
      );
    } catch {
      appToast.error(t("services.contract.errors.pdf_failed"));
    } finally {
      setBusy(false);
    }
  };

  const preparing = executed && !pdfReady;
  return (
    <Card data-testid="service-contract">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="text-muted-foreground">
            <FileSignature className="size-4" />
          </span>
          {t("services.contract.title")}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3 text-sm">
          <span dir="ltr" data-testid="service-contract-no">
            #{contract.contract_no}
          </span>
          <span data-testid="service-contract-status">
            <StatusChip
              label={t(`services.contract.status.${contract.status}`)}
              tone={tone(contract.status)}
            />
          </span>
        </div>
        {executed && canRead ? (
          <Button
            type="button"
            variant="outline"
            disabled={preparing || busy}
            aria-busy={preparing || busy}
            onClick={() => void download()}
            data-testid="service-contract-pdf"
          >
            {preparing || busy ? (
              <Loader2
                className="size-4 animate-spin"
                data-testid="service-contract-pdf-spinner"
              />
            ) : (
              <FileDown className="size-4" />
            )}
            {preparing
              ? t("services.contract.pdf_preparing")
              : t("services.contract.pdf")}
          </Button>
        ) : null}
      </CardContent>
    </Card>
  );
}
