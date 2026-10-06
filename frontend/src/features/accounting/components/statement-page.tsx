"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Lock } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  CLIENT_SIDE_MANUAL,
  EntityPage,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { DisputeButton } from "@/features/accounting/components/dispute-dialog";
import { BalanceLabel, Money } from "@/features/accounting/components/shared";
import { StatementExport } from "@/features/accounting/components/statement-export";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import { isStatementLineDisputable } from "@/features/accounting/lib/disputes";
import {
  accountingService,
  type CariStatement,
  type CariStatementLine,
  type StatementPeriod,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

export const STATEMENT_PERSIST_KEY = "tenant-accounting-statement-v1";

/** A dealer that cannot write its book (no dealer_accounting module). */
export function ReadOnlyNotice() {
  const { t } = useLocale();
  return (
    <Alert data-testid="accounting-read-only">
      <Lock className="size-4" />
      <AlertDescription>{t("accounting.read_only_dealer")}</AlertDescription>
    </Alert>
  );
}

export type StatementViewProps = {
  statement: CariStatement;
  orgUuid: string;
  parentUuid: string | null;
  canDispute: boolean;
  /** Entry uuids that already carry an open dispute. */
  openDisputes: ReadonlySet<string>;
};

/**
 * Statement body: opening / debit / credit / closing and the period rows
 * with their running balance. A disputable row of the parent's cari shows
 * the "dispute" button (K24); a row with an open dispute shows its marker.
 */
export function StatementView({
  statement,
  orgUuid,
  parentUuid,
  canDispute,
  openDisputes,
}: StatementViewProps) {
  const { t, format } = useLocale();
  const s = statement;
  const columns = useMemo(
    () =>
      [
        createColumn<CariStatementLine>({
          accessorKey: "date",
          labelKey: "accounting.fields.date",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.date)}
            </span>
          ),
        }),
        createColumn<CariStatementLine>({
          accessorKey: "description",
          labelKey: "accounting.fields.description",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div>
              <div className="font-medium">{row.original.description}</div>
              {row.original.description !== row.original.category_label ? (
                <div className="text-muted-foreground text-xs">
                  {row.original.category_label}
                </div>
              ) : null}
            </div>
          ),
        }),
        createColumn<CariStatementLine>({
          accessorKey: "source_label",
          labelKey: "accounting.fields.source",
          enableSorting: false,
          cell: ({ row }) => row.original.source_label || "—",
        }),
        createColumn<CariStatementLine>({
          accessorKey: "debit",
          labelKey: "accounting.statement.debit",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="text-end">
              {Number(row.original.debit) ? (
                <Money amount={row.original.debit} currency={s.currency} />
              ) : (
                "—"
              )}
            </div>
          ),
        }),
        createColumn<CariStatementLine>({
          accessorKey: "credit",
          labelKey: "accounting.statement.credit",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="text-end">
              {Number(row.original.credit) ? (
                <Money amount={row.original.credit} currency={s.currency} />
              ) : (
                "—"
              )}
            </div>
          ),
        }),
        createColumn<CariStatementLine>({
          accessorKey: "balance",
          labelKey: "accounting.fields.balance",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <div className="text-end">
              <Money amount={row.original.balance} currency={s.currency} />
            </div>
          ),
        }),
        createColumn<CariStatementLine>({
          id: "status",
          accessorFn: (row) =>
            row.reversal_of_uuid ? 2 : row.reversed ? 1 : 0,
          labelKey: "accounting.fields.status",
          enableSorting: false,
          cell: ({ row }) => {
            const line = row.original;
            const disputed = openDisputes.has(line.uuid);
            const disputable =
              canDispute &&
              !disputed &&
              isStatementLineDisputable(line, s.cari, parentUuid);
            return (
              <div
                className="flex flex-wrap items-center gap-1"
                data-testid="statement-line"
                data-entry={line.uuid}
              >
                {line.reversal_of_uuid ? (
                  <StatusChip
                    label={t("accounting.entries.reversal")}
                    tone="warning"
                  />
                ) : null}
                {line.reversed ? (
                  <StatusChip
                    label={t("accounting.entries.voided")}
                    tone="danger"
                  />
                ) : null}
                {disputed ? (
                  <span data-testid="statement-line-disputed">
                    <StatusChip
                      label={t("accounting.disputes.statuses.open")}
                      tone="warning"
                    />
                  </span>
                ) : null}
                {disputable ? (
                  <DisputeButton
                    orgUuid={orgUuid}
                    entryUuid={line.uuid}
                    summary={`${format.date(line.date)} · ${line.description}`}
                  />
                ) : null}
              </div>
            );
          },
        }),
      ] as ColumnDef<CariStatementLine, unknown>[],
    [canDispute, format, openDisputes, orgUuid, parentUuid, s, t],
  );
  const totals: { key: string; amount: string }[] = [
    { key: "opening", amount: s.opening_balance },
    { key: "total_debit", amount: s.total_debit },
    { key: "total_credit", amount: s.total_credit },
  ];

  return (
    <div className="space-y-6" data-testid="statement-view">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {totals.map((x) => (
          <Card key={x.key}>
            <CardHeader>
              <CardTitle className="text-muted-foreground text-sm font-normal">
                {t(`accounting.statement.${x.key}`)}
              </CardTitle>
            </CardHeader>
            <CardContent data-testid={`statement-${x.key}`}>
              <Money
                amount={x.amount}
                currency={s.currency}
                className="text-xl font-semibold"
              />
            </CardContent>
          </Card>
        ))}
        <Card>
          <CardHeader>
            <CardTitle className="text-muted-foreground text-sm font-normal">
              {t("accounting.statement.closing")}
            </CardTitle>
            <CardAction>
              <BalanceLabel balance={s.closing_balance} />
            </CardAction>
          </CardHeader>
          <CardContent data-testid="statement-closing">
            <Money
              amount={s.closing_balance}
              currency={s.currency}
              className="text-xl font-semibold"
            />
          </CardContent>
        </Card>
      </div>

      {s.lines.length === 0 ? (
        <p
          className="text-muted-foreground text-sm"
          data-testid="statement-empty"
        >
          {t("accounting.statement.empty")}
        </p>
      ) : (
        <div data-testid="statement-lines">
          <EntityTable
            columns={columns}
            data={s.lines}
            getRowId={(row) => row.uuid}
            manual={CLIENT_SIDE_MANUAL}
            initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
            emptyTitle={t("accounting.statement.empty")}
            emptyDescription=""
            features={{
              persistKey: STATEMENT_PERSIST_KEY,
              // The running balance keeps the chronological order.
              sorting: false,
              rowSelection: false,
            }}
          />
        </div>
      )}
    </div>
  );
}

