"use client";

import { useMutation } from "@tanstack/react-query";
import { Info } from "lucide-react";
import { useState } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { VinField } from "@/features/services/components/vin-field";
import { normalizeVin, validateVin } from "@/features/services/lib/vin";
import {
  serviceWizardService,
  type Service,
} from "@/features/services/services/service-wizard.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export type MeasurementStepProps = {
  service: Service;
  onSaved: (service: Service) => void;
  onBack: () => void;
};

/**
 * Step 3: "Is there a measurement?" and the VIN. A measurement needs a VIN
 * (services CHECK, K28); the VIN follows the backend rules and is saved on
 * the service snapshot (PATCH /v1/services/{uuid}).
 */
export function MeasurementStep({
  service,
  onSaved,
  onBack,
}: MeasurementStepProps) {
  const { t } = useLocale();
  const [hasMeasurement, setHasMeasurement] = useState(service.has_measurement);
  const [vin, setVin] = useState(service.vin ?? "");
  const [submitted, setSubmitted] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);
  const editable = service.editable;

  const vinError = validateVin(vin, { required: hasMeasurement });

  const save = useMutation({
    mutationFn: () => {
      const normalized = normalizeVin(vin);
      return serviceWizardService.updateService(service.uuid, {
        vin: normalized === "" ? null : normalized,
        has_measurement: hasMeasurement,
      });
    },
    onSuccess: (saved) => {
      appToast.success(t("services.wizard.saved"));
      onSaved(saved);
    },
    onError: (error: unknown) => {
      if (isApiError(error)) {
        const fields = error.fieldErrors();
        const fieldError = fields.vin ?? fields.has_measurement;
        if (fieldError) {
          setServerError(t("services.vin.errors.server"));
          return;
        }
        appToast.error(error.message);
        return;
      }
      appToast.error(t("services.wizard.save_failed"));
    },
  });

  const submit = () => {
    setSubmitted(true);
    setServerError(null);
    if (vinError) return;
    save.mutate();
  };

  return (
    <div className="space-y-6" data-testid="measurement-step">
      <fieldset className="space-y-3">
        <legend className="text-sm font-medium">
          {t("services.measurement.question")}
        </legend>
        <div className="flex flex-wrap gap-2" role="group">
          <Button
            type="button"
            variant={hasMeasurement ? "default" : "outline"}
            aria-pressed={hasMeasurement}
            disabled={!editable}
            data-testid="measurement-yes"
            onClick={() => setHasMeasurement(true)}
          >
            {t("services.measurement.yes")}
          </Button>
          <Button
            type="button"
            variant={hasMeasurement ? "outline" : "default"}
            aria-pressed={!hasMeasurement}
            disabled={!editable}
            data-testid="measurement-no"
            onClick={() => setHasMeasurement(false)}
          >
            {t("services.measurement.no")}
          </Button>
        </div>
        <p className="text-muted-foreground text-sm">
          {hasMeasurement
            ? t("services.measurement.vin_required")
            : t("services.measurement.vin_optional")}
        </p>
      </fieldset>

      <VinField
        value={vin}
        onChange={(v) => {
          setVin(v);
          setServerError(null);
        }}
        required={hasMeasurement}
        showError={submitted}
        serverError={serverError}
        disabled={!editable}
      />

      {hasMeasurement ? (
        <Alert>
          <Info />
          <AlertDescription>
            {t("services.measurement.report_placeholder")}
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="flex flex-wrap justify-between gap-2">
        <Button type="button" variant="outline" onClick={onBack}>
          {t("services.wizard.back")}
        </Button>
        <Button
          type="button"
          data-testid="measurement-save"
          disabled={!editable || save.isPending}
          onClick={submit}
        >
          {t("services.wizard.save_continue")}
        </Button>
      </div>
    </div>
  );
}
