"use client";

import { useQuery } from "@tanstack/react-query";
import { AlertTriangle } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useFeature } from "@/features/modules/hooks/use-features";
import {
  stockForecastKeys,
  stockForecastService,
} from "@/features/stock-forecast/services/stock-forecast.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export function StockForecastWidget({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const feature = useFeature(slug, "stock_forecast");
  const visible = can(Permission.StockForecastRead) && feature.enabled;
  const query = useQuery({
    queryKey: stockForecastKeys.widget,
    queryFn: () => stockForecastService.widget(),
    enabled: visible,
    staleTime: 60_000,
  });

  if (!visible) return null;

  return (
    <Card data-testid="stock-forecast-widget">
      <CardHeader className="flex flex-row items-start justify-between gap-2">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2">
            <AlertTriangle className="size-4" />
            {t("stock_forecast.widget.title")}
          </CardTitle>
          <CardDescription>
            {t("stock_forecast.widget.description")}
          </CardDescription>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href={routes.tenant.stockForecast.list(slug)}>
            {t("stock_forecast.widget.open")}
          </Link>
        </Button>
      </CardHeader>
      <CardContent>
        {query.isLoading ? (
          <Skeleton className="h-16" />
        ) : (
          <div className="rounded-md border p-4">
            <div className="text-muted-foreground text-xs">
              {t("stock_forecast.widget.critical")}
            </div>
            <div className="text-3xl font-semibold tabular-nums">
              {format.number(query.data?.critical_count ?? 0)}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
