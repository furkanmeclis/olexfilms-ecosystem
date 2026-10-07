"use client";

import { useQuery } from "@tanstack/react-query";
import {
  Car,
  ExternalLink,
  History,
  ImageIcon,
  Package,
  ShieldCheck,
  UserRound,
  Wand2,
  Wrench,
} from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  canContinueWizard,
  canDownloadWarrantyCertificate,
  resolveServiceListAccess,
} from "@/features/services/lib/access";
import {
  customerName,
  serviceStatusTone,
  sortedStatusLogs,
  vehicleTitle,
} from "@/features/services/lib/detail";
import {
  serviceImageSrc,
  serviceWizardKeys,
  serviceWizardService,
  type Service,
} from "@/features/services/services/service-wizard.service";
import {
  ServiceItemsTable,
  ServiceWarrantiesTable,
} from "@/features/services/components/service-detail-tables";
import { ServiceContractCard } from "@/features/services/components/service-contract-card";
import { ServicePdfButton } from "@/features/services/components/service-pdf-button";
import { ServiceMeasurementsSection } from "@/features/measurements/components/service-measurements-section";
import { useFeature } from "@/features/modules/hooks/use-features";
import { WarrantyCertificateButton } from "@/features/warranty/components/warranty-certificate-button";
import { panelCertificateClient } from "@/features/warranty/services/certificate.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function Section({
  title,
  icon,
  testId,
  children,
}: {
  title: string;
  icon: ReactNode;
  testId: string;
  children: ReactNode;
}) {
  return (
    <Card data-testid={testId}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="text-muted-foreground">{icon}</span>
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

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

function Empty({ children, testId }: { children: ReactNode; testId: string }) {
  return (
    <p className="text-muted-foreground text-sm" data-testid={testId}>
      {children}
    </p>
  );
}

function VehicleCustomer({
  service,
  slug,
  canOpenVehicle,
}: {
  service: Service;
  slug: string;
  canOpenVehicle: boolean;
}) {
  const { t, format } = useLocale();
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <Section
        title={t("services.detail.vehicle")}
        icon={<Car className="size-4" />}
        testId="detail-vehicle"
      >
        <dl className="grid gap-3 sm:grid-cols-2">
          <Field label={t("services.detail.vehicle_model")}>
            {dash(vehicleTitle(service))}
          </Field>
          <Field label={t("services.vehicle.plate")}>
            <span className="font-mono" dir="ltr">
              {dash(service.plate)}
              {service.plate && service.plate_country
                ? ` (${service.plate_country})`
                : ""}
            </span>
          </Field>
          <Field label={t("services.vin.label")}>
            <span className="font-mono" dir="ltr">
              {dash(service.vin)}
            </span>
          </Field>
          <Field label={t("services.wizard.km")}>
            {service.km === null ? "—" : format.number(service.km)}
          </Field>
        </dl>
        {canOpenVehicle ? (
          <Button asChild variant="link" className="mt-3 h-auto p-0">
            <Link
              href={routes.tenant.vehicles.detail(slug, service.vehicle_uuid)}
              data-testid="detail-vehicle-link"
            >
              <ExternalLink className="size-4" />
              {t("services.detail.vehicle_open")}
            </Link>
          </Button>
        ) : null}
      </Section>
      <Section
        title={t("services.detail.customer")}
        icon={<UserRound className="size-4" />}
        testId="detail-customer"
      >
        <dl className="grid gap-3 sm:grid-cols-2">
          <Field label={t("services.customer.name")}>
            {dash(customerName(service))}
            {service.customer.anonymized ? (
              <Badge variant="outline" className="ms-2">
                {t("services.detail.anonymized")}
              </Badge>
            ) : null}
          </Field>
          <Field label={t("services.customer.phone")}>
            <span dir="ltr">{dash(service.customer.phone)}</span>
          </Field>
          <Field label={t("services.detail.organization")}>
            {service.organization.name}
          </Field>
          <Field label={t("services.detail.measurement")}>
            {service.has_measurement
              ? t("services.measurement.yes")
              : t("services.measurement.no")}
          </Field>
        </dl>
      </Section>
    </div>
  );
}

function Items({ service }: { service: Service }) {
  const { t } = useLocale();
  const items = service.items ?? [];
  return (
    <Section
      title={t("services.stock.items_title", { count: items.length })}
      icon={<Package className="size-4" />}
      testId="detail-items"
    >
      {items.length === 0 ? (
        <Empty testId="detail-items-empty">
          {t("services.stock.items_empty")}
        </Empty>
      ) : (
        <ServiceItemsTable items={items} />
      )}
    </Section>
  );
}

function Images({ service }: { service: Service }) {
  const { t } = useLocale();
  const images = [...(service.images ?? [])].sort(
    (a, b) => a.sort_order - b.sort_order,
  );
  return (
    <Section
      title={t("services.detail.images")}
      icon={<ImageIcon className="size-4" />}
      testId="detail-images"
    >
      {images.length === 0 ? (
        <Empty testId="detail-images-empty">
          {t("services.detail.images_empty")}
        </Empty>
      ) : (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          {images.map((img, index) => (
            <a
              key={img.uuid}
              href={serviceImageSrc(img.url)}
              target="_blank"
              rel="noreferrer"
              className="block"
            >
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={serviceImageSrc(img.url)}
                alt={
                  img.title ?? t("services.detail.image_alt", { n: index + 1 })
                }
                loading="lazy"
                className="bg-muted aspect-square w-full rounded-md border object-cover"
                data-testid="detail-image"
              />
            </a>
          ))}
        </div>
      )}
    </Section>
  );
}

function Warranties({ service, slug }: { service: Service; slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const warranties = service.warranties ?? [];
  const empty =
    service.status === "completed"
      ? t("services.detail.warranties_none")
      : t("services.detail.warranties_pending");
  return (
    <Section
      title={t("services.detail.warranties")}
      icon={<ShieldCheck className="size-4" />}
      testId="detail-warranties"
    >
      {warranties.length === 0 ? (
        <Empty testId="detail-warranties-empty">{empty}</Empty>
      ) : (
        <ServiceWarrantiesTable
          slug={slug}
          warranties={warranties}
          canOpen={can(permissions.warranties.read)}
          emptyTitle={empty}
        />
      )}
    </Section>
  );
}

function StatusHistory({ service }: { service: Service }) {
  const { t, format } = useLocale();
  const logs = sortedStatusLogs(service.status_logs);
  return (
    <Section
      title={t("services.detail.history")}
      icon={<History className="size-4" />}
      testId="detail-history"
    >
      {logs.length === 0 ? (
        <Empty testId="detail-history-empty">
          {t("services.detail.history_empty")}
        </Empty>
      ) : (
        <ol className="space-y-3 border-s ps-4">
          {logs.map((log, i) => (
            <li
              key={`${log.created_at}-${i}`}
              className="space-y-0.5"
              data-testid="detail-log"
              data-status={log.to_status}
            >
              <p className="text-sm font-medium">
                {log.from_status
                  ? t("services.detail.history_change", {
                      from: t(`services.status.${log.from_status}`),
                      to: t(`services.status.${log.to_status}`),
                    })
                  : t(`services.status.${log.to_status}`)}
              </p>
              <p className="text-muted-foreground text-xs">
                {format.dateTime(log.created_at)}
                {log.by_other_organization ? (
                  <Badge variant="outline" className="ms-2">
                    {t("services.detail.history_other_org")}
                  </Badge>
                ) : null}
              </p>
              {log.note ? <p className="text-sm">{log.note}</p> : null}
            </li>
          ))}
        </ol>
      )}
    </Section>
  );
}

/**
 * Service detail (TEC-183): vehicle and customer, items with their parts,
 * images, warranties and the status history. A draft the caller may still
 * edit links back to the wizard; "PDF" downloads the service PDF (TEC-196).
 * TEC-300: a service expecting a measurement shows the before/after
 * section while the measurements module and measurements.link are on.
 */
export function ServiceDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format, locale } = useLocale();
  const { can } = usePermission();
  const access = resolveServiceListAccess(can);
  const measurements = useFeature(slug, "measurements");

  const service = useQuery({
    queryKey: serviceWizardKeys.service(uuid),
    queryFn: () => serviceWizardService.getService(uuid),
    enabled: access.canRead && uuid !== "",
  });

  const s = service.data;
  const title = s ? s.service_no : t("services.detail.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Wrench className="size-6" />}
      description={
        s
          ? t("services.detail.created", {
              date: format.dateTime(s.created_at),
            })
          : undefined
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("services.list.title"),
          href: access.canRead ? routes.tenant.services.list(slug) : undefined,
        },
        { label: title },
      ]}
      actions={
        s ? (
          <div className="flex flex-wrap items-center gap-2">
            <span data-testid="detail-status">
              <StatusChip
                label={s.status_label}
                tone={serviceStatusTone(s.status)}
              />
            </span>
            {canContinueWizard(can, s) ? (
              <Button asChild>
                <Link
                  href={routes.tenant.services.wizard(slug, s.uuid)}
                  data-testid="continue-wizard"
                >
                  <Wand2 className="size-4" />
                  {t("services.detail.continue_wizard")}
                </Link>
              </Button>
            ) : null}
            {canDownloadWarrantyCertificate(can, s) ? (
              <WarrantyCertificateButton
                client={panelCertificateClient(s.uuid)}
                locale={locale}
              />
            ) : null}
            <ServicePdfButton
              serviceUuid={s.uuid}
              serviceNo={s.service_no}
              locale={locale}
            />
          </div>
        ) : null
      }
    />
  );

  if (!access.canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("services.list.forbidden")}
        />
      </div>
    );
  }
  if (service.isLoading) return <Loading />;
  if (service.isError || !s) {
    const notFound = isApiError(service.error) && service.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={
            notFound
              ? t("services.detail.not_found")
              : t("common.error_generic")
          }
          onRetry={notFound ? undefined : () => void service.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6" data-testid="service-detail">
      {header}
      {s.status === "cancelled" && s.cancel_reason ? (
        <p
          className="border-destructive/40 bg-destructive/5 rounded-lg border p-3 text-sm"
          data-testid="cancel-reason"
        >
          {t("services.detail.cancel_reason", { reason: s.cancel_reason })}
        </p>
      ) : null}
      <VehicleCustomer
        service={s}
        slug={slug}
        canOpenVehicle={can(permissions.vehicles.read)}
      />
      {s.package || s.notes ? (
        <Card>
          <CardContent className="pt-6">
            <dl className="grid gap-3 sm:grid-cols-2">
              <Field label={t("services.detail.package")}>
                {dash(s.package)}
              </Field>
              <Field label={t("services.detail.notes")}>
                <span className="whitespace-pre-line">{dash(s.notes)}</span>
              </Field>
            </dl>
          </CardContent>
        </Card>
      ) : null}
      {s.contract ? <ServiceContractCard contract={s.contract} /> : null}
      {s.has_measurement &&
      measurements.enabled &&
      can(permissions.measurements.link) ? (
        <ServiceMeasurementsSection
          slug={slug}
          serviceUuid={s.uuid}
          editable={s.status !== "cancelled"}
          canOpen={can(permissions.measurements.read)}
        />
      ) : null}
      <Items service={s} />
      <Warranties service={s} slug={slug} />
      <div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <Images service={s} />
        <StatusHistory service={s} />
      </div>
    </div>
  );
}
