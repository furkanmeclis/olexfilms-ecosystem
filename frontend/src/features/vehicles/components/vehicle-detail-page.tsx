"use client";

import { useQuery } from "@tanstack/react-query";
import { Car, User } from "lucide-react";
import Link from "next/link";
import { useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatsCard } from "@/components/common/stats-card";
import { PageHeader } from "@/components/layout/page-header";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  customerKeys,
  customersService,
} from "@/features/customers/services/customers.service";
import {
  serviceWizardKeys,
  serviceWizardService,
} from "@/features/services/services/service-wizard.service";
import {
  claimKeys,
  claimsService,
} from "@/features/warranty-claims/services/claims.service";
import {
  warrantyKeys,
  warrantyService,
} from "@/features/warranty/services/warranty.service";
import {
  VehicleClaimsTable,
  VehicleServicesTable,
  VehicleWarrantiesTable,
} from "@/features/vehicles/components/vehicle-360-tables";
import { VehicleTransferCard } from "@/features/vehicles/components/vehicle-transfer-card";
import {
  VEHICLE_360_LIMIT,
  ownerFromServices,
  personName,
  summarizeVehicle360,
} from "@/features/vehicles/lib/vehicle-360";
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

type Tab = "services" | "warranties" | "claims" | "ownership";

/**
 * Vehicle 360 (TEC-190, TEC-863): the vehicle's data, its current owner,
 * summary figures and tabs with the vehicle's services, warranties and
 * warranty claims (each list endpoint filtered by `vehicle_uuid`, so the
 * caller's own scope applies) and the ownership transfer. Each section
 * needs its own read permission; a missing one hides the section.
 */
