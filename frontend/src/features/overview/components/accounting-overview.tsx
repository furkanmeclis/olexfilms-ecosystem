"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { BookOpen, ChevronRight } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import {
  BarChart,
  KpiGrid,
  type KpiTileProps,
} from "@/components/analytics";
import { routes } from "@/config/routes";
import {
  AccountingDashboardBody,
  AccountingQueueButton,
} from "@/features/accounting";
import { useAccountingAccess } from "@/features/accounting/hooks/use-accounting-access";
import { OverviewShell } from "@/features/overview/components/overview-shell";
import { periodRange } from "@/features/overview/lib/period-range";
import type { ReportQuery } from "@/features/reports";
import {
  staffReportsKeys,
  staffReportsService,
  type PnlReport,
} from "@/features/staff-reports/services/staff-reports.service";
import { useLocale } from "@/providers/locale-provider";

function amount(v: string | null | undefined): number | null {
  if (v === null || v === undefined || v === "") return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

/** P&L of the period (and of the compared window) by month. */
function usePnl(org: string, enabled: boolean, query: ReportQuery) {
  const { locale } = useLocale();
  // The day is fixed per query so the keys stay stable during the session.
  const range = useMemo(() => periodRange(query, new Date()), [query]);
  const current = useQuery({
    queryKey: staffReportsKeys.report(org, "pnl", {
      from: range.from,
      to: range.to,
      group: "month",
      locale,
    }),
    queryFn: () =>
      staffReportsService.pnl({
        from: range.from,
        to: range.to,
        group: "month",
        locale,
      }),
    enabled,
    placeholderData: keepPreviousData,
  });
  const prev = range.previous;
  const previous = useQuery({
    queryKey: staffReportsKeys.report(org, "pnl", {
      from: prev?.from,
      to: prev?.to,
      group: "month",
      locale,
    }),
    queryFn: () =>
      staffReportsService.pnl({
        from: prev?.from,
        to: prev?.to,
        group: "month",
        locale,
      }),
    enabled: enabled && prev !== null,
    placeholderData: keepPreviousData,
  });
  return { current, previous: prev ? previous : null };
}

function pnlTiles(
  current: PnlReport | undefined,
  previous: PnlReport | undefined,
  labels: { income: string; expense: string; net: string },
  href: string,
): KpiTileProps[] {
  const currency = current?.currency || undefined;
  const lines = current?.lines ?? [];
  const tile = (
    key: "income" | "expense" | "net",
    good: "up" | "down",
  ): KpiTileProps => ({
    id: key,
    label: labels[key],
    value: amount(current?.totals[key]),
    previous: previous ? amount(previous.totals[key]) : undefined,
    good,
    format: { kind: "money", currency },
    spark:
      lines.length > 1
        ? lines.map((l) => amount(l[key]) ?? 0)
        : undefined,
    href,
  });
  return [tile("income", "up"), tile("expense", "down"), tile("net", "up")];
}

/**
 * TEC-1212: accounting overview. The period's income, expenses and net
 * (accounting P&L) against the compared window, income vs expenses by
 * month, then the queue counters and cards of the accounting home.
 */
export function AccountingOverview({ slug }: { slug: string }) {
  const { t } = useLocale();
  const access = useAccountingAccess(slug);
  const reportsHref = routes.tenant.accountingReports(slug);
  return (
    <OverviewShell
      testId="accounting-overview"
      title={t("accounting.home.title")}
      description={t("accounting.home.description")}
      icon={<BookOpen className="size-6" />}
      actions={<AccountingQueueButton slug={slug} />}
    >
      {(query) => (
        <>
          <AccountingPnlSummary
            org={access.orgUuid}
            enabled={access.canRead && access.orgUuid !== ""}
            query={query}
            reportsHref={reportsHref}
          />
          <AccountingDashboardBody slug={slug} />
        </>
      )}
    </OverviewShell>
  );
}

function AccountingPnlSummary({
  org,
  enabled,
  query,
  reportsHref,
}: {
  org: string;
  enabled: boolean;
  query: ReportQuery;
  reportsHref: string;
}) {
  const { t } = useLocale();
  const { current, previous } = usePnl(org, enabled, query);
  if (!enabled) return null;
  const labels = {
    income: t("staff_reports.reports.pnl.income"),
    expense: t("staff_reports.reports.pnl.expense"),
    net: t("staff_reports.reports.pnl.net"),
  };
  const tiles = pnlTiles(
    current.data,
    previous?.data,
    labels,
    reportsHref,
  );
  const lines = current.data?.lines ?? [];
  const currency = current.data?.currency || undefined;
  return (
    <div className="flex flex-col gap-6" data-testid="accounting-pnl">
      <KpiGrid items={tiles} loading={current.isPending} />
      <BarChart
        title={t("dashboard.role.accounting.pnl_title")}
        description={t("dashboard.role.accounting.pnl_description")}
        actions={
          <Link
            href={reportsHref}
            className="text-primary inline-flex items-center gap-1 text-sm underline-offset-4 hover:underline"
          >
            {t("reports.panel.open")}
            <ChevronRight className="size-4 rtl:rotate-180" aria-hidden />
          </Link>
        }
        data={lines.map((l) => ({
          key: l.key,
          label: l.label,
          income: amount(l.income),
          expense: amount(l.expense),
        }))}
        series={[
          { key: "income", label: labels.income },
          { key: "expense", label: labels.expense },
        ]}
        format={{ kind: "money", currency }}
        loading={current.isPending}
        error={current.isError && !current.data ? t("reports.panel.error") : undefined}
        onRetry={() => void current.refetch()}
      />
    </div>
  );
}
