"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Info, TriangleAlert } from "lucide-react";
import { useState } from "react";

import { Loading } from "@/components/common/loading";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  MeasurementOption,
  toastLinkError,
} from "@/features/measurements/components/service-measurements-section";
import {
  phaseLink,
  phaseOptions,
  serviceMeasurementKeys,
  serviceMeasurementsService,
} from "@/features/measurements/services/service-measurements.service";
import { useFeature } from "@/features/modules/hooks/use-features";
import { VinField } from "@/features/services/components/vin-field";
import { normalizeVin, validateVin } from "@/features/services/lib/vin";
import {
  serviceWizardKeys,
  serviceWizardService,
  type Service,
} from "@/features/services/services/service-wizard.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export type MeasurementStepProps = {
  slug: string;
  service: Service;
  onSaved: (service: Service) => void;
  onBack: () => void;
};

/**
 * Step 3: "Is there a measurement?" and the VIN. A measurement needs a VIN
 * (services CHECK, K28); the VIN follows the backend rules and is saved on
 * the service snapshot (PATCH /v1/services/{uuid}).
 * TEC-300: with the measurements module and measurements.link, a "yes"
 * lists the unlinked measurements of the saved VIN and the dealer may pick
 * the "before" report (POST /v1/services/{uuid}/measurements after the
 * save). No pick only warns; it never blocks the wizard.
 */
export function MeasurementStep({
  slug,
  service,
  onSaved,
  onBack,
}: MeasurementStepProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const queryClient = useQueryClient();
  const measurementsModule = useFeature(slug, "measurements");
  const [hasMeasurement, setHasMeasurement] = useState(service.has_measurement);
  const [vin, setVin] = useState(service.vin ?? "");
  const [submitted, setSubmitted] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);
  const editable = service.editable;

  const vinError = validateVin(vin, { required: hasMeasurement });
  const canPick =
    measurementsModule.enabled && can(permissions.measurements.link);
  // Candidates follow the VIN saved on the service.
  const vinSaved = Boolean(service.vin) && normalizeVin(vin) === service.vin;

  const reports = useQuery({
    queryKey: serviceMeasurementKeys.links(service.uuid),
    queryFn: () => serviceMeasurementsService.list(service.uuid),
    enabled: canPick && hasMeasurement && vinSaved,
  });
  const current = phaseLink(reports.data, "before");
  const options = [
    ...(current ? [current.measurement] : []),
    ...phaseOptions(reports.data, "before"),
  ];
  const suggested = new Set(
    (reports.data?.suggestions ?? [])
      .filter((s) => s.phase === "before")
      .map((s) => s.measurement.uuid),
  );
  // undefined = untouched: the current "before" link stays selected.
  const [picked, setPicked] = useState<string | null | undefined>(undefined);
  const selected =
    picked === undefined ? (current?.measurement.uuid ?? null) : picked;
  const linkLocked = Boolean(current?.confirmed);

  const save = useMutation({
    mutationFn: async () => {
      const normalized = normalizeVin(vin);
      const saved = await serviceWizardService.updateService(service.uuid, {
        vin: normalized === "" ? null : normalized,
        has_measurement: hasMeasurement,
      });
      const linkIt =
        canPick &&
        hasMeasurement &&
        vinSaved &&
        selected !== null &&
        !(current?.confirmed && current.measurement.uuid === selected);
      if (linkIt) {
        try {
          const next = await serviceMeasurementsService.link(
            service.uuid,
            selected,
            "before",
          );
          queryClient.setQueryData(
            serviceMeasurementKeys.links(service.uuid),
            next,
          );
        } catch (error) {
          return { saved, linkError: error };
        }
      }
      return { saved, linkError: null as unknown };
    },
    onSuccess: ({ saved, linkError }) => {
      if (linkError) {
        // The answer is saved; the pick failed: stay to choose again.
        queryClient.setQueryData(serviceWizardKeys.service(saved.uuid), saved);
        toastLinkError(linkError, t);
        return;
      }
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

      {hasMeasurement && !canPick ? (
        <Alert>
          <Info />
          <AlertDescription>
            {t("services.measurement.report_auto")}
          </AlertDescription>
        </Alert>
      ) : null}
      {hasMeasurement && canPick ? (
        <section className="space-y-3" data-testid="measurement-reports">
          <div>
            <p className="text-sm font-medium">
              {t("measurements.wizard.title")}
            </p>
            <p className="text-muted-foreground text-sm">
              {t("measurements.wizard.description")}
            </p>
          </div>
          {!vinSaved ? (
            <Alert>
              <Info />
              <AlertDescription>
                {t("measurements.wizard.vin_changed")}
              </AlertDescription>
            </Alert>
          ) : reports.isLoading ? (
            <Loading />
          ) : reports.isError ? (
            <p className="text-destructive text-sm">
              {t("measurements.service.load_failed")}
            </p>
          ) : options.length === 0 ? (
            <p
              className="text-muted-foreground text-sm"
              data-testid="measurement-reports-empty"
            >
              {t("measurements.wizard.empty")}
            </p>
          ) : (
            <div className="space-y-2" role="radiogroup">
              {options.map((m) => (
                <MeasurementOption
                  key={m.uuid}
                  measurement={m}
                  selected={selected === m.uuid}
                  suggested={suggested.has(m.uuid)}
                  disabled={!editable || linkLocked}
                  onSelect={() =>
                    setPicked(selected === m.uuid ? null : m.uuid)
                  }
                />
              ))}
            </div>
          )}
          {vinSaved && !reports.isLoading && selected === null ? (
            <Alert data-testid="measurement-reports-warning">
              <TriangleAlert />
              <AlertDescription>
                {t("measurements.wizard.none_selected")}
              </AlertDescription>
            </Alert>
          ) : null}
        </section>
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
