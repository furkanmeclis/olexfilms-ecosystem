"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Flame, Plus } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { leadInputClass } from "@/features/leads/components/lead-fields";
import {
  LEAD_PAGE_SIZE,
  LEAD_STATUSES,
  LEAD_TARGET_TYPES,
  leadName,
  leadStatusTone,
  leadTemperatureTone,
  listQuery,
  pageCount,
  type LeadListFilters,
} from "@/features/leads/lib/leads";
import {
  leadKeys,
  leadsService,
} from "@/features/leads/services/leads.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const ALL = "";

export function LeadsListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.leads.read);
  const canWrite = can(permissions.leads.write);
  const [filters, setFilters] = useState<LeadListFilters>({
    status: ALL,
    target_type: ALL,
    follow_up: ALL,
    q: "",
  });
  const [page, setPage] = useState(0);
  const query = useMemo(() => listQuery(filters, page), [filters, page]);
  const list = useQuery({
    queryKey: leadKeys.list(query),
    queryFn: () => leadsService.list(query),
    enabled: canRead,
    placeholderData: keepPreviousData,
  });

  const patch = (p: Partial<LeadListFilters>) => {
    setFilters((f) => ({ ...f, ...p }));
    setPage(0);
  };

  const title = t("leads.list.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Flame className="size-6" />}
      description={t("leads.list.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        canWrite ? (
          <Button asChild>
            <Link
              href={routes.tenant.leads.create(slug)}
              data-testid="lead-new"
            >
              <Plus className="size-4" />
              {t("leads.list.new")}
            </Link>
          </Button>
        ) : null
      }
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("leads.list.forbidden")}
        />
      </div>
    );
  }

  const total = list.data?.total ?? 0;
  const pages = pageCount(total, LEAD_PAGE_SIZE);
  const rows = list.data?.items ?? [];

  return (
    <div className="space-y-6">
      {header}
      <Card>
        <CardContent className="space-y-4 pt-6">
          <div className="flex flex-wrap gap-2" role="tablist">
            {(["", "overdue", "today"] as const).map((value) => (
              <Button
                key={value || "all"}
                type="button"
                variant={filters.follow_up === value ? "default" : "outline"}
                size="sm"
                data-testid={`lead-tab-${value || "all"}`}
                onClick={() => patch({ follow_up: value })}
              >
                {t(`leads.follow_up.${value || "all"}`)}
              </Button>
            ))}
          </div>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <div className="space-y-1.5">
              <Label htmlFor="lead-search">{t("leads.list.search")}</Label>
              <input
                id="lead-search"
                data-testid="lead-search"
                className={leadInputClass}
                value={filters.q}
                onChange={(e) => patch({ q: e.target.value })}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="lead-filter-status">
                {t("leads.columns.status")}
              </Label>
              <select
                id="lead-filter-status"
                data-testid="lead-filter-status"
                className={leadInputClass}
                value={filters.status}
                onChange={(e) =>
                  patch({ status: e.target.value as LeadListFilters["status"] })
                }
              >
                <option value={ALL}>{t("leads.list.all")}</option>
                {LEAD_STATUSES.map((s) => (
                  <option key={s} value={s}>
                    {t(`leads.status.${s}`)}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="lead-filter-target">
                {t("leads.columns.target_type")}
              </Label>
              <select
                id="lead-filter-target"
                data-testid="lead-filter-target"
                className={leadInputClass}
                value={filters.target_type}
                onChange={(e) =>
                  patch({
                    target_type: e.target
                      .value as LeadListFilters["target_type"],
                  })
                }
              >
                <option value={ALL}>{t("leads.list.all")}</option>
                {LEAD_TARGET_TYPES.map((x) => (
                  <option key={x} value={x}>
                    {t(`leads.target_type.${x}`)}
                  </option>
                ))}
              </select>
            </div>
          </div>
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
                {t("leads.list.loading")}
              </p>
            ) : rows.length === 0 ? (
              <div className="py-8 text-center" data-testid="leads-empty">
                <p className="font-medium">{t("leads.list.empty_title")}</p>
                <p className="text-muted-foreground text-sm">
                  {t("leads.list.empty_description")}
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm" data-testid="leads-table">
                  <thead>
                    <tr className="text-muted-foreground border-b text-xs">
                      {[
                        "target_type",
                        "name",
                        "source",
                        "temperature",
                        "status",
                        "follow_up",
                        "assignee",
                      ].map((c) => (
                        <th key={c} className="p-2 text-start font-medium">
                          {t(`leads.columns.${c}`)}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((lead) => (
                      <tr
                        key={lead.uuid}
                        className="hover:bg-accent/50 border-b align-top last:border-0"
                        data-testid="lead-row"
                      >
                        <td className="p-2">
                          {t(`leads.target_type.${lead.target_type}`)}
                        </td>
                        <td className="p-2">
                          <Link
                            href={routes.tenant.leads.detail(slug, lead.uuid)}
                            className="font-medium hover:underline"
                          >
                            {leadName(lead)}
                          </Link>
                        </td>
                        <td className="p-2">
                          {t(`leads.source.${lead.source}`)}
                        </td>
                        <td className="p-2">
                          <StatusChip
                            label={t(`leads.temperature.${lead.temperature}`)}
                            tone={leadTemperatureTone(lead.temperature)}
                          />
                        </td>
                        <td className="p-2">
                          <StatusChip
                            label={t(`leads.status.${lead.status}`)}
                            tone={leadStatusTone(lead.status)}
                          />
                        </td>
                        <td className="text-muted-foreground p-2 text-xs whitespace-nowrap">
                          {lead.follow_up_date
                            ? format.dateTime(lead.follow_up_date)
                            : "—"}
                        </td>
                        <td className="p-2">
                          {lead.assignee_user_id
                            ? `#${lead.assignee_user_id}`
                            : t("leads.form.unassigned")}
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
              <p className="text-muted-foreground text-sm">
                {t("leads.list.page", { page: page + 1, pages, total })}
              </p>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={page === 0 || list.isFetching}
                  onClick={() => setPage((p) => Math.max(0, p - 1))}
                >
                  <ChevronLeft className="size-4 rtl:rotate-180" />
                  {t("leads.list.prev")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={page + 1 >= pages || list.isFetching}
                  onClick={() => setPage((p) => p + 1)}
                >
                  {t("leads.list.next")}
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
