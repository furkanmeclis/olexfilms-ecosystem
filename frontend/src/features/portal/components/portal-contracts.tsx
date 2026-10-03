"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { FileSignature } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { PortalPage } from "@/features/portal/components/portal-page";
import { portalApi } from "@/features/portal/lib/portal-client";
import { PORTAL_CONTRACT_PAGE_SIZE } from "@/features/portal/lib/portal-vehicles";
import { pageCount } from "@/features/warranty/lib/warranty-list";
import { useLocale } from "@/providers/locale-provider";

/**
 * Portal > My contracts (TEC-245): the signed vehicle intake contracts of
 * the user (GET /v1/portal/contracts). The contracts module comes with F3,
 * so the list is empty until then and the page shows its empty state.
 */
export function PortalContracts() {
  const { t, format } = useLocale();
  const [page, setPage] = useState(0);
  const list = useQuery({
    queryKey: ["portal", "contracts", page],
    queryFn: () =>
      portalApi.listContracts(
        PORTAL_CONTRACT_PAGE_SIZE,
        page * PORTAL_CONTRACT_PAGE_SIZE,
      ),
    placeholderData: keepPreviousData,
  });
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const pages = pageCount(total, PORTAL_CONTRACT_PAGE_SIZE);

  return (
    <PortalPage
      title={t("portal.contracts.title")}
      icon={<FileSignature className="size-6" />}
      testId="portal-contracts-page"
    >
      {list.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void list.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : list.isLoading ? (
        <p className="text-muted-foreground text-sm">
          {t("portal.contracts.loading")}
        </p>
      ) : rows.length === 0 ? (
        <Card>
          <CardContent
            className="flex flex-col items-center gap-2 py-10 text-center"
            data-testid="portal-contracts-empty"
          >
            <FileSignature className="text-muted-foreground size-10" />
            <p className="font-medium">{t("portal.contracts.empty_title")}</p>
            <p className="text-muted-foreground max-w-sm text-sm">
              {t("portal.contracts.empty_description")}
            </p>
          </CardContent>
        </Card>
      ) : (
        <ul className="space-y-3" data-testid="portal-contracts">
          {rows.map((c) => (
            <li key={c.service.uuid} data-testid="portal-contract">
              <Card>
                <CardContent className="flex flex-wrap items-start justify-between gap-3 pt-6">
                  <div className="min-w-0 space-y-1">
                    <p className="font-medium">
                      {[c.car_brand_name, c.car_model_name]
                        .filter(Boolean)
                        .join(" ")}
                      {c.plate ? (
                        <span
                          className="text-muted-foreground ms-2 font-mono text-sm tracking-wider"
                          dir="ltr"
                        >
                          {c.plate}
                        </span>
                      ) : null}
                    </p>
                    <p className="text-muted-foreground text-sm">
                      {c.organization.name}
                    </p>
                    <p className="text-muted-foreground text-xs">
                      {t("portal.contracts.signed_at", {
                        date: format.date(c.created_at),
                      })}
                    </p>
                  </div>
                  <div className="flex flex-col items-end gap-2">
                    <Badge variant="outline" dir="ltr">
                      {c.service.service_no}
                    </Badge>
                    <Button asChild variant="outline" size="sm">
                      <Link href={routes.portal.service(c.service.uuid)}>
                        {t("portal.contracts.open_service")}
                      </Link>
                    </Button>
                  </div>
                </CardContent>
              </Card>
            </li>
          ))}
        </ul>
      )}
      {pages > 1 ? (
        <div className="flex items-center justify-between gap-2">
          <p className="text-muted-foreground text-sm">
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
