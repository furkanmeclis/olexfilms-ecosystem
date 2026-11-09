"use client";

import { useMutation } from "@tanstack/react-query";
import { CheckCircle2, Info, Save } from "lucide-react";
import { useState } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  CertificateWarningBand,
  blockingCertificateWarnings,
  completionBlockedReason,
} from "@/features/certificates/components/certificate-status";
import { missingAnglesFromError } from "@/features/photo-standard/lib/photo-standard";
import {
  CustomerName,
  VehicleSummary,
} from "@/features/services/components/customer-vehicle-step";
import { ServiceNoField } from "@/features/services/components/service-no-field";
import { ItemRow } from "@/features/services/components/stock-step";
import { WizardActions } from "@/features/services/components/wizard-bar";
import { isKnownPart, partsFromItems } from "@/features/services/lib/car-parts";
import {
  extractServiceNo,
  isValidServiceNo,
} from "@/features/services/lib/service-no";
import { stockErrorKey } from "@/features/services/lib/stock";
import {
  serviceWizardService,
  type Service,
} from "@/features/services/services/service-wizard.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export type CompleteStepProps = {
  service: Service;
  onChanged: (service: Service) => void;
  onBack: () => void;
  onCompleted: (service: Service) => void;
  showCertificateWarnings?: boolean;
  /** TEC-500: completion refused for missing intake photo angles. */
  onPhotosIncomplete?: (missing: string[]) => void;
};

/**
 * Last step (TEC-1076, revize 9): a summary of the service, the service
 * number (TEC-1004 ServiceNoField: typed, pasted or scanned from the
 * warranty card) and the completion. The draft carries the generated
 * number until here; a changed number is saved (PATCH service_no) before
 * the service is completed. Without services.complete the number can still
 * be saved.
 */
export function CompleteStep({
  service,
  onChanged,
  onBack,
  onCompleted,
  showCertificateWarnings = true,
  onPhotosIncomplete,
}: CompleteStepProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const [serviceNo, setServiceNo] = useState(service.service_no);
  const items = service.items ?? [];
  const parts = partsFromItems(items);
  const no = extractServiceNo(serviceNo);
  const changed = no !== "" && no !== service.service_no;
  const contractSigned =
    service.contract != null && service.contract.status !== "voided";

  const errorText = (error: unknown) => {
    if (isApiError(error) && error.code === "SERVICE_NO_TAKEN") {
      return t("services.service_no.taken");
    }
    return t(stockErrorKey(isApiError(error) ? error.code : undefined));
  };

  /** Saves a changed number; resolves with the (maybe updated) service. */
  const saveNumber = async (): Promise<Service> => {
    if (!changed) return service;
    const saved = await serviceWizardService.updateService(service.uuid, {
      service_no: no,
    });
    onChanged(saved);
    return saved;
  };

  const validNumber = () => {
    if (changed && !isValidServiceNo(no)) {
      appToast.error(t("services.service_no.invalid"));
      return false;
    }
    return true;
  };

  const save = useMutation({
    mutationFn: saveNumber,
    onSuccess: () => appToast.success(t("services.wizard.saved")),
    onError: (error: unknown) => appToast.error(errorText(error)),
  });

  const complete = useMutation({
    mutationFn: async () => {
      const saved = await saveNumber();
      return serviceWizardService.transition(saved.uuid, "completed");
    },
    onSuccess: (saved) => {
      appToast.success(t("services.complete.done", { no: saved.service_no }));
      onCompleted(saved);
    },
    onError: (error: unknown) => {
      appToast.error(errorText(error));
      const missing = missingAnglesFromError(error);
      if (missing && onPhotosIncomplete) onPhotosIncomplete(missing);
    },
  });

  const canCompletePermission = can(permissions.services.complete);
  const canComplete =
    canCompletePermission &&
    service.available_transitions.includes("completed") &&
    (!showCertificateWarnings ||
      blockingCertificateWarnings(service).length === 0);
  const certificateBlock = showCertificateWarnings
    ? completionBlockedReason(service)
    : null;
  const busy = save.isPending || complete.isPending;

  return (
    <div className="space-y-6" data-testid="complete-step">
      <div className="grid gap-4 md:grid-cols-2">
        <section className="space-y-2">
          <h3 className="text-sm font-medium">
            {t("services.customer.title")}
          </h3>
          <p data-testid="summary-customer">
            <CustomerName customer={service.customer} />
          </p>
        </section>
        <section className="space-y-2">
          <h3 className="text-sm font-medium">{t("services.vehicle.title")}</h3>
          <VehicleSummary
            brandUuid={service.car_brand.uuid}
            brand={service.car_brand.name}
            model={service.car_model.name}
            plate={service.plate}
            year={service.model_year}
            vin={service.vin}
          />
        </section>
      </div>

      <section className="space-y-2" data-testid="summary-parts">
        <h3 className="text-sm font-medium">
          {t("services.complete.parts_title")}
        </h3>
        {parts.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("services.parts.selected_none")}
          </p>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {parts.map((p) => (
              <Badge key={p} variant="outline">
                {isKnownPart(p) ? t(`services.parts.names.${p}`) : p}
              </Badge>
            ))}
          </div>
        )}
      </section>

      <section className="space-y-2">
        <h3 className="text-sm font-medium">
          {t("services.stock.items_title", { count: items.length })}
        </h3>
        {items.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="items-empty"
          >
            {t("services.stock.items_empty")}
          </p>
        ) : (
          <ul className="space-y-2">
            {items.map((item) => (
              <ItemRow key={item.uuid} item={item} editable={false} />
            ))}
          </ul>
        )}
      </section>

      <section className="space-y-3 border-t pt-4">
        <ServiceNoField
          value={serviceNo}
          onChange={setServiceNo}
          serviceUuid={service.uuid}
          disabled={!service.service_no_editable}
        />
        <p
          className="text-muted-foreground text-xs"
          data-testid="service-no-last-hint"
        >
          {t("services.complete.service_no_hint", { no: service.service_no })}
        </p>
        {changed && contractSigned ? (
          <Alert>
            <Info />
            <AlertDescription data-testid="service-no-contract-hint">
              {t("services.complete.service_no_contract_hint", {
                no: service.service_no,
              })}
            </AlertDescription>
          </Alert>
        ) : null}
      </section>

      {!canCompletePermission ? (
        <p
          className="text-muted-foreground text-sm"
          data-testid="complete-hint"
        >
          {t("services.complete.no_permission")}
        </p>
      ) : null}
      <CertificateWarningBand
        service={service}
        compact
        enabled={showCertificateWarnings}
      />
      {certificateBlock ? (
        <p
          className="text-muted-foreground text-sm"
          data-testid="complete-hint"
        >
          {t(certificateBlock)}
        </p>
      ) : null}

      <WizardActions>
        <Button type="button" variant="outline" onClick={onBack}>
          {t("services.wizard.back")}
        </Button>
        {!canCompletePermission && service.service_no_editable ? (
          <Button
            type="button"
            variant="outline"
            data-testid="service-no-save"
            disabled={!changed || busy}
            onClick={() => {
              if (validNumber()) save.mutate();
            }}
          >
            <Save className="size-4" />
            {t("common.save")}
          </Button>
        ) : null}
        {canCompletePermission ? (
          <Button
            type="button"
            data-testid="complete-service"
            disabled={!canComplete || items.length === 0 || busy}
            onClick={() => {
              if (validNumber()) complete.mutate();
            }}
          >
            <CheckCircle2 className="size-4" />
            {t("services.complete.button")}
          </Button>
        ) : null}
      </WizardActions>
    </div>
  );
}
