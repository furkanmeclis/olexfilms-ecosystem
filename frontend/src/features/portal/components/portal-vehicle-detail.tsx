"use client";

import { useQuery } from "@tanstack/react-query";
import { Car, ChevronRight, ShieldCheck, Wrench } from "lucide-react";
import Link from "next/link";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { PortalPage } from "@/features/portal/components/portal-page";
import { PortalWarrantyQr } from "@/features/portal/components/portal-warranty-qr";
import {
  PortalApiError,
  portalApi,
  type PortalActiveWarranty,
} from "@/features/portal/lib/portal-client";
import { portalVehicleTitle } from "@/features/portal/lib/portal-vehicles";
import { serviceStatusTone } from "@/features/services/lib/detail";
import { WarrantyCertificateButton } from "@/features/warranty/components/warranty-certificate-button";
import { WarrantyProgressBar } from "@/features/warranty/components/warranty-progress";
import { portalCertificateClient } from "@/features/warranty/services/certificate.service";
import { useLocale } from "@/providers/locale-provider";

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="bg-muted/40 rounded-lg p-3">
      <p className="text-muted-foreground text-xs">{label}</p>
      <p className="text-lg font-semibold tabular-nums">{value}</p>
    </div>
  );
}

/** Active warranty card: product, days left bar, certificate PDF and QR. */
export function PortalActiveWarrantyCard({
  warranty,
  now,
}: {
  warranty: PortalActiveWarranty;
  now?: Date;
}) {
  const { t, format, locale } = useLocale();
  return (
    <Card data-testid="portal-active-warranty" data-uuid={warranty.uuid}>
      <CardContent className="flex flex-col gap-4 pt-6 sm:flex-row">
        <div className="min-w-0 flex-1 space-y-3">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div className="min-w-0">
              <p className="font-medium">{warranty.product.name}</p>
              <p className="text-muted-foreground text-xs">
                {warranty.organization.name} ·{" "}
                <Link
                  href={routes.portal.service(warranty.service.uuid)}
                  className="hover:underline"
                >
                  <span dir="ltr">{warranty.service.service_no}</span>
                </Link>
              </p>
            </div>
            <StatusChip label={t("warranty.status.active")} tone="success" />
          </div>
          <WarrantyProgressBar
            warranty={{
              status: "active",
              start_at: warranty.start_at,
              end_at: warranty.end_at,
            }}
            now={now}
          />
          <p className="text-muted-foreground text-xs">
            {format.date(warranty.start_at)} – {format.date(warranty.end_at)}
          </p>
          <WarrantyCertificateButton
            client={portalCertificateClient(warranty.service.uuid)}
            locale={locale}
            testId={`warranty-pdf-${warranty.uuid}`}
          />
        </div>
        <PortalWarrantyQr publicCode={warranty.public_code} />
      </CardContent>
    </Card>
  );
}

/**
 * Portal > My vehicles > vehicle (TEC-241): summary, active warranties
 * (days left, certificate PDF, QR of the public page) and the service
 * history (GET /v1/portal/vehicles/{uuid}).
 */
