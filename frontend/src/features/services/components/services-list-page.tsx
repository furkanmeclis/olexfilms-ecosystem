"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, ClipboardList, Plus } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { routes } from "@/config/routes";
import { resolveServiceListAccess } from "@/features/services/lib/access";
import {
  customerName,
  serviceStatusTone,
  vehicleTitle,
} from "@/features/services/lib/detail";
import {
  ALL_STATUSES,
  EMPTY_SERVICE_FILTERS,
  SERVICE_STATUSES,
  buildServiceListQuery,
  invalidDateRange,
  pageCount,
  type ServiceListFilters,
} from "@/features/services/lib/list-filters";
import {
  serviceWizardKeys,
  serviceWizardService,
} from "@/features/services/services/service-wizard.service";
import { useDebounce } from "@/hooks/use-debounce";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const SERVICE_PAGE_SIZE = 20;

/**
 * Tenant > Services (TEC-183): services of the services.read scope with
 * status, created date and plate / customer / number search, paged.
 */
export function ServicesListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const access = resolveServiceListAccess(can);
  const [filters, setFilters] = useState<ServiceListFilters>(
    EMPTY_SERVICE_FILTERS,
  );
  const [page, setPage] = useState(0);
  const q = useDebounce(filters.q, 300);

  const query = useMemo(
    () =>
      buildServiceListQuery(
        { ...filters, q },
        { limit: SERVICE_PAGE_SIZE, offset: page * SERVICE_PAGE_SIZE },
      ),
    [filters, q, page],
  );

  const list = useQuery({
    queryKey: serviceWizardKeys.list(query),
    queryFn: () => serviceWizardService.listServices(query),
    enabled: access.canRead,
    placeholderData: keepPreviousData,
  });

  const change = (patch: Partial<ServiceListFilters>) => {
    setFilters((f) => ({ ...f, ...patch }));
    setPage(0);
  };

  const title = t("services.list.title");
  const header = (
    <PageHeader
      title={title}
      icon={<ClipboardList className="size-6" />}
      description={t("services.list.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        access.canCreate ? (
          <Button asChild>
            <Link
              href={routes.tenant.services.create(slug)}
              data-testid="new-service"
            >
              <Plus className="size-4" />
              {t("services.nav_new")}
            </Link>
          </Button>
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

  const total = list.data?.total ?? 0;
  const pages = pageCount(total, SERVICE_PAGE_SIZE);
  const rows = list.data?.items ?? [];
  const badRange = invalidDateRange(filters.from, filters.to);
  const filtered =
    filters.status !== ALL_STATUSES ||
    filters.from !== "" ||
    filters.to !== "" ||
    filters.q.trim() !== "";

  return (
    <div className="space-y-6">
      {header}
      <Card>
        <CardContent className="space-y-4 pt-6" data-testid="service-filters">
          <div
            className="flex flex-wrap gap-2"
            role="group"
            aria-label={t("services.list.status")}
          >
            {[ALL_STATUSES, ...SERVICE_STATUSES].map((status) => {
              const active = filters.status === status;
              return (
                <Button
                  key={status}
                  type="button"
                  size="sm"
                  variant={active ? "default" : "outline"}
                  aria-pressed={active}
                  data-status={status}
                  onClick={() =>
                    change({ status: status as ServiceListFilters["status"] })
                  }
                >
                  {status === ALL_STATUSES
                    ? t("services.list.all_statuses")
                    : t(`services.status.${status}`)}
                </Button>
              );
            })}
          </div>
          <div className="grid gap-3 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)]">
            <div className="space-y-1.5">
              <Label htmlFor="service-search">
                {t("services.list.search")}
              </Label>
              <Input
                id="service-search"
                type="search"
                value={filters.q}
                maxLength={100}
                placeholder={t("services.list.search_placeholder")}
                onChange={(e) => change({ q: e.target.value })}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="service-from">
                {t("services.list.date_from")}
              </Label>
              <DatePicker
                id="service-from"
                value={filters.from}
                placeholder={t("services.list.date_from")}
                onChange={(from) => change({ from })}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="service-to">{t("services.list.date_to")}</Label>
              <DatePicker
                id="service-to"
                value={filters.to}
                placeholder={t("services.list.date_to")}
                aria-invalid={badRange || undefined}
                onChange={(to) => change({ to })}
              />
            </div>
          </div>
          {badRange ? (
            <p className="text-destructive text-sm" data-testid="date-error">
              {t("services.list.date_invalid")}
            </p>
          ) : null}
          {filtered ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              data-testid="clear-filters"
              onClick={() => {
                setFilters(EMPTY_SERVICE_FILTERS);
                setPage(0);
              }}
            >
              {t("services.list.clear_filters")}
            </Button>
          ) : null}
        </CardContent>
      </Card>

      {list.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void list.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <Card>
          <CardContent className="pt-6">
            {list.isLoading ? (
              <p className="text-muted-foreground text-sm">
                {t("services.wizard.loading")}
              </p>
            ) : rows.length === 0 ? (
              <div className="py-8 text-center" data-testid="services-empty">
                <p className="font-medium">{t("services.list.empty_title")}</p>
                <p className="text-muted-foreground text-sm">
                  {filtered
                    ? t("services.list.empty_filtered")
                    : t("services.list.empty_description")}
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm" data-testid="services-table">
                  <thead>
                    <tr className="text-muted-foreground border-b text-start text-xs">
                      <th className="p-2 text-start font-medium">
                        {t("services.list.columns.service_no")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("services.list.columns.status")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("services.list.columns.customer")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("services.list.columns.vehicle")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("services.list.columns.organization")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("services.list.columns.created_at")}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((s) => (
                      <tr
                        key={s.uuid}
                        className="hover:bg-accent/50 border-b last:border-0"
                        data-testid="service-row"
                        data-uuid={s.uuid}
                      >
                        <td className="p-2">
                          <Link
                            href={routes.tenant.services.detail(slug, s.uuid)}
                            className="font-mono font-medium hover:underline"
                            dir="ltr"
                          >
                            {s.service_no}
                          </Link>
                        </td>
                        <td className="p-2">
                          <StatusChip
                            label={s.status_label}
                            tone={serviceStatusTone(s.status)}
                          />
                        </td>
                        <td className="p-2">{customerName(s)}</td>
                        <td className="p-2">
                          <div>{vehicleTitle(s)}</div>
                          {s.plate ? (
                            <div
                              className="text-muted-foreground font-mono text-xs"
                              dir="ltr"
                            >
                              {s.plate}
                            </div>
                          ) : null}
                        </td>
                        <td className="p-2">{s.organization.name}</td>
                        <td className="p-2 whitespace-nowrap">
                          {format.dateTime(s.created_at)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            <div
              className={cn(
                "mt-4 flex flex-wrap items-center justify-between gap-2",
                rows.length === 0 && page === 0 && "hidden",
              )}
            >
              <p
                className="text-muted-foreground text-sm"
                data-testid="page-info"
              >
                {t("services.list.page", {
                  page: page + 1,
                  pages,
                  total,
                })}
              </p>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  data-testid="page-prev"
                  disabled={page === 0 || list.isFetching}
                  onClick={() => setPage((p) => Math.max(0, p - 1))}
                >
                  <ChevronLeft className="size-4 rtl:rotate-180" />
                  {t("services.list.prev")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  data-testid="page-next"
                  disabled={page + 1 >= pages || list.isFetching}
                  onClick={() => setPage((p) => p + 1)}
                >
                  {t("services.list.next")}
                  <ChevronRight className="size-4 rtl:rotate-180" />
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
