"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { BarChart3, Car, ChevronRight, Package } from "lucide-react";
import Link from "next/link";
import { useState, type ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
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
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  HOME_OVERVIEW_DEFAULT_PERIOD,
  HOME_OVERVIEW_PERIODS,
  homeOverviewService,
  type HomeOverview,
  type HomeOverviewPeriod,
  type OverviewRecentService,
} from "@/features/services/services/home-overview.service";
import { VehicleBrandLogo } from "@/features/vehicle-catalog/components/vehicle-brand-logo";
import { logoVersion } from "@/features/vehicle-catalog/lib/images";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const PERIOD_LABELS: Record<HomeOverviewPeriod, string> = {
  "30d": "dashboard.top_vehicles.period.30d",
  "90d": "dashboard.top_vehicles.period.90d",
  "12m": "dashboard.top_vehicles.period.12m",
  all: "dashboard.top_vehicles.period.all",
};

type RankedRow = {
  key: string;
  label: string;
  count: number;
  leading: ReactNode;
};

/** One ranked list (rank, label, count and a bar relative to the first). */
function RankedList({
  title,
  icon,
  rows,
  testId,
}: {
  title: string;
  icon: ReactNode;
  rows: RankedRow[];
  testId: string;
}) {
  const { t } = useLocale();
  const max = rows.reduce((m, r) => Math.max(m, r.count), 0);
  return (
    <section className="min-w-0 space-y-3" data-testid={testId}>
      <h3 className="flex items-center gap-2 text-sm font-medium">
        <span className="text-muted-foreground [&_svg]:size-4" aria-hidden>
          {icon}
        </span>
        {title}
      </h3>
      {rows.length === 0 ? (
        <p className="text-muted-foreground py-6 text-center text-sm">
          {t("dashboard.overview.empty")}
        </p>
      ) : (
        <ol className="space-y-3">
          {rows.map((row, i) => (
            <li key={row.key} className="space-y-1.5" data-rank={i + 1}>
              <div className="flex min-w-0 items-center gap-2 text-sm">
                <span className="text-muted-foreground w-4 shrink-0 text-xs tabular-nums">
                  {i + 1}
                </span>
                {row.leading}
                <span className="min-w-0 flex-1 truncate" title={row.label}>
                  {row.label}
                </span>
                <span
                  className="shrink-0 font-medium tabular-nums"
                  aria-label={`${t("dashboard.overview.count")}: ${row.count}`}
                >
                  {row.count}
                </span>
              </div>
              <div className="bg-muted ms-6 h-1.5 overflow-hidden rounded-full">
                <div
                  className="bg-chart-1 h-full rounded-full"
                  style={{
                    width: `${max > 0 ? Math.max(4, (row.count / max) * 100) : 0}%`,
                  }}
                />
              </div>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

function RecentServices({
  slug,
  items,
}: {
  slug: string;
  items: readonly OverviewRecentService[];
}) {
  const { t, format } = useLocale();
  return (
    <section className="min-w-0 space-y-3" data-testid="home-overview-recent">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="flex items-center gap-2 text-sm font-medium">
          <Car className="text-muted-foreground size-4" aria-hidden />
          {t("dashboard.overview.recent_title")}
        </h3>
        <Link
          href={routes.tenant.services.list(slug)}
          className="text-primary text-sm underline-offset-4 hover:underline"
        >
          {t("dashboard.overview.view_all")}
        </Link>
      </div>
      {items.length === 0 ? (
        <p className="text-muted-foreground py-6 text-center text-sm">
          {t("dashboard.overview.recent_empty")}
        </p>
      ) : (
        <ul className="divide-y rounded-lg border">
          {items.map((s) => {
            const vehicle = [s.car_brand.name, s.car_model.name, s.model_year]
              .filter(Boolean)
              .join(" ");
            const content = s.products.length
              ? s.products.join(", ")
              : (s.package ?? "");
            return (
              <li key={s.uuid} data-service={s.uuid}>
                <Link
                  href={routes.tenant.services.detail(slug, s.uuid)}
                  className="hover:bg-muted/50 focus-visible:ring-ring flex min-w-0 items-center gap-3 px-3 py-2.5 outline-none focus-visible:ring-2"
                >
                  <VehicleBrandLogo
                    uuid={s.car_brand.uuid}
                    name={s.car_brand.name}
                    version={
                      s.car_brand.has_logo
                        ? logoVersion(s.car_brand.logo_url)
                        : null
                    }
                    height={28}
                  />
                  <span className="min-w-0 flex-1">
                    <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                      <Badge variant="outline" className="font-mono">
                        {s.plate ?? t("dashboard.overview.no_plate")}
                      </Badge>
                      <span className="truncate text-sm font-medium">
                        {vehicle}
                      </span>
                    </span>
                    {content ? (
                      <span
                        className="text-muted-foreground block truncate text-xs"
                        title={content}
                      >
                        {content}
                      </span>
                    ) : null}
                  </span>
                  <time
                    dateTime={s.completed_at}
                    className="text-muted-foreground shrink-0 text-xs tabular-nums"
                  >
                    {format.date(s.completed_at)}
                  </time>
                  <ChevronRight
                    className="text-muted-foreground size-4 shrink-0 rtl:rotate-180"
                    aria-hidden
                  />
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

export type HomeOverviewCardProps = {
  slug: string;
  data: HomeOverview | undefined;
  period: HomeOverviewPeriod;
  onPeriodChange: (period: HomeOverviewPeriod) => void;
  isLoading?: boolean;
  isError?: boolean;
  onRetry?: () => void;
};

/**
 * TEC-868 dealer home "Analiz": most serviced car models and most used
 * products of the period side by side, then the latest completed services
 * with links to the service. Bars use the --chart-1 token.
 */
export function HomeOverviewCard({
  slug,
  data,
  period,
  onPeriodChange,
  isLoading,
  isError,
  onRetry,
}: HomeOverviewCardProps) {
  const { t } = useLocale();
  const vehicles: RankedRow[] = (data?.top_vehicles ?? []).map((v) => ({
    key: v.car_model.uuid,
    label: `${v.car_brand.name} ${v.car_model.name}`,
    count: v.service_count,
    leading: (
      <VehicleBrandLogo
        uuid={v.car_brand.uuid}
        name={v.car_brand.name}
        version={
          v.car_brand.has_logo ? logoVersion(v.car_brand.logo_url) : null
        }
        height={20}
      />
    ),
  }));
  const products: RankedRow[] = (data?.top_products ?? []).map((p) => ({
    key: p.uuid,
    label: p.name,
    count: p.service_count,
    leading: null,
  }));

  return (
    <Card className="xl:col-span-2" data-testid="home-overview">
      <CardHeader className="max-sm:has-data-[slot=card-action]:grid-cols-1">
        <CardTitle className="flex items-center gap-2">
          <BarChart3 className="size-5" aria-hidden />
          {t("dashboard.overview.title")}
        </CardTitle>
        <CardDescription>{t("dashboard.overview.description")}</CardDescription>
        {/* On a phone the period switch goes under the description. */}
        <CardAction className="max-sm:col-start-1 max-sm:row-span-1 max-sm:row-start-3 max-sm:justify-self-start">
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={period}
            aria-label={t("dashboard.overview.period_label")}
            onValueChange={(v) => v && onPeriodChange(v as HomeOverviewPeriod)}
          >
            {HOME_OVERVIEW_PERIODS.map((p) => (
              <ToggleGroupItem key={p} value={p} data-period={p}>
                {t(PERIOD_LABELS[p])}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </CardAction>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div
            aria-busy="true"
            data-testid="home-overview-loading"
            className="grid gap-6 md:grid-cols-2"
          >
            {[0, 1].map((col) => (
              <div key={col} className="space-y-3">
                {Array.from({ length: 4 }, (_, i) => (
                  <Skeleton key={i} className="h-5" />
                ))}
              </div>
            ))}
          </div>
        ) : isError ? (
          <div
            role="alert"
            data-testid="home-overview-error"
            className="text-muted-foreground flex flex-col items-center gap-2 py-10 text-center text-sm"
          >
            <p>{t("dashboard.overview.error")}</p>
            {onRetry ? (
              <button
                type="button"
                className="text-primary underline-offset-4 hover:underline"
                onClick={onRetry}
              >
                {t("common.retry")}
              </button>
            ) : null}
          </div>
        ) : (
          <div className="space-y-8">
            <div className="grid gap-8 md:grid-cols-2">
              <RankedList
                title={t("dashboard.overview.top_vehicles")}
                icon={<Car />}
                rows={vehicles}
                testId="home-overview-vehicles"
              />
              <RankedList
                title={t("dashboard.overview.top_products")}
                icon={<Package />}
                rows={products}
                testId="home-overview-products"
              />
            </div>
            <RecentServices slug={slug} items={data?.recent_services ?? []} />
          </div>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * Dealer home analytics widget: renders only in a dealer organization for
 * a services.read holder (the center home has the brand-wide top chart).
 */
export function HomeOverviewWidget({ slug }: { slug: string }) {
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const visible = can(permissions.services.read) && org?.type === "dealer";
  const [period, setPeriod] = useState<HomeOverviewPeriod>(
    HOME_OVERVIEW_DEFAULT_PERIOD,
  );
  const query = useQuery({
    queryKey: ["services", "home-overview", slug, period] as const,
    queryFn: () => homeOverviewService.get(period),
    enabled: visible,
    placeholderData: keepPreviousData,
    staleTime: 60_000,
  });
  if (!visible) return null;
  return (
    <HomeOverviewCard
      slug={slug}
      data={query.data}
      period={period}
      onPeriodChange={setPeriod}
      isLoading={query.isPending}
      isError={query.isError}
      onRetry={() => void query.refetch()}
    />
  );
}