export function PortalVehicleDetail({
  uuid,
  now,
}: {
  uuid: string;
  now?: Date;
}) {
  const { t, format } = useLocale();
  const detail = useQuery({
    queryKey: ["portal", "vehicles", "detail", uuid],
    queryFn: () => portalApi.getVehicle(uuid),
    retry: (count, error) =>
      !(error instanceof PortalApiError && error.status === 404) && count < 2,
  });
  const back = {
    href: routes.portal.vehicles,
    label: t("portal.vehicle.back"),
  };

  if (detail.isError) {
    const notFound =
      detail.error instanceof PortalApiError && detail.error.status === 404;
    return (
      <PortalPage title={t("portal.vehicles.title")} back={back}>
        {notFound ? (
          <Card>
            <CardContent
              className="py-10 text-center"
              data-testid="portal-vehicle-not-found"
            >
              {t("portal.vehicle.not_found")}
            </CardContent>
          </Card>
        ) : (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void detail.refetch()}
            retryLabel={t("common.retry")}
          />
        )}
      </PortalPage>
    );
  }
  if (!detail.data) {
    return (
      <PortalPage title={t("portal.vehicles.title")} back={back}>
        <p className="text-muted-foreground text-sm">
          {t("portal.vehicles.loading")}
        </p>
      </PortalPage>
    );
  }

  const v = detail.data;
  const summary = v.service_summary;
  return (
    <PortalPage
      title={portalVehicleTitle(v) || t("portal.vehicles.unknown_vehicle")}
      icon={<Car className="size-6" />}
      back={back}
      testId="portal-vehicle-detail"
    >
      <Card>
        <CardContent className="space-y-4 pt-6">
          <dl className="grid gap-3 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-muted-foreground text-xs">
                {t("portal.vehicle.plate")}
              </dt>
              <dd className="font-mono tracking-wider" dir="ltr">
                {v.plate || "—"}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground text-xs">
                {t("portal.vehicle.vin")}
              </dt>
              <dd className="font-mono break-all" dir="ltr">
                {v.vin || "—"}
              </dd>
            </div>
          </dl>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <Stat
              label={t("portal.vehicle.stat_services")}
              value={summary.total}
            />
            <Stat
              label={t("portal.vehicle.stat_completed")}
              value={summary.completed}
            />
            <Stat
              label={t("portal.vehicle.stat_dealers")}
              value={summary.organization_count}
            />
            <Stat
              label={t("portal.vehicle.stat_last_service")}
              value={
                summary.last_service_at
                  ? format.date(summary.last_service_at)
                  : "—"
              }
            />
          </div>
        </CardContent>
      </Card>

      <section className="space-y-3" aria-labelledby="portal-active-warranties">
        <h2
          id="portal-active-warranties"
          className="flex items-center gap-2 text-base font-semibold"
        >
          <ShieldCheck className="size-5" />
          {t("portal.vehicle.active_warranties")}
        </h2>
        {v.active_warranties.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="portal-active-warranties-empty"
          >
            {t("portal.vehicle.no_active_warranty")}
          </p>
        ) : (
          <div className="space-y-3">
            {v.active_warranties.map((w) => (
              <PortalActiveWarrantyCard key={w.uuid} warranty={w} now={now} />
            ))}
          </div>
        )}
      </section>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Wrench className="size-5" />
            {t("portal.vehicle.history")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {v.services.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("portal.vehicle.no_services")}
            </p>
          ) : (
            <ol className="divide-y" data-testid="portal-vehicle-services">
              {v.services.map((s) => (
                <li key={s.uuid} data-uuid={s.uuid}>
                  <Link
                    href={routes.portal.service(s.uuid)}
                    className="hover:bg-muted/50 -mx-2 flex items-center justify-between gap-3 rounded-md px-2 py-3"
                  >
                    <div className="min-w-0 space-y-0.5">
                      <div className="flex flex-wrap items-center gap-2 text-sm font-medium">
                        <span dir="ltr" className="font-mono">
                          {s.service_no}
                        </span>
                        <StatusChip
                          label={t(`services.status.${s.status}`)}
                          tone={serviceStatusTone(s.status)}
                        />
                      </div>
                      <p className="text-muted-foreground truncate text-xs">
                        {[
                          s.organization.name,
                          s.package,
                          format.date(s.completed_at ?? s.created_at),
                        ]
                          .filter(Boolean)
                          .join(" · ")}
                      </p>
                    </div>
                    <ChevronRight className="text-muted-foreground size-4 shrink-0 rtl:rotate-180" />
                  </Link>
                </li>
              ))}
            </ol>
          )}
        </CardContent>
      </Card>
    </PortalPage>
  );
}
