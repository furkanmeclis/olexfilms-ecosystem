"use client";

import { useQuery } from "@tanstack/react-query";
import { ArrowDownLeft, ArrowUpRight, FileText } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { counterpartyKind } from "@/features/accounting/components/cari-page";
import { SettlementDialog } from "@/features/accounting/components/settlement-dialog";
import {
  BalanceLabel,
  EntryAmount,
  EntryStatus,
  Money,
  useCategoryLabels,
} from "@/features/accounting/components/shared";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import {
  accountingService,
  type SettlementKind,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

const RECENT_LIMIT = 10;

/**
 * Cari detail: balance and the latest rows. The statement (PDF/Excel) and
 * disputes come with TEC-195; the statement button is a placeholder.
 */
export function CariDetailPage({ slug, uuid }: { slug: string; uuid: string }) {
  const { t, format } = useLocale();
  const access = useAccountingAccess(slug);
  const [settle, setSettle] = useState<SettlementKind | null>(null);
  const enabled = access.canRead && Boolean(access.orgUuid);
  const categories = useCategoryLabels(access.orgUuid, enabled);

  const cari = useQuery({
    queryKey: accountingKeys.cari(access.orgUuid, uuid),
    queryFn: () => accountingService.getCari(uuid),
    enabled,
  });
  const recentParams = { cari_uuid: uuid, limit: RECENT_LIMIT, offset: 0 };
  const recent = useQuery({
    queryKey: accountingKeys.entries(access.orgUuid, recentParams),
    queryFn: () => accountingService.listEntries(recentParams),
    enabled,
  });

  const c = cari.data;
  const title = c?.counterparty.name ?? t("accounting.cari.detail_title");

  return (
    <EntityPage
      title={title}
      description={
        c
          ? t(`accounting.counterparty_types.${counterpartyKind(c)}`)
          : undefined
      }
      permission={permissions.accounting.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("accounting.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("accounting.cari.title"),
          href: routes.tenant.accounting.cari(slug),
        },
        { label: title },
      ]}
      actions={
        <div className="flex flex-wrap gap-2">
          {access.canWrite && c?.active ? (
            <>
              <Button onClick={() => setSettle("collection")}>
                <ArrowDownLeft className="size-4 rtl:-scale-x-100" />
                {t("accounting.settlement.new_collection")}
              </Button>
              <Button variant="outline" onClick={() => setSettle("payment")}>
                <ArrowUpRight className="size-4 rtl:-scale-x-100" />
                {t("accounting.settlement.new_payment")}
              </Button>
            </>
          ) : null}
          <Button
            variant="outline"
            disabled
            title={t("accounting.cari.statement_soon")}
            data-testid="cari-statement"
          >
            <FileText className="size-4" />
            {t("accounting.cari.statement")}
          </Button>
        </div>
      }
    >
      {cari.isLoading ? (
        <Loading />
      ) : cari.isError || !c ? (
        <ErrorState
          title={
            isApiError(cari.error) && cari.error.status === 404
              ? t("accounting.cari.not_found")
              : t("accounting.load_failed")
          }
          onRetry={() => void cari.refetch()}
        />
      ) : (
        <div className="space-y-6">
          <div className="grid gap-4 sm:grid-cols-3">
            <Card>
              <CardHeader>
                <CardTitle className="text-muted-foreground text-sm font-normal">
                  {t("accounting.fields.balance")}
                </CardTitle>
                <CardAction>
                  <BalanceLabel balance={c.balance} />
                </CardAction>
              </CardHeader>
              <CardContent>
                <Money
                  amount={Math.abs(Number(c.balance))}
                  currency={c.currency}
                  className="text-2xl font-semibold"
                />
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle className="text-muted-foreground text-sm font-normal">
                  {t("accounting.fields.entry_count")}
                </CardTitle>
              </CardHeader>
              <CardContent className="text-2xl font-semibold tabular-nums">
                {format.number(c.entry_count)}
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle className="text-muted-foreground text-sm font-normal">
                  {t("accounting.fields.last_entry")}
                </CardTitle>
              </CardHeader>
              <CardContent className="text-lg font-medium">
                {c.last_entry_at ? format.dateTime(c.last_entry_at) : "—"}
              </CardContent>
            </Card>
          </div>
          <Card>
            <CardHeader>
              <CardTitle>{t("accounting.cari.recent")}</CardTitle>
              <CardAction>
                <Button asChild variant="link" size="sm">
                  <Link
                    href={`${routes.tenant.accounting.entries(slug)}?cari=${encodeURIComponent(uuid)}`}
                  >
                    {t("accounting.cari.all_entries")}
                  </Link>
                </Button>
              </CardAction>
            </CardHeader>
            <CardContent>
              {recent.isLoading ? (
                <Loading />
              ) : (recent.data?.items ?? []).length === 0 ? (
                <p className="text-muted-foreground text-sm">
                  {t("accounting.entries.empty_title")}
                </p>
              ) : (
                <ul className="divide-y" data-testid="cari-recent">
                  {recent.data?.items.map((e) => (
                    <li
                      key={e.uuid}
                      className="flex flex-wrap items-center justify-between gap-3 py-2"
                    >
                      <div className="min-w-0">
                        <p className="font-medium">
                          {categories.get(e.category) ?? e.category}
                        </p>
                        <p className="text-muted-foreground text-xs">
                          {format.dateTime(e.created_at)} ·{" "}
                          {t(`accounting.directions.${e.direction}`)}
                          {e.description ? ` · ${e.description}` : ""}
                        </p>
                        <EntryStatus entry={e} />
                      </div>
                      <EntryAmount entry={e} />
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>
      )}
      {settle && c ? (
        <SettlementDialog
          orgUuid={access.orgUuid}
          kind={settle}
          open
          fixedCari={c}
          onOpenChange={(open) => {
            if (!open) setSettle(null);
          }}
        />
      ) : null}
    </EntityPage>
  );
}
