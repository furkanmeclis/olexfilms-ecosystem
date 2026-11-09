"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ChevronRight } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { StatsCard } from "@/components/common/stats-card";
import { StatusChip } from "@/components/common/status-chip";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { routes } from "@/config/routes";
import { TopProductsChart } from "@/features/center-overview/components/top-products-chart";
import {
  CENTER_DEFAULT_PERIOD,
  CENTER_PERIODS,
  CENTER_RECENT_LIMIT,
  centerOverviewService,
  centerPeriodStart,
  overviewCard,
  type CenterPeriod,
} from "@/features/center-overview/services/center-overview.service";
import { serviceStatusTone } from "@/features/services/lib/detail";
import {
  serviceWizardKeys,
  serviceWizardService,
} from "@/features/services/services/service-wizard.service";
import { TopVehicleModelsWidget } from "@/features/vehicle-catalog/components/top-vehicle-models-widget";
import { canSeeTopVehicles } from "@/features/vehicle-catalog/lib/top-vehicles";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const PERIOD_LABELS: Record<CenterPeriod, string> = {
  today: "dashboard.center.period.today",
  "7d": "dashboard.center.period.7d",
  "30d": "dashboard.center.period.30d",
  "90d": "dashboard.center.period.90d",
  "12m": "dashboard.center.period.12m",
};

/**
 * TEC-1072: center home overview (legacy hub dashboard): dealer and
 * distributor counts, services opened and warranties started in the
 * period, the latest opened services, and the top vehicle models / most
 * used products charts. One period (today, 7 / 30 / 90 days, 12 months)
 * drives every card and chart. Visible where the top vehicles widget is
 * (a brand-wide services.read grant in the center, or super_admin).
 */
export function CenterOverviewSection({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { scopeFor } = usePermission();
  const org = useActiveOrganization(slug);
  const visible = canSeeTopVehicles(scopeFor, org?.type);
  const [period, setPeriod] = useState<CenterPeriod>(CENTER_DEFAULT_PERIOD);

  const overview = useQuery({
    queryKey: ["center-overview", slug, "overview", period] as const,
    queryFn: () => centerOverviewService.overview(period),
    enabled: visible,
    placeholderData: keepPreviousData,
    staleTime: 60_000,
  });
  const products = useQuery({
    queryKey: ["center-overview", slug, "top-products", period] as const,
    queryFn: () => centerOverviewService.topProducts(period),
    enabled: visible,
    placeholderData: keepPreviousData,
    staleTime: 60_000,
  });
  // The day is fixed per render of the period so the query key is stable.
  const createdFrom = useMemo(
    () => centerPeriodStart(period, new Date()).toISOString(),
    [period],
  );
  const recentParams = useMemo(
    () => ({
      limit: CENTER_RECENT_LIMIT,
      offset: 0,
      sort: "-created_at",
      created_from: createdFrom,
    }),
    [createdFrom],
  );
  const recent = useQuery({
    queryKey: serviceWizardKeys.list(recentParams),
    queryFn: () => serviceWizardService.listServices(recentParams),
    enabled: visible,
    placeholderData: keepPreviousData,
    staleTime: 60_000,
  });

  if (!visible) return null;

  const card = (key: string) => {
    const v = overviewCard(overview.data, key);
    return v === null ? "—" : format.number(v);
  };
  const activeWarranties = overviewCard(overview.data, "warranties_active");

  return (
    <section className="flex flex-col gap-6" data-testid="center-overview">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">{t("dashboard.center.title")}</h2>
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          value={period}
          aria-label={t("dashboard.center.period_label")}
          onValueChange={(v) => v && setPeriod(v as CenterPeriod)}
        >
          {CENTER_PERIODS.map((p) => (
            <ToggleGroupItem key={p} value={p} data-period={p}>
              {t(PERIOD_LABELS[p])}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </div>

      {overview.isPending ? (
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          {Array.from({ length: 4 }, (_, i) => (
            <Skeleton key={i} className="h-[104px] rounded-xl" />
          ))}
        </div>
      ) : overview.isError ? null : (
        <div
          className="grid grid-cols-2 gap-3 lg:grid-cols-4"
          data-testid="center-overview-cards"
        >
          <StatsCard
            title={t("dashboard.center.cards.dealers")}
            value={card("dealers_total")}
            hint={t("dashboard.center.cards.dealers_hint", {
              active: card("dealers_active"),
            })}
          />
          <StatsCard
            title={t("dashboard.center.cards.distributors")}
            value={card("distributors_total")}
          />
          <StatsCard
            title={t("dashboard.center.cards.services")}
            value={card("services_created")}
            hint={t("dashboard.center.cards.services_hint", {
              completed: card("services_completed"),
            })}
          />
          <StatsCard
            title={t("dashboard.center.cards.warranties")}
            value={card("warranties_started")}
            hint={
              activeWarranties === null
                ? undefined
                : t("dashboard.center.cards.warranties_hint", {
                    active: format.number(activeWarranties),
                  })
            }
          />
        </div>
      )}

      <div className="grid gap-6 xl:grid-cols-2">
        <TopVehicleModelsWidget slug={slug} period={period} />
        <TopProductsChart
          points={products.data?.series[0]?.points}
          isLoading={products.isPending}
          isError={products.isError}
          onRetry={() => void products.refetch()}
        />
      </div>

      <Card data-testid="center-recent-services">
        <CardHeader>
          <CardTitle>{t("dashboard.center.recent.title")}</CardTitle>
          <CardDescription>
            {t("dashboard.center.recent.description")}
          </CardDescription>
          <CardAction>
            <Link
              href={routes.tenant.services.list(slug)}
              className="text-primary inline-flex items-center gap-1 text-sm underline-offset-4 hover:underline"
            >
              {t("dashboard.center.recent.view_all")}
              <ChevronRight className="size-4 rtl:rotate-180" aria-hidden />
            </Link>
          </CardAction>
        </CardHeader>
        <CardContent>
          {recent.isPending ? (
            <div className="flex flex-col gap-2">
              {Array.from({ length: 4 }, (_, i) => (
                <Skeleton key={i} className="h-10" />
              ))}
            </div>
          ) : recent.isError ? (
            <p role="alert" className="text-muted-foreground py-6 text-sm">
              {t("dashboard.center.error")}
            </p>
          ) : (recent.data?.items.length ?? 0) === 0 ? (
            <p className="text-muted-foreground py-6 text-center text-sm">
              {t("dashboard.center.recent.empty")}
            </p>
          ) : (
            <ul className="divide-y">
              {recent.data?.items.map((s) => (
                <li key={s.uuid} data-testid="center-recent-row">
                  <Link
                    href={routes.tenant.services.detail(slug, s.uuid)}
                    className="hover:bg-muted/50 flex min-w-0 items-center gap-3 py-2.5"
                  >
                    <span className="font-mono text-sm" dir="ltr">
                      {s.service_no}
                    </span>
                    <span className="min-w-0 flex-1 truncate text-sm">
                      {[s.car_brand.name, s.car_model.name]
                        .filter(Boolean)
                        .join(" ")}
                      {s.plate ? (
                        <span className="text-muted-foreground ms-2 font-mono">
                          {s.plate}
                        </span>
                      ) : null}
                    </span>
                    <span className="text-muted-foreground hidden truncate text-xs sm:inline">
                      {s.organization.name}
                    </span>
                    <span className="text-muted-foreground hidden text-xs whitespace-nowrap md:inline">
                      {format.dateTime(s.created_at)}
                    </span>
                    <StatusChip
                      label={s.status_label}
                      tone={serviceStatusTone(s.status)}
                    />
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </section>
  );
}
