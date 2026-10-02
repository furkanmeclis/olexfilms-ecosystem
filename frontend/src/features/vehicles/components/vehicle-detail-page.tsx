"use client";

import { useQuery } from "@tanstack/react-query";
import { Car } from "lucide-react";
import type { ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Card, CardContent } from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { VehicleTransferCard } from "@/features/vehicles/components/vehicle-transfer-card";
import {
  vehicleKeys,
  vehicleTransferService,
} from "@/features/vehicles/services/vehicle-transfer.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm break-words">{children}</dd>
    </div>
  );
}

const dash = (v: string | number | null | undefined) =>
  v === null || v === undefined || v === "" ? "—" : v;

/**
 * Vehicle detail (TEC-190, minimal): the vehicle's data and its ownership
 * transfer. Needs vehicles.read; the transfer card needs vehicles.transfer.
 */
export function VehicleDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.vehicles.read);
  const canTransfer = can(permissions.vehicles.transfer);

  const vehicle = useQuery({
    queryKey: vehicleKeys.vehicle(uuid),
    queryFn: () => vehicleTransferService.getVehicle(uuid),
    enabled: canRead && uuid !== "",
  });
  const v = vehicle.data;
  const header = (
    <PageHeader
      title={v?.plate || t("vehicles.detail.title")}
      icon={<Car className="size-6" />}
      description={t("vehicles.detail.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("vehicles.detail.title") },
      ]}
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("vehicles.detail.forbidden")}
        />
      </div>
    );
  }
  if (vehicle.isLoading) return <Loading />;
  if (vehicle.isError || !v) {
    const notFound = isApiError(vehicle.error) && vehicle.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={
            notFound
              ? t("vehicles.detail.not_found")
              : t("common.error_generic")
          }
          onRetry={notFound ? undefined : () => void vehicle.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }

  const model = [v.car_brand?.name, v.car_model?.name, v.model_year]
    .filter(Boolean)
    .join(" ");
  return (
    <div className="space-y-6" data-testid="vehicle-detail">
      {header}
      <Card>
        <CardContent className="pt-6">
          <dl className="grid gap-3 sm:grid-cols-3">
            <Field label={t("vehicles.detail.model")}>{dash(model)}</Field>
            <Field label={t("vehicles.detail.plate")}>
              <span className="font-mono" dir="ltr">
                {dash(v.plate)}
                {v.plate && v.plate_country ? ` (${v.plate_country})` : ""}
              </span>
            </Field>
            <Field label={t("vehicles.detail.vin")}>
              <span className="font-mono" dir="ltr">
                {dash(v.vin)}
              </span>
            </Field>
          </dl>
        </CardContent>
      </Card>
      {canTransfer ? <VehicleTransferCard vehicleUuid={v.uuid} /> : null}
    </div>
  );
}