/** Open disputes of the active organization, keyed by entry uuid. */
export function useOpenDisputeEntries(orgUuid: string, enabled: boolean) {
  const params = { status: "open" as const, limit: 100, offset: 0 };
  const query = useQuery({
    queryKey: accountingKeys.disputes(orgUuid, params),
    queryFn: () => accountingService.listDisputes(params),
    enabled: enabled && Boolean(orgUuid),
  });
  return useMemo(() => {
    const set = new Set<string>();
    for (const d of query.data?.items ?? []) {
      if (d.status === "open") set.add(d.entry.uuid);
    }
    return set;
  }, [query.data]);
}

/** Tenant > Accounting > Cari > Statement (TEC-175 data, TEC-195 screen). */
export function StatementPage({ slug, uuid }: { slug: string; uuid: string }) {
  const { t } = useLocale();
  const access = useAccountingAccess(slug);
  const [period, setPeriod] = useState<StatementPeriod>({ from: "", to: "" });
  const enabled = access.canRead && Boolean(access.orgUuid);
  const invalidRange = Boolean(
    period.from && period.to && period.to < period.from,
  );

  const statement = useQuery({
    queryKey: accountingKeys.statement(access.orgUuid, uuid, period),
    queryFn: () => accountingService.getStatement(uuid, period),
    enabled: enabled && !invalidRange,
  });
  const openDisputes = useOpenDisputeEntries(
    access.orgUuid,
    enabled && access.canDispute,
  );

  const s = statement.data;
  const name = s?.cari.counterparty.name ?? t("accounting.cari.detail_title");

  return (
    <EntityPage
      title={t("accounting.statement.title")}
      description={s ? s.cari.counterparty.name : undefined}
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
        {
          label: name,
          href: routes.tenant.accounting.cariDetail(slug, uuid),
        },
        { label: t("accounting.statement.title") },
      ]}
      actions={
        enabled && !invalidRange ? (
          <StatementExport
            orgUuid={access.orgUuid}
            cariUuid={uuid}
            period={period}
          />
        ) : null
      }
    >
      <div className="space-y-6">
        {access.readOnlyDealer ? <ReadOnlyNotice /> : null}
        <div className="flex flex-wrap items-end gap-3">
          <div className="grid gap-1.5">
            <Label htmlFor="statement-from">
              {t("accounting.filters.date_from")}
            </Label>
            <DatePicker
              id="statement-from"
              value={period.from}
              placeholder={t("accounting.filters.any_date")}
              onChange={(v) => setPeriod((p) => ({ ...p, from: v }))}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="statement-to">
              {t("accounting.filters.date_to")}
            </Label>
            <DatePicker
              id="statement-to"
              value={period.to}
              placeholder={t("accounting.filters.any_date")}
              onChange={(v) => setPeriod((p) => ({ ...p, to: v }))}
            />
          </div>
          {period.from || period.to ? (
            <Button
              type="button"
              variant="ghost"
              onClick={() => setPeriod({ from: "", to: "" })}
            >
              {t("accounting.filters.clear")}
            </Button>
          ) : null}
        </div>
        {invalidRange ? (
          <p className="text-destructive text-sm" role="alert">
            {t("accounting.statement.invalid_range")}
          </p>
        ) : statement.isLoading ? (
          <Loading />
        ) : statement.isError || !s ? (
          <ErrorState
            title={
              isApiError(statement.error) && statement.error.status === 404
                ? t("accounting.cari.not_found")
                : t("accounting.load_failed")
            }
            onRetry={() => void statement.refetch()}
          />
        ) : (
          <StatementView
            statement={s}
            orgUuid={access.orgUuid}
            parentUuid={access.parentUuid}
            canDispute={access.canDispute}
            openDisputes={openDisputes}
          />
        )}
      </div>
    </EntityPage>
  );
}
