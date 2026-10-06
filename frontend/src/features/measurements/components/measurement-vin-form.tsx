"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  measurementKeys,
  measurementsService,
} from "@/features/measurements/services/measurements.service";
import { VinField } from "@/features/services/components/vin-field";
import { normalizeVin, validateVin } from "@/features/services/lib/vin";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * VIN completion of a vin_pending measurement (PATCH
 * /v1/measurements/{uuid}/vin, measurements.link): the VIN rules of the
 * service wizard are checked before sending; the answer replaces the
 * detail (status accepted).
 */
export function MeasurementVinForm({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [vin, setVin] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);

  const save = useMutation({
    mutationFn: (value: string) => measurementsService.completeVin(uuid, value),
    onSuccess: (detail) => {
      qc.setQueryData(measurementKeys.detail(uuid), detail);
      void qc.invalidateQueries({ queryKey: measurementKeys.lists });
      appToast.success(t("measurements.vin.saved"));
    },
    onError: (error) => {
      if (isApiError(error) && error.code === "VALIDATION_ERROR") {
        setServerError(
          error.details.find((d) => d.field === "vin")?.message ??
            error.message,
        );
        return;
      }
      if (isApiError(error) && error.code === "MEASUREMENT_VIN_ALREADY_SET") {
        appToast.error(t("measurements.vin.already_set"));
        return;
      }
      appToast.error(t("measurements.vin.failed"));
    },
  });

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    setSubmitted(true);
    setServerError(null);
    if (validateVin(vin, { required: true })) return;
    save.mutate(normalizeVin(vin));
  };

  return (
    <Card data-testid="measurement-vin-form">
      <CardHeader>
        <CardTitle>{t("measurements.vin.title")}</CardTitle>
        <CardDescription>{t("measurements.vin.description")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form className="space-y-4" onSubmit={onSubmit} noValidate>
          <VinField
            value={vin}
            onChange={(value) => {
              setVin(value);
              setServerError(null);
            }}
            required
            showError={submitted}
            serverError={serverError}
            disabled={save.isPending}
          />
          <div className="flex justify-end">
            <Button type="submit" disabled={save.isPending}>
              {t("measurements.vin.submit")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
