"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ArrowLeft, ExternalLink, ShieldCheck } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import { portalApi } from "@/features/portal/lib/portal-client";
import { PortalClaimBadge } from "@/features/warranty-claims/components/portal-claim-badge";
import { latestClaimByWarranty } from "@/features/warranty-claims/lib/claims";
import { WarrantyFilterBar } from "@/features/warranty/components/warranty-filter-bar";
import { WarrantyCertificateButton } from "@/features/warranty/components/warranty-certificate-button";
import { WarrantyProgressBar } from "@/features/warranty/components/warranty-progress";
import {
  buildWarrantyListQuery,
  EMPTY_WARRANTY_FILTERS,
  hasWarrantyFilters,
  pageCount,
  PORTAL_WARRANTY_SORTS,
  type PortalWarrantySort,
  warrantyStatusTone,
  warrantyVehicleTitle,
  type WarrantyListFilters,
} from "@/features/warranty/lib/warranty-list";
import { portalCertificateClient } from "@/features/warranty/services/certificate.service";
import { useDebounce } from "@/hooks/use-debounce";
import { useLocale } from "@/providers/locale-provider";

export const PORTAL_WARRANTY_PAGE_SIZE = 10;

const PORTAL_SORT_LABELS: Record<PortalWarrantySort, string> = {
  expiry: "warranty.portal.sort.expiry",
  "-start_at": "warranty.portal.sort.desc_start_at",
  start_at: "warranty.portal.sort.start_at",
};

/**
 * Portal > My warranties (TEC-191): only the warranties the signed-in
 * customer / fleet user holds (GET /v1/portal/warranties), with days left
 * and the elapsed-period bar. Stays a card list (portal customer cards are
 * a DataTable exception); TEC-378 adds the sort choice of TEC-377.
 */
export function PortalWarranties() {
  const { t, format, locale } = useLocale();
  const [filters, setFilters] = useState<WarrantyListFilters>(
    EMPTY_WARRANTY_FILTERS,
  );
  const [sort, setSort] = useState<PortalWarrantySort>("expiry");
  const [page, setPage] = useState(0);
  const q = useDebounce(filters.q, 300);
  const query = useMemo(
    () => ({
      ...buildWarrantyListQuery(
        { ...filters, q, productUuid: "" },
        {
          limit: PORTAL_WARRANTY_PAGE_SIZE,
          offset: page * PORTAL_WARRANTY_PAGE_SIZE,
        },
      ),
      sort,
    }),
    [filters, q, page, sort],
  );
  const list = useQuery({
    queryKey: ["portal", "warranties", query],
    queryFn: () => portalApi.listWarranties(query),
    placeholderData: keepPreviousData,
  });

  // TEC-339: claim status per warranty (status + date only).
  const claims = useQuery({
    queryKey: ["portal", "warranty-claims"],
    queryFn: () => portalApi.listWarrantyClaims(),
  });
  const claimByWarranty = useMemo(
    () => latestClaimByWarranty(claims.data?.items ?? []),
    [claims.data],
  );

  const change = (patch: Partial<WarrantyListFilters>) => {
    setFilters((f) => ({ ...f, ...patch }));
    setPage(0);
  };
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const pages = pageCount(total, PORTAL_WARRANTY_PAGE_SIZE);

  return (
    <div className="mx-auto w-full max-w-3xl space-y-6 px-4 py-8">
      <div className="flex items-center justify-between gap-4">
        <h1 className="flex items-center gap-2 text-xl font-semibold">
          <ShieldCheck className="size-6" />
          {t("warranty.portal.title")}
        </h1>
        <Button asChild variant="outline" size="sm">
          <Link href={routes.portal.home}>
            <ArrowLeft className="size-4 rtl:rotate-180" />
            {t("warranty.portal.back")}
          </Link>
        </Button>
      </div>
      <Card>
        <CardContent className="pt-6">
          <WarrantyFilterBar
            filters={filters}
            onChange={change}
            searchPlaceholder={t("warranty.portal.search_placeholder")}
            extra={
              <div className="space-y-1.5">
                <Label htmlFor="portal-warranty-sort">
                  {t("warranty.portal.sort.label")}
                </Label>
                <Select
                  value={sort}
                  onValueChange={(value) => {
                    setSort(value as PortalWarrantySort);
                    setPage(0);
                  }}
                >
                  <SelectTrigger
                    id="portal-warranty-sort"
                    data-testid="portal-warranty-sort"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PORTAL_WARRANTY_SORTS.map((value) => (
                      <SelectItem key={value} value={value}>
                        {t(PORTAL_SORT_LABELS[value])}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            }
          />
        </CardContent>
      </Card>
      {list.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void list.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : list.isLoading ? (
        <p className="text-muted-foreground text-sm">
          {t("warranty.list.loading")}
        </p>
      ) : rows.length === 0 ? (
        <Card>
          <CardContent
            className="py-8 text-center"
            data-testid="portal-warranties-empty"
          >
            <p className="font-medium">{t("warranty.portal.empty_title")}</p>
            <p className="text-muted-foreground text-sm">
              {hasWarrantyFilters(filters)
                ? t("warranty.list.empty_filtered")
                : t("warranty.portal.empty_description")}
            </p>
          </CardContent>
        </Card>
      ) : (
        <ul className="space-y-3" data-testid="portal-warranties">
          {rows.map((w) => (
            <li key={w.uuid} data-testid="portal-warranty" data-uuid={w.uuid}>
              <Card>
                <CardContent className="space-y-3 pt-6">
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div>
                      <p className="font-medium">{w.product.name}</p>
                      <p className="text-muted-foreground text-sm">
                        {warrantyVehicleTitle(w)}
                        {w.vehicle.plate ? (
                          <span className="ms-2 font-mono" dir="ltr">
                            {w.vehicle.plate}
                          </span>
                        ) : null}
                      </p>
                    </div>
                    <StatusChip
                      label={t(`warranty.status.${w.status}`)}
                      tone={warrantyStatusTone(w.status)}
                    />
                  </div>
                  <WarrantyProgressBar warranty={w} />
                  {claimByWarranty.get(w.uuid) ? (
                    <PortalClaimBadge claim={claimByWarranty.get(w.uuid)!} />
                  ) : null}
                  {w.status === "active" ? (
                    <WarrantyCertificateButton
                      client={portalCertificateClient(w.service.uuid)}
                      locale={locale}
                      testId={`warranty-pdf-${w.uuid}`}
                    />
                  ) : null}
                  <div className="text-muted-foreground flex flex-wrap items-center justify-between gap-2 text-xs">
                    <span>
                      {format.date(w.start_at)} – {format.date(w.end_at)} ·{" "}
                      {w.organization.name}
                    </span>
                    <a
                      href={`/garanti/${encodeURIComponent(w.public_code)}`}
                      target="_blank"
                      rel="noreferrer"
                      className="text-primary inline-flex items-center gap-1 hover:underline"
                    >
                      <ExternalLink className="size-3" />
                      {t("warranty.detail.public_page")}
                    </a>
                  </div>
                </CardContent>
              </Card>
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
    </div>
  );
}