export function VehicleDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.vehicles.read);
  const canTransfer = can(permissions.vehicles.transfer);
  const canServices = can(permissions.services.read);
  const canWarranties = can(permissions.warranties.read);
  const canClaims = can(permissions.warrantyClaims.read);
  const canCustomers = can(permissions.customers.read);

  const vehicle = useQuery({
    queryKey: vehicleKeys.vehicle(uuid),
    queryFn: () => vehicleTransferService.getVehicle(uuid),
    enabled: canRead && uuid !== "",
  });
  const v = vehicle.data;
  const ready = canRead && !!v;

  const listParams = {
    vehicle_uuid: uuid,
    limit: VEHICLE_360_LIMIT,
    offset: 0,
  };
  const services = useQuery({
    queryKey: serviceWizardKeys.list({ ...listParams, sort: "-created_at" }),
    queryFn: () =>
      serviceWizardService.listServices({ ...listParams, sort: "-created_at" }),
    enabled: ready && canServices,
  });
  const warranties = useQuery({
    queryKey: warrantyKeys.list(listParams),
    queryFn: () => warrantyService.list(listParams),
    enabled: ready && canWarranties,
  });
  const claims = useQuery({
    queryKey: claimKeys.list(listParams),
    queryFn: () => claimsService.list(listParams),
    enabled: ready && canClaims,
  });
  const customer = useQuery({
    queryKey: customerKeys.detail(v?.customer_uuid ?? ""),
    queryFn: () => customersService.get(v?.customer_uuid ?? ""),
    enabled: ready && canCustomers && !!v?.customer_uuid,
  });

  const tabs: Tab[] = [
    ...(canServices ? (["services"] as const) : []),
    ...(canWarranties ? (["warranties"] as const) : []),
    ...(canClaims ? (["claims"] as const) : []),
    ...(canTransfer ? (["ownership"] as const) : []),
  ];
  const [tab, setTab] = useState<Tab | null>(null);
  const activeTab = tab && tabs.includes(tab) ? tab : (tabs[0] ?? null);

  const header = (
    <PageHeader
      title={v?.plate || t("vehicles.detail.title")}
      icon={<Car className="size-6" />}
      description={t("vehicles.detail.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        ...(canRead
          ? [
              {
                label: t("vehicles.list.title"),
                href: routes.tenant.vehicles.list(slug),
              },
            ]
          : []),
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

  const serviceRows = services.data?.items ?? [];
  const warrantyRows = warranties.data?.items ?? [];
  const claimRows = claims.data?.items ?? [];
  const summary = summarizeVehicle360({
    services: serviceRows,
    warranties: warrantyRows,
    claims: claimRows,
  });

  const fallbackOwner = ownerFromServices(serviceRows, v.customer_uuid);
  const owner = customer.data
    ? {
        name: personName(customer.data),
        company: customer.data.company_name,
        phone: customer.data.phone,
        email: customer.data.email,
      }
    : fallbackOwner
      ? {
          name: personName(fallbackOwner),
          company: null,
          phone: fallbackOwner.phone,
          email: null,
        }
      : null;

  const model = [v.car_brand?.name, v.car_model?.name, v.model_year]
    .filter(Boolean)
    .join(" ");

  const truncated = (total: number | undefined, shown: number) =>
    total !== undefined && total > shown ? (
      <p className="text-muted-foreground text-xs">
        {t("vehicles.detail.truncated", { shown, total })}
      </p>
    ) : null;

  const sectionError = (refetch: () => unknown) => (
    <ErrorState
      title={t("vehicles.detail.section_error")}
      onRetry={() => void refetch()}
      retryLabel={t("common.retry")}
    />
  );

  return (
    <div className="space-y-6" data-testid="vehicle-detail">
      {header}

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
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
              <Field label={t("vehicles.columns.created_at")}>
                {format.date(v.created_at)}
              </Field>
            </dl>
          </CardContent>
        </Card>
        <Card data-testid="vehicle-owner">
          <CardContent className="space-y-2 pt-6">
            <div className="text-muted-foreground flex items-center gap-2 text-xs">
              <User className="size-4" />
              {t("vehicles.detail.owner")}
            </div>
            {owner ? (
              <dl className="space-y-2">
                <div className="font-medium">{owner.name || "—"}</div>
                {owner.company ? (
                  <div className="text-muted-foreground text-sm">
                    {owner.company}
                  </div>
                ) : null}
                {owner.phone ? (
                  <Field label={t("vehicles.detail.owner_phone")}>
                    <span dir="ltr">{owner.phone}</span>
                  </Field>
                ) : null}
                {owner.email ? (
                  <Field label={t("vehicles.detail.owner_email")}>
                    <span dir="ltr">{owner.email}</span>
                  </Field>
                ) : null}
              </dl>
            ) : (
              <p className="text-muted-foreground text-sm">
                {customer.isLoading || services.isLoading
                  ? t("common.loading")
                  : t("vehicles.detail.owner_unknown")}
              </p>
            )}
            {canCustomers ? (
              <Link
                href={routes.tenant.customers.detail(slug, v.customer_uuid)}
                className="text-primary text-sm hover:underline"
                data-testid="vehicle-owner-link"
              >
                {t("vehicles.detail.owner_open")}
              </Link>
            ) : null}
          </CardContent>
        </Card>
      </div>

      {canServices || canWarranties || canClaims ? (
        <div
          className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4"
          data-testid="vehicle-kpis"
        >
          {canServices ? (
            <StatsCard
              title={t("vehicles.detail.kpi_services")}
              value={services.isLoading ? "…" : (services.data?.total ?? 0)}
            />
          ) : null}
          {canServices ? (
            <StatsCard
              title={t("vehicles.detail.kpi_last_service")}
              value={
                services.isLoading
                  ? "…"
                  : summary.lastServiceAt
                    ? format.date(summary.lastServiceAt)
                    : "—"
              }
            />
          ) : null}
          {canWarranties ? (
            <StatsCard
              title={t("vehicles.detail.kpi_active_warranties")}
              value={warranties.isLoading ? "…" : summary.activeWarranties}
              hint={
                summary.nextWarrantyEnd
                  ? t("vehicles.detail.kpi_next_warranty_end", {
                      date: format.date(summary.nextWarrantyEnd),
                    })
                  : undefined
              }
            />
          ) : null}
          {canClaims ? (
            <StatsCard
              title={t("vehicles.detail.kpi_open_claims")}
              value={claims.isLoading ? "…" : summary.openClaims}
            />
          ) : null}
        </div>
      ) : null}

      {activeTab ? (
        <Tabs value={activeTab} onValueChange={(next) => setTab(next as Tab)}>
          <TabsList className="flex-wrap">
            {tabs.map((value) => (
              <TabsTrigger
                key={value}
                value={value}
                data-testid={`vehicle-tab-${value}`}
              >
                {t(`vehicles.detail.tab_${value}`)}
              </TabsTrigger>
            ))}
          </TabsList>
          {canServices ? (
            <TabsContent value="services" className="space-y-2 pt-4">
              {services.isError ? (
                sectionError(services.refetch)
              ) : (
                <>
                  <VehicleServicesTable
                    slug={slug}
                    rows={serviceRows}
                    isLoading={services.isLoading}
                  />
                  {truncated(services.data?.total, serviceRows.length)}
                </>
              )}
            </TabsContent>
          ) : null}
          {canWarranties ? (
            <TabsContent value="warranties" className="space-y-2 pt-4">
              {warranties.isError ? (
                sectionError(warranties.refetch)
              ) : (
                <>
                  <VehicleWarrantiesTable
                    slug={slug}
                    rows={warrantyRows}
                    isLoading={warranties.isLoading}
                  />
                  {truncated(warranties.data?.total, warrantyRows.length)}
                </>
              )}
            </TabsContent>
          ) : null}
          {canClaims ? (
            <TabsContent value="claims" className="space-y-2 pt-4">
              {claims.isError ? (
                sectionError(claims.refetch)
              ) : (
                <>
                  <VehicleClaimsTable
                    slug={slug}
                    rows={claimRows}
                    isLoading={claims.isLoading}
                  />
                  {truncated(claims.data?.total, claimRows.length)}
                </>
              )}
            </TabsContent>
          ) : null}
          {canTransfer ? (
            <TabsContent value="ownership" className="pt-4">
              <VehicleTransferCard vehicleUuid={v.uuid} />
            </TabsContent>
          ) : null}
        </Tabs>
      ) : null}
    </div>
  );
}
