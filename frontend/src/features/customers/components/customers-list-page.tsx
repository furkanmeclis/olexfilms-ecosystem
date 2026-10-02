"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Plus, Search, Users } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { routes } from "@/config/routes";
import { CustomerListExportButton } from "@/features/customers/components/customer-list-export";
import { resolveCustomerListAccess } from "@/features/customers/lib/access";
import { customerDisplayName } from "@/features/customers/lib/form";
import {
  customerKeys,
  customersService,
  type CustomerListQuery,
  type CustomerStatus,
} from "@/features/customers/services/customers.service";
import { useDebounce } from "@/hooks/use-debounce";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const CUSTOMER_PAGE_SIZE = 20;
const STATUSES: CustomerStatus[] = [
  "active",
  "pending",
  "disabled",
  "anonymized",
];

export function customerStatusTone(
  status: CustomerStatus,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "active":
      return "success";
    case "pending":
      return "warning";
    case "disabled":
      return "danger";
    default:
      return "default";
  }
}

export function customerPageCount(total: number, size: number): number {
  return Math.max(1, Math.ceil(total / size));
}

/**
 * Tenant > Customers (TEC-163): customers linked to the organizations in
 * scope, searched by name / phone / e-mail (`q`), filtered by status, paged.
 */
export function CustomersListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const access = resolveCustomerListAccess(can);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<CustomerStatus | "">("");
  const [page, setPage] = useState(0);
  const q = useDebounce(search.trim(), 300);

  const query: CustomerListQuery = {
    ...(q ? { q } : {}),
    ...(status ? { status } : {}),
    limit: CUSTOMER_PAGE_SIZE,
    offset: page * CUSTOMER_PAGE_SIZE,
  };
  const list = useQuery({
    queryKey: customerKeys.list(query),
    queryFn: () => customersService.list(query),
    enabled: access.canRead,
    placeholderData: keepPreviousData,
  });

  const title = t("customers.list.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Users className="size-6" />}
      description={t("customers.list.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        access.canExport || access.canCreate ? (
          <div className="flex flex-wrap gap-2">
            {access.canExport ? (
              <CustomerListExportButton
                filters={{
                  ...(q ? { q } : {}),
                  ...(status ? { status } : {}),
                }}
              />
            ) : null}
            {access.canCreate ? (
              <Button asChild>
                <Link
                  href={routes.tenant.customers.create(slug)}
                  data-testid="new-customer"
                >
                  <Plus className="size-4" />
                  {t("customers.nav_new")}
                </Link>
              </Button>
            ) : null}
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
          description={t("customers.list.forbidden")}
        />
      </div>
    );
  }

  const total = list.data?.total ?? 0;
  const pages = customerPageCount(total, CUSTOMER_PAGE_SIZE);
  const rows = list.data?.items ?? [];
  const filtered = Boolean(q || status);

  return (
    <div className="space-y-6">
      {header}
      <Card>
        <CardContent className="space-y-4 pt-6" data-testid="customer-filters">
          <div className="space-y-1.5">
            <Label htmlFor="customer-search">
              {t("customers.list.search")}
            </Label>
            <div className="relative">
              <Search className="text-muted-foreground pointer-events-none absolute start-3 top-1/2 size-4 -translate-y-1/2" />
              <Input
                id="customer-search"
                type="search"
                className="ps-9"
                value={search}
                placeholder={t("customers.list.search_placeholder")}
                onChange={(e) => {
                  setSearch(e.target.value);
                  setPage(0);
                }}
              />
            </div>
          </div>
          <div
            className="flex flex-wrap gap-2"
            role="group"
            aria-label={t("customers.list.status")}
          >
            {(["", ...STATUSES] as const).map((s) => {
              const active = status === s;
              return (
                <Button
                  key={s || "all"}
                  type="button"
                  size="sm"
                  variant={active ? "default" : "outline"}
                  aria-pressed={active}
                  data-status={s || "all"}
                  onClick={() => {
                    setStatus(s);
                    setPage(0);
                  }}
                >
                  {s ? t(`customers.status.${s}`) : t("customers.list.all")}
                </Button>
              );
            })}
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
                {t("customers.loading")}
              </p>
            ) : rows.length === 0 ? (
              <div className="py-8 text-center" data-testid="customers-empty">
                <p className="font-medium">{t("customers.list.empty_title")}</p>
                <p className="text-muted-foreground text-sm">
                  {filtered
                    ? t("customers.list.empty_filtered")
                    : t("customers.list.empty_description")}
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm" data-testid="customers-table">
                  <thead>
                    <tr className="text-muted-foreground border-b text-xs">
                      <th className="p-2 text-start font-medium">
                        {t("customers.fields.name")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("customers.fields.phone")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("customers.fields.email")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("customers.fields.status")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("customers.fields.linked_at")}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((c) => (
                      <tr
                        key={c.uuid}
                        className="hover:bg-accent/50 border-b last:border-0"
                        data-testid="customer-row"
                        data-uuid={c.uuid}
                      >
                        <td className="p-2">
                          <Link
                            href={routes.tenant.customers.detail(slug, c.uuid)}
                            className="font-medium hover:underline"
                          >
                            {customerDisplayName(c)}
                          </Link>
                          {c.type === "corporate" && c.company_name ? (
                            <p className="text-muted-foreground text-xs">
                              {c.company_name}
                            </p>
                          ) : null}
                        </td>
                        <td className="p-2 whitespace-nowrap">
                          <span dir="ltr">{c.phone ?? "—"}</span>
                        </td>
                        <td className="p-2">{c.email ?? "—"}</td>
                        <td className="p-2">
                          <StatusChip
                            label={t(`customers.status.${c.status}`)}
                            tone={customerStatusTone(c.status)}
                          />
                        </td>
                        <td className="p-2 whitespace-nowrap">
                          {c.linked_at ? format.date(c.linked_at) : "—"}
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
                {t("customers.list.page", { page: page + 1, pages, total })}
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
                  {t("customers.list.prev")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  data-testid="page-next"
                  disabled={page + 1 >= pages || list.isFetching}
                  onClick={() => setPage((p) => p + 1)}
                >
                  {t("customers.list.next")}
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
