"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Car, ChevronRight, ShieldCheck, Wrench } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { PortalPage } from "@/features/portal/components/portal-page";
import { portalApi } from "@/features/portal/lib/portal-client";
import {
  PORTAL_VEHICLE_PAGE_SIZE,
  portalVehicleTitle,
} from "@/features/portal/lib/portal-vehicles";
import { pageCount } from "@/features/warranty/lib/warranty-list";
import { useLocale } from "@/providers/locale-provider";

/**
 * Portal > My vehicles (TEC-241): the vehicles the signed-in user owns
 * (GET /v1/portal/vehicles) with their service and active warranty counts.
 */
export function PortalVehicles() {
  const { t, format } = useLocale();
  const [page, setPage] = useState(0);
  const list = useQuery({
    queryKey: ["portal", "vehicles", page],
    queryFn: () =>
      portalApi.listVehicles(
        PORTAL_VEHICLE_PAGE_SIZE,
        page * PORTAL_VEHICLE_PAGE_SIZE,
      ),
    placeholderData: keepPreviousData,
  });
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const pages = pageCount(total, PORTAL_VEHICLE_PAGE_SIZE);

  return (
    <PortalPage
      title={t("portal.vehicles.title")}
      icon={<Car className="size-6" />}
      testId="portal-vehicles-page"
    >
      {list.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void list.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : list.isLoading ? (
        <p className="text-muted-foreground text-sm">
          {t("portal.vehicles.loading")}
        </p>
      ) : rows.length === 0 ? (
        <Card>
          <CardContent
            className="flex flex-col items-center gap-2 py-10 text-center"
            data-testid="portal-vehicles-empty"
          >
            <Car className="text-muted-foreground size-10" />
            <p className="font-medium">{t("portal.vehicles.empty_title")}</p>
            <p className="text-muted-foreground max-w-sm text-sm">
              {t("portal.vehicles.empty_description")}
            </p>
          </CardContent>
        </Card>
      ) : (
        <ul className="grid gap-3 sm:grid-cols-2" data-testid="portal-vehicles">
          {rows.map((v) => (
            <li key={v.uuid} data-testid="portal-vehicle" data-uuid={v.uuid}>
              <Link
                href={routes.portal.vehicle(v.uuid)}
                className="group focus-visible:ring-ring block rounded-xl outline-none focus-visible:ring-2"
              >
                <Card className="group-hover:border-primary/50 h-full transition-colors">
                  <CardContent className="space-y-3 pt-6">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className="truncate font-medium">
                          {portalVehicleTitle(v) ||
                            t("portal.vehicles.unknown_vehicle")}
                        </p>
                        {v.plate ? (
                          <p
                            className="text-muted-foreground font-mono text-sm tracking-wider"
                            dir="ltr"
                          >
                            {v.plate}
                          </p>
                        ) : null}
                      </div>
                      <ChevronRight className="text-muted-foreground size-4 shrink-0 rtl:rotate-180" />
                    </div>
                    <div className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs">
                      <span className="inline-flex items-center gap-1">
                        <Wrench className="size-3" />
                        {t("portal.vehicles.service_count", {
                          count: v.service_count,
                        })}
                      </span>
                      <span
                        className="inline-flex items-center gap-1"
                        data-testid="active-warranty-count"
                      >
                        <ShieldCheck className="size-3" />
                        {t("portal.vehicles.active_warranty_count", {
                          count: v.active_warranty_count,
                        })}
                      </span>
                    </div>
                    <p className="text-muted-foreground text-xs">
                      {v.last_service_at
                        ? t("portal.vehicles.last_service", {
                            date: format.date(v.last_service_at),
                          })
                        : t("portal.vehicles.no_service")}
                    </p>
                  </CardContent>
                </Card>
              </Link>
            </li>
          ))}
        </ul>
      )}
      {pages > 1 ? (
        <div className="flex items-center justify-between gap-2">
          <p className="text-muted-foreground text-sm" data-testid="page-info">
            {t("warranty.list.page", { page: page + 1, pages, total })}
          </p>
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={page === 0 || list.isFetching}
              onClick={() => setPage((p) => Math.max(0, p - 1))}
            >
              {t("warranty.list.prev")}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={page + 1 >= pages || list.isFetching}
              onClick={() => setPage((p) => p + 1)}
            >
              {t("warranty.list.next")}
            </Button>
          </div>
        </div>
      ) : null}
    </PortalPage>
  );
}
