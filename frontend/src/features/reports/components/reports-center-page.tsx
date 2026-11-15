"use client";

import {
  ChartColumn,
  ChartLine,
  ChartPie,
  ChevronRight,
  LayoutList,
  Search,
  Sigma,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { useEnabledFeatures } from "@/features/modules/hooks/use-features";
import {
  groupByCategory,
  reportCategory,
  visibleSpecialReports,
  type ReportCategory,
} from "@/features/reports/lib/catalog";
import {
  CATEGORY_LABEL_KEYS,
  KIND_LABEL_KEYS,
} from "@/features/reports/lib/labels";
import { useReportCatalog } from "@/features/reports/hooks/use-report";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** One tile of the center: a generic report or a dedicated page. */
type Entry = {
  key: string;
  title: string;
  description?: string;
  kind?: string;
  href: string;
  category: ReportCategory;
};

const KIND_ICONS: Record<string, LucideIcon> = {
  summary: Sigma,
  timeseries: ChartLine,
  distribution: ChartPie,
  ranking: ChartColumn,
  table: LayoutList,
};

/**
 * TEC-1212: reports center. Lists the backend catalog (already filtered by
 * permission, module and reach) and the dedicated report pages (accounting,
 * warranty claims, efficiency, performance) by category, with a search.
 */
export function ReportsCenterPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const features = useEnabledFeatures(slug);
  const [q, setQ] = useState("");

  const catalog = useReportCatalog(slug);

  const entries = useMemo<Entry[]>(() => {
    const generic: Entry[] = (catalog.data?.items ?? []).map((item) => ({
      key: item.key,
      title: item.title,
      kind: item.kind,
      href: routes.tenant.reports.detail(slug, item.key),
      category: reportCategory(item.key),
    }));
    const special: Entry[] = visibleSpecialReports(
      can,
      org?.type,
      features,
    ).map((r) => ({
      key: r.key,
      title: t(r.titleKey),
      description: t(r.descriptionKey),
      href: r.href(slug),
      category: r.category,
    }));
    return [...generic, ...special];
  }, [catalog.data, can, org?.type, features, slug, t]);

  const filtered = useMemo(() => {
    const needle = q.trim().toLocaleLowerCase();
    if (!needle) return entries;
    return entries.filter((e) =>
      `${e.title} ${e.description ?? ""}`
        .toLocaleLowerCase()
        .includes(needle),
    );
  }, [entries, q]);
  const groups = groupByCategory(filtered, (e) => e.category);

  return (
    <div className="flex flex-col gap-6" data-testid="reports-center">
      <PageHeader
        title={t("reports.page.title")}
        description={t("reports.page.description")}
        actions={
          <div className="relative w-full sm:w-72">
            <Search
              className="text-muted-foreground pointer-events-none absolute start-2.5 top-1/2 size-4 -translate-y-1/2"
              aria-hidden
            />
            <Input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder={t("reports.page.search")}
              aria-label={t("reports.page.search")}
              className="ps-8"
            />
          </div>
        }
      />
      {catalog.isPending ? (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className="h-20 rounded-xl" />
          ))}
        </div>
      ) : catalog.isError && entries.length === 0 ? (
        <ErrorState
          title={t("reports.page.error")}
          onRetry={() => void catalog.refetch()}
          retryLabel={t("reports.retry")}
        />
      ) : groups.length === 0 ? (
        <EmptyState
          title={q ? t("reports.page.no_match") : t("reports.page.empty")}
        />
      ) : (
        groups.map((group) => (
          <section
            key={group.category}
            className="flex flex-col gap-3"
            data-testid={`reports-category-${group.category}`}
            aria-labelledby={`reports-category-${group.category}-title`}
          >
            <h2
              id={`reports-category-${group.category}-title`}
              className="text-muted-foreground text-sm font-semibold tracking-wide uppercase"
            >
              {t(CATEGORY_LABEL_KEYS[group.category])}
            </h2>
            <ul className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              {group.items.map((entry) => {
                const Icon = (entry.kind && KIND_ICONS[entry.kind]) || ChartColumn;
                return (
                  <li key={entry.key}>
                    <Link
                      href={entry.href}
                      data-report={entry.key}
                      className="bg-card hover:bg-accent/50 focus-visible:ring-ring flex h-full items-center gap-3 rounded-xl border p-4 transition-colors focus-visible:ring-2 focus-visible:outline-none"
                    >
                      <span className="bg-muted text-muted-foreground inline-flex size-9 shrink-0 items-center justify-center rounded-lg">
                        <Icon className="size-4" aria-hidden />
                      </span>
                      <span className="flex min-w-0 flex-1 flex-col gap-1">
                        <span className="truncate font-medium">
                          {entry.title}
                        </span>
                        {entry.description ? (
                          <span className="text-muted-foreground line-clamp-2 text-xs">
                            {entry.description}
                          </span>
                        ) : entry.kind ? (
                          <Badge variant="secondary" className="w-fit">
                            {t(KIND_LABEL_KEYS[entry.kind] ?? "reports.kind.table")}
                          </Badge>
                        ) : null}
                      </span>
                      <ChevronRight
                        className="text-muted-foreground size-4 shrink-0 rtl:rotate-180"
                        aria-hidden
                      />
                    </Link>
                  </li>
                );
              })}
            </ul>
          </section>
        ))
      )}
    </div>
  );
}
