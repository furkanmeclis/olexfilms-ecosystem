"use client";

import { useQuery } from "@tanstack/react-query";
import {
  ExternalLink,
  MapPin,
  MessageCircle,
  Package,
  ShieldCheck,
  Store,
  Wrench,
} from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { PortalCarParts } from "@/features/portal/components/portal-car-parts";
import { PortalPage } from "@/features/portal/components/portal-page";
import { PortalServiceReview } from "@/features/portal/components/portal-service-review";
import {
  PortalApiError,
  portalApi,
  type PortalService,
} from "@/features/portal/lib/portal-client";
import {
  highlightedParts,
  portalVehicleTitle,
  publicWarrantyPath,
  whatsappUrl,
} from "@/features/portal/lib/portal-vehicles";
import { portalServicePdfClient } from "@/features/portal/services/portal-service-pdf.service";
import { ServicePdfButton } from "@/features/services/components/service-pdf-button";
import { isKnownPart } from "@/features/services/lib/car-parts";
import {
  serviceStatusTone,
  warrantyTone,
} from "@/features/services/lib/detail";
import type { ServiceStatus } from "@/features/services/services/service-wizard.service";
import { WarrantyCertificateButton } from "@/features/warranty/components/warranty-certificate-button";
import { WarrantyProgressBar } from "@/features/warranty/components/warranty-progress";
import { portalCertificateClient } from "@/features/warranty/services/certificate.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** Body of a loaded service (exported for tests). */
export function PortalServiceView({
  service,
  now,
  reviewFromLink = false,
}: {
  service: PortalService;
  now?: Date;
  /** TEC-353: opened from the WhatsApp review link. */
  reviewFromLink?: boolean;
}) {
  const { t, format, locale } = useLocale();
  const [focused, setFocused] = useState<string | null>(null);
  const applied = useMemo(
    () => new Set(service.applied_parts),
    [service.applied_parts],
  );
  const highlighted = useMemo(
    () => highlightedParts(service, focused),
    [service, focused],
  );
  const partLabel = (part: string) =>
    isKnownPart(part) ? t(`services.parts.names.${part}`) : part;
  const wa = whatsappUrl(service.dealer.whatsapp);
  const hasActiveWarranty = service.warranties.some(
    (w) => w.status === "active",
  );
  const vehicleTitle = portalVehicleTitle(service);

  return (
    <div className="space-y-6">
      <Card>
        <CardContent className="flex flex-wrap items-start justify-between gap-4 pt-6">
          <div className="space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-mono font-semibold" dir="ltr">
                {service.service_no}
              </span>
              <StatusChip
                label={service.status_label}
                tone={serviceStatusTone(service.status as ServiceStatus)}
              />
            </div>
            <p className="text-muted-foreground text-sm">
              {vehicleTitle}
              {service.plate ? (
                <span className="ms-2 font-mono" dir="ltr">
                  {service.plate}
                </span>
              ) : null}
            </p>
            <p className="text-muted-foreground text-xs">
              {service.completed_at
                ? t("portal.service.completed_at", {
                    date: format.date(service.completed_at),
                  })
                : t("portal.service.created_at", {
                    date: format.date(service.created_at),
                  })}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <ServicePdfButton
              serviceUuid={service.uuid}
              serviceNo={service.service_no}
              locale={locale}
              client={portalServicePdfClient(service.uuid)}
            />
            {hasActiveWarranty ? (
              <WarrantyCertificateButton
                client={portalCertificateClient(service.uuid)}
                locale={locale}
              />
            ) : null}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Wrench className="size-5" />
            {t("portal.service.parts_title")}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <PortalCarParts applied={applied} highlighted={highlighted} />
          {service.applied_parts.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("portal.service.no_parts")}
            </p>
          ) : (
            <ul
              className="flex flex-wrap gap-1.5"
              aria-label={t("portal.service.parts_title")}
            >
              {service.applied_parts.map((p) => (
                <li key={p}>
                  <Badge
                    variant={highlighted.has(p) ? "default" : "secondary"}
                    data-part-chip={p}
                  >
                    {partLabel(p)}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Package className="size-5" />
            {t("portal.service.products_title")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {service.products.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("portal.service.no_products")}
            </p>
          ) : (
            <ul className="space-y-2" data-testid="portal-service-products">
              {service.products.map((p) => {
                const on = focused === p.service_item_uuid;
                return (
                  <li key={p.service_item_uuid}>
                    <button
                      type="button"
                      aria-pressed={on}
                      data-product={p.service_item_uuid}
                      disabled={p.applied_parts.length === 0}
                      onClick={() =>
                        setFocused(on ? null : p.service_item_uuid)
                      }
                      className={cn(
                        "w-full space-y-1 rounded-lg border p-3 text-start transition-colors",
                        "focus-visible:ring-ring outline-none focus-visible:ring-2",
                        on
                          ? "border-primary bg-primary/5"
                          : "enabled:hover:bg-muted/50",
                      )}
                    >
                      <span className="flex flex-wrap items-center justify-between gap-2">
                        <span className="font-medium">{p.name}</span>
                        <span className="text-muted-foreground text-xs">
                          {p.category}
                        </span>
                      </span>
                      {p.applied_parts.length > 0 ? (
                        <span className="text-muted-foreground block text-xs">
                          {p.applied_parts.map(partLabel).join(", ")}
                        </span>
                      ) : null}
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
          {service.products.some((p) => p.applied_parts.length > 0) ? (
            <p className="text-muted-foreground mt-3 text-xs">
              {t("portal.service.products_hint")}
            </p>
          ) : null}
        </CardContent>
      </Card>

      {service.warranties.length > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <ShieldCheck className="size-5" />
              {t("portal.service.warranties_title")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="space-y-4">
              {service.warranties.map((w) => (
                <li
                  key={w.uuid}
                  className="space-y-2"
                  data-testid="portal-service-warranty"
                >
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="font-medium">{w.product_name}</span>
                    <StatusChip
                      label={t(`warranty.status.${w.status}`)}
                      tone={warrantyTone(w.status)}
                    />
                  </div>
                  <WarrantyProgressBar warranty={w} now={now} />
                  <a
                    href={publicWarrantyPath(w.public_code)}
                    target="_blank"
                    rel="noreferrer"
                    className="text-primary inline-flex items-center gap-1 text-xs hover:underline"
                  >
                    <ExternalLink className="size-3" />
                    {t("warranty.detail.public_page")}
                  </a>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      ) : null}

      <Card data-testid="portal-dealer">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Store className="size-5" />
            {t("portal.service.dealer_title")}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="font-medium">{service.dealer.name}</p>
          {service.dealer.address ||
          service.dealer.city ||
          service.dealer.district ? (
            <p className="text-muted-foreground flex items-start gap-2 text-sm">
              <MapPin className="mt-0.5 size-4 shrink-0" />
              <span>
                {[
                  service.dealer.address,
                  [service.dealer.district, service.dealer.city]
                    .filter(Boolean)
                    .join(" / "),
                ]
                  .filter(Boolean)
                  .join(", ")}
              </span>
            </p>
          ) : null}
          {wa ? (
            <Button
              asChild
              className="bg-[#25D366] text-white hover:bg-[#1ebe5b]"
            >
              <a
                href={wa}
                target="_blank"
                rel="noreferrer"
                data-testid="dealer-whatsapp"
              >
                <MessageCircle className="size-4" />
                {t("portal.service.whatsapp")}
              </a>
            </Button>
          ) : null}
        </CardContent>
      </Card>

      <PortalServiceReview
        serviceUuid={service.uuid}
        fromLink={reviewFromLink}
      />
    </div>
  );
}

/**
 * Portal > service (TEC-241): applied parts on the vehicle drawing, the
 * products, the warranties the user holds, the dealer card with WhatsApp
 * and the service PDF (GET /v1/portal/services/{uuid}, TEC-239).
 */
export function PortalServiceDetail({
  uuid,
  now,
  reviewFromLink = false,
}: {
  uuid: string;
  now?: Date;
  /** TEC-353: `?source=whatsapp_link` (the WhatsApp review link). */
  reviewFromLink?: boolean;
}) {
  const { t } = useLocale();
  const detail = useQuery({
    queryKey: ["portal", "services", "detail", uuid],
    queryFn: () => portalApi.getService(uuid),
    retry: (count, error) =>
      !(error instanceof PortalApiError && error.status === 404) && count < 2,
  });
  const service = detail.data;
  const back = service
    ? {
        href: routes.portal.vehicle(service.vehicle_uuid),
        label: t("portal.service.back"),
      }
    : { href: routes.portal.vehicles, label: t("portal.vehicle.back") };
  const notFound =
    detail.error instanceof PortalApiError && detail.error.status === 404;

  return (
    <PortalPage
      title={t("portal.service.title")}
      icon={<Wrench className="size-6" />}
      back={back}
      testId="portal-service-detail"
    >
      {detail.isError ? (
        notFound ? (
          <Card>
            <CardContent
              className="py-10 text-center"
              data-testid="portal-service-not-found"
            >
              {t("portal.service.not_found")}
            </CardContent>
          </Card>
        ) : (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void detail.refetch()}
            retryLabel={t("common.retry")}
          />
        )
      ) : service ? (
        <PortalServiceView
          service={service}
          now={now}
          reviewFromLink={reviewFromLink}
        />
      ) : (
        <p className="text-muted-foreground text-sm">
          {t("portal.vehicles.loading")}
        </p>
      )}
    </PortalPage>
  );
}
