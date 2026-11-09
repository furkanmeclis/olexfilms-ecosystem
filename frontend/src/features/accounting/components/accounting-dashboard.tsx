"use client";

import { useQuery } from "@tanstack/react-query";
import {
  BookOpen,
  FileCheck2,
  ListChecks,
  Truck,
  Undo2,
  Users,
} from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout";
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
import { BalanceLabel, Money } from "@/features/accounting/components/shared";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import { QUEUE_TO_INVOICE } from "@/features/accounting/lib/home-role";
import { accountingService } from "@/features/accounting/services/accounting.service";
import {
  orderKeys,
  ordersService,
  type OrderListQuery,
} from "@/features/orders/services/orders.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const DASHBOARD_ROWS = 6;

/**
 * TEC-1070: the accounting home (legacy olexfilms-warehouse accounting
 * dashboard: the accounting package first, then orders and customers).
 * Queue counters, orders waiting for an invoice mark, received returns
 * waiting to be processed and the caris with the highest balance.
 */
export function AccountingDashboard({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const access = useAccountingAccess(slug);
  const org = access.orgUuid;
  const summary = useQuery({
    queryKey: accountingKeys.queueSummary(org),
    queryFn: () => accountingService.queueSummary(),
    enabled: access.canRead && org !== "",
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
  const queueHref = routes.tenant.accounting.queue(slug);
  const tiles: {
    id: string;
    label: string;
    value: number | undefined;
    icon: ReactNode;
    href: string;
  }[] = [
    {
      id: "to_invoice",
      label: t("accounting.home.to_invoice"),
      value: summary.data?.orders_to_invoice,
      icon: <FileCheck2 className="size-3.5" />,
      href: `${queueHref}?tab=orders`,
    },
    {
      id: "in_transit",
      label: t("accounting.home.in_transit"),
      value: summary.data?.orders_in_transit,
      icon: <Truck className="size-3.5" />,
      href: `${queueHref}?tab=transit`,
    },
    {
      id: "returns",
      label: t("accounting.home.returns_pending"),
      value: summary.data?.returns_pending,
      icon: <Undo2 className="size-3.5" />,
      href: `${queueHref}?tab=returns`,
    },
  ];

  return (
    <div className="flex flex-col gap-6" data-testid="accounting-dashboard">
      <PageHeader
        title={t("accounting.home.title")}
        description={t("accounting.home.description")}
        icon={<BookOpen className="size-6" />}
        actions={
          <Button asChild>
            <Link href={queueHref}>
              <ListChecks className="size-4" />
              {t("accounting.queue.title")}
            </Link>
          </Button>
        }
      />
      <div className="grid gap-3 sm:grid-cols-3">
        {tiles.map((tile) => {
          const hot = typeof tile.value === "number" && tile.value > 0;
          return (
            <Link
              key={tile.id}
              href={tile.href}
              data-testid="accounting-kpi"
              data-kpi={tile.id}
              data-count={tile.value}
              className={cn(
                "bg-card hover:bg-accent/50 rounded-lg border p-3 transition-colors",
                hot && "border-primary/50",
              )}
            >
              <div className="text-muted-foreground flex items-center gap-1.5 text-xs">
                {tile.icon}
                <span className="truncate">{tile.label}</span>
              </div>
              {summary.isError ? (
                <div className="text-destructive mt-1 text-sm">
                  {t("accounting.load_failed")}
                </div>
              ) : typeof tile.value === "number" ? (
                <div
                  className={cn(
                    "mt-1 text-2xl font-semibold tabular-nums",
                    hot && "text-primary",
                  )}
                >
                  {tile.value}
                </div>
              ) : (
                <Skeleton className="mt-1.5 h-7 w-12" />
              )}
            </Link>
          );
        })}
      </div>
      <div className="grid gap-6 xl:grid-cols-2">
        {can(Permission.OrdersRead) ? (
          <ToInvoiceCard slug={slug} href={`${queueHref}?tab=orders`} />
        ) : null}
        <ReturnsCard slug={slug} href={`${queueHref}?tab=returns`} />
        <TopCariCard slug={slug} />
      </div>
    </div>
  );
}

function DashCard({
  testId,
  icon,
  title,
  description,
  href,
  children,
}: {
  testId: string;
  icon: ReactNode;
  title: string;
  description: string;
  href: string;
  children: ReactNode;
}) {
  const { t } = useLocale();
  return (
    <Card data-testid={testId}>
      <CardHeader className="flex flex-row items-start justify-between gap-2">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2">
            {icon}
            {title}
          </CardTitle>
          <CardDescription>{description}</CardDescription>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href={href}>{t("accounting.home.view_all")}</Link>
        </Button>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

function ToInvoiceCard({ slug, href }: { slug: string; href: string }) {
  const { t, format } = useLocale();
  const params: OrderListQuery = {
    side: "seller",
    ...QUEUE_TO_INVOICE,
    sort: "created_at",
    limit: DASHBOARD_ROWS,
    offset: 0,
  };
  const query = useQuery({
    queryKey: orderKeys.list(params),
    queryFn: () => ordersService.list(params),
    staleTime: 30_000,
  });
  const items = query.data?.items ?? [];
  return (
    <DashCard
      testId="accounting-to-invoice"
      icon={<FileCheck2 className="size-4" />}
      title={t("accounting.home.to_invoice")}
      description={t("accounting.home.to_invoice_description")}
      href={href}
    >
      {query.isError ? (
        <ErrorState
          title={t("accounting.load_failed")}
          onRetry={() => void query.refetch()}
        />
      ) : query.isPending ? (
        <Skeleton className="h-32" />
      ) : items.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("accounting.queue.orders_empty")}
        </p>
      ) : (
        <ul className="divide-y text-sm">
          {items.map((o) => (
            <li key={o.uuid}>
              <Link
                href={routes.tenant.orders.detail(slug, o.uuid)}
                className="hover:bg-accent/50 -mx-2 flex items-center justify-between gap-3 rounded-md px-2 py-2"
                data-testid="accounting-order-row"
              >
                <div className="min-w-0">
                  <div className="truncate font-medium">{o.buyer.name}</div>
                  <div className="text-muted-foreground flex gap-2 text-xs">
                    <span className="font-mono" dir="ltr">
                      {o.order_no}
                    </span>
                    {o.received_at ? (
                      <span>{format.dateTime(o.received_at)}</span>
                    ) : null}
                  </div>
                </div>
                {o.total != null ? (
                  <Money amount={o.total} currency={o.currency} />
                ) : null}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </DashCard>
  );
}

function ReturnsCard({ slug, href }: { slug: string; href: string }) {
  const { t, format } = useLocale();
  const access = useAccountingAccess(slug);
  const params = {
    processed: false,
    sort: "received_at",
    limit: DASHBOARD_ROWS,
    offset: 0,
  };
  const query = useQuery({
    queryKey: accountingKeys.queueReturns(access.orgUuid, params),
    queryFn: () => accountingService.listQueueReturns(params),
    enabled: access.canRead && access.orgUuid !== "",
    staleTime: 30_000,
  });
  const items = query.data?.items ?? [];
  return (
    <DashCard
      testId="accounting-returns"
      icon={<Undo2 className="size-4" />}
      title={t("accounting.home.returns_pending")}
      description={t("accounting.home.returns_description")}
      href={href}
    >
      {query.isError ? (
        <ErrorState
          title={t("accounting.load_failed")}
          onRetry={() => void query.refetch()}
        />
      ) : query.isPending ? (
        <Skeleton className="h-24" />
      ) : items.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("accounting.queue.returns_empty")}
        </p>
      ) : (
        <ul className="divide-y text-sm">
          {items.map((r) => (
            <li
              key={r.uuid}
              className="flex items-center justify-between gap-3 py-2"
              data-testid="accounting-return-row"
            >
              <div className="min-w-0">
                <div className="truncate font-medium">{r.sender.name}</div>
                <div className="text-muted-foreground flex gap-2 text-xs">
                  <span className="font-mono" dir="ltr">
                    {r.transfer_no}
                  </span>
                  {r.received_at ? (
                    <span>{format.dateTime(r.received_at)}</span>
                  ) : null}
                </div>
              </div>
              {r.total ? (
                <Money amount={r.total} currency={r.currency} />
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </DashCard>
  );
}

function TopCariCard({ slug }: { slug: string }) {
  const { t } = useLocale();
  const access = useAccountingAccess(slug);
  const params = { sort: "-balance", limit: DASHBOARD_ROWS, offset: 0 };
  const query = useQuery({
    queryKey: accountingKeys.cariList(access.orgUuid, params),
    queryFn: () => accountingService.listCari(params),
    enabled: access.canRead && access.orgUuid !== "",
    staleTime: 60_000,
  });
  const items = query.data?.items ?? [];
  return (
    <DashCard
      testId="accounting-top-cari"
      icon={<Users className="size-4" />}
      title={t("accounting.home.top_cari")}
      description={t("accounting.home.top_cari_description")}
      href={routes.tenant.accounting.cari(slug)}
    >
      {query.isError ? (
        <ErrorState
          title={t("accounting.load_failed")}
          onRetry={() => void query.refetch()}
        />
      ) : query.isPending ? (
        <Skeleton className="h-24" />
      ) : items.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("accounting.cari.empty_title")}
        </p>
      ) : (
        <ul className="divide-y text-sm">
          {items.map((c) => (
            <li key={c.uuid}>
              <Link
                href={routes.tenant.accounting.cariDetail(slug, c.uuid)}
                className="hover:bg-accent/50 -mx-2 flex items-center justify-between gap-3 rounded-md px-2 py-2"
                data-testid="accounting-cari-row"
              >
                <span className="truncate font-medium">
                  {c.counterparty.name}
                </span>
                <span className="flex items-center gap-2">
                  <BalanceLabel balance={c.balance} />
                  <Money
                    amount={Math.abs(Number(c.balance))}
                    currency={c.currency}
                  />
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </DashCard>
  );
}
