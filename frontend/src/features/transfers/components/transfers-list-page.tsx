"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ArrowLeftRight, ChevronLeft, ChevronRight, Plus } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  pageCount,
  TRANSFER_DIRECTIONS,
  TRANSFER_STATUSES,
  transferStatusTone,
} from "@/features/transfers/lib/transfers";
import {
  transferKeys,
  transfersService,
  type StockTransferStatus,
  type TransferDirection,
  type TransferListQuery,
} from "@/features/transfers/services/transfers.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const TRANSFER_PAGE_SIZE = 20;

const ALL = "all";

/**
 * Tenant > Transfers (TEC-197): stock transfer requests of the active
 * organization as the giver (outgoing), the receiver (incoming) or the
 * common parent (approval), with a status filter.
 */
export function TransfersListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canRead =
    can(Permission.TransfersRequest) || can(Permission.TransfersApprove);
  const canCreate = can(Permission.TransfersRequest);
  const [direction, setDirection] = useState<TransferDirection | typeof ALL>(
    ALL,
  );
  const [status, setStatus] = useState<StockTransferStatus | typeof ALL>(ALL);
  const [page, setPage] = useState(0);

  const query = useMemo<TransferListQuery>(
    () => ({
      ...(direction === ALL ? {} : { direction }),
      ...(status === ALL ? {} : { status }),
      limit: TRANSFER_PAGE_SIZE,
      offset: page * TRANSFER_PAGE_SIZE,
    }),
    [direction, status, page],
  );

  const list = useQuery({
    queryKey: transferKeys.list(query),
    queryFn: () => transfersService.list(query),
    enabled: canRead,
    placeholderData: keepPreviousData,
  });

  const title = t("transfers.list.title");
  const header = (
    <PageHeader
      title={title}
      icon={<ArrowLeftRight className="size-6" />}
      description={t("transfers.list.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        canCreate ? (
          <Button asChild>
            <Link
              href={routes.tenant.transfers.create(slug)}
              data-testid="transfer-new"
            >
              <Plus className="size-4" />
              {t("transfers.list.new")}
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
          description={t("transfers.list.forbidden")}
        />
      </div>
    );
  }

  const total = list.data?.total ?? 0;
  const pages = pageCount(total, TRANSFER_PAGE_SIZE);
  const rows = list.data?.items ?? [];

  return (
    <div className="space-y-6">
      {header}
      <Card>
        <CardContent className="space-y-4 pt-6">
          <div
            className="flex flex-wrap gap-2"
            role="group"
            aria-label={t("transfers.list.direction")}
          >
            {[ALL, ...TRANSFER_DIRECTIONS].map((d) => {
              const active = direction === d;
              return (
                <Button
                  key={d}
                  type="button"
                  size="sm"
                  variant={active ? "default" : "outline"}
                  aria-pressed={active}
                  data-direction={d}
                  onClick={() => {
                    setDirection(d as TransferDirection | typeof ALL);
                    setPage(0);
                  }}
                >
                  {t(`transfers.direction.${d}`)}
                </Button>
              );
            })}
          </div>
          <div
            className="flex flex-wrap gap-2"
            role="group"
            aria-label={t("transfers.list.status")}
          >
            {[ALL, ...TRANSFER_STATUSES].map((s) => {
              const active = status === s;
              return (
                <Button
                  key={s}
                  type="button"
                  size="sm"
                  variant={active ? "secondary" : "ghost"}
                  aria-pressed={active}
                  data-status={s}
                  onClick={() => {
                    setStatus(s as StockTransferStatus | typeof ALL);
                    setPage(0);
                  }}
                >
                  {s === ALL
                    ? t("transfers.list.all_statuses")
                    : t(`transfers.status.${s}`)}
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
                {t("transfers.list.loading")}
              </p>
            ) : rows.length === 0 ? (
              <div className="py-8 text-center" data-testid="transfers-empty">
                <p className="font-medium">{t("transfers.list.empty_title")}</p>
                <p className="text-muted-foreground text-sm">
                  {t("transfers.list.empty_description")}
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm" data-testid="transfers-table">
                  <thead>
                    <tr className="text-muted-foreground border-b text-xs">
                      <th className="p-2 text-start font-medium">
                        {t("transfers.columns.no")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("transfers.columns.status")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("transfers.columns.sender")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("transfers.columns.receiver")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("transfers.columns.units")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("transfers.columns.created")}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((r) => (
                      <tr
                        key={r.uuid}
                        className="hover:bg-accent/50 border-b align-top last:border-0"
                        data-testid="transfer-row"
                        data-uuid={r.uuid}
                      >
                        <td className="p-2">
                          <Link
                            href={routes.tenant.transfers.detail(slug, r.uuid)}
                            className="font-mono text-xs font-medium hover:underline"
                            dir="ltr"
                          >
                            {r.transfer_no}
                          </Link>
                          <div className="text-muted-foreground text-xs">
                            {t(`transfers.role.${r.role}`)}
                          </div>
                        </td>
                        <td className="p-2">
                          <StatusChip
                            label={t(`transfers.status.${r.status}`)}
                            tone={transferStatusTone(r.status)}
                          />
                        </td>
                        <td className="p-2">{r.sender.name}</td>
                        <td className="p-2">{r.receiver.name}</td>
                        <td className="p-2">{format.number(r.item_count)}</td>
                        <td className="text-muted-foreground p-2 text-xs whitespace-nowrap">
                          {format.dateTime(r.created_at)}
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
                {t("transfers.list.page", { page: page + 1, pages, total })}
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
                  {t("transfers.list.prev")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={page + 1 >= pages || list.isFetching}
                  onClick={() => setPage((p) => p + 1)}
                >
                  {t("transfers.list.next")}
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
