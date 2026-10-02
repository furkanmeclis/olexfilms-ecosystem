"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Landmark, Pencil, Wallet } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { EntityCreateButton, EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { AccountFormDialog } from "@/features/accounting/components/account-form-dialog";
import { Money } from "@/features/accounting/components/shared";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import {
  accountCreateInput,
  accountUpdateInput,
  type AccountFormValues,
} from "@/features/accounting/lib/form";
import {
  accountingService,
  type FinanceAccount,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Tenant > Accounting > Cash and bank accounts (TEC-176). */
export function AccountsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const access = useAccountingAccess(slug);
  const [editing, setEditing] = useState<FinanceAccount | null>(null);
  const [open, setOpen] = useState(false);

  const list = useQuery({
    queryKey: accountingKeys.accounts(access.orgUuid, {}),
    queryFn: () => accountingService.listAccounts(),
    enabled: access.canRead && Boolean(access.orgUuid),
  });

  const save = useMutation({
    mutationFn: (values: AccountFormValues) =>
      editing
        ? accountingService.updateAccount(
            editing.uuid,
            accountUpdateInput(values),
          )
        : accountingService.createAccount(accountCreateInput(values)),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: accountingKeys.all(access.orgUuid),
      });
      appToast.success(t("accounting.toast.saved"));
      setOpen(false);
    },
    onError: (error: unknown) =>
      appToast.error(
        isApiError(error) ? error.message : t("accounting.toast.failed"),
      ),
  });

  const openForm = (account: FinanceAccount | null) => {
    setEditing(account);
    setOpen(true);
  };

  const items = list.data?.items ?? [];

  return (
    <EntityPage
      title={t("accounting.accounts.title")}
      description={
        access.canWrite
          ? t("accounting.accounts.description")
          : t("accounting.read_only")
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
        { label: t("accounting.nav") },
        { label: t("accounting.accounts.title") },
      ]}
      actions={
        access.canWrite ? (
          <EntityCreateButton
            label={t("accounting.accounts.create")}
            onClick={() => openForm(null)}
          />
        ) : null
      }
    >
      {list.isLoading ? (
        <Loading />
      ) : list.isError ? (
        <ErrorState
          title={t("accounting.load_failed")}
          onRetry={() => void list.refetch()}
        />
      ) : items.length === 0 ? (
        <EmptyState
          title={t("accounting.accounts.empty_title")}
          description={t("accounting.accounts.empty_description")}
        />
      ) : (
        <div
          className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3"
          data-testid="account-list"
        >
          {items.map((a) => (
            <Card key={a.uuid} data-testid="account-card">
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  {a.type === "bank" ? (
                    <Landmark className="size-4 shrink-0" />
                  ) : (
                    <Wallet className="size-4 shrink-0" />
                  )}
                  <span className="truncate">{a.name}</span>
                </CardTitle>
                <CardDescription className="flex flex-wrap items-center gap-2">
                  {t(`accounting.account_types.${a.type}`)}
                  {a.active ? null : (
                    <StatusChip label={t("accounting.status.inactive")} />
                  )}
                </CardDescription>
                {access.canWrite ? (
                  <CardAction>
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={t("accounting.accounts.edit")}
                      onClick={() => openForm(a)}
                    >
                      <Pencil className="size-4" />
                    </Button>
                  </CardAction>
                ) : null}
              </CardHeader>
              <CardContent className="space-y-1">
                <Money
                  amount={a.balance}
                  currency={a.currency}
                  className="text-2xl font-semibold"
                />
                {a.iban ? (
                  <p
                    dir="ltr"
                    className="text-muted-foreground text-start font-mono text-xs"
                  >
                    {a.iban}
                  </p>
                ) : null}
                <p className="text-muted-foreground text-xs">
                  {t("accounting.accounts.entry_count", {
                    n: format.number(a.entry_count),
                  })}
                  {a.last_entry_at
                    ? ` · ${format.dateTime(a.last_entry_at)}`
                    : ""}
                </p>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
      <AccountFormDialog
        open={open}
        account={editing}
        pending={save.isPending}
        onOpenChange={setOpen}
        onSubmit={(values) => save.mutateAsync(values)}
      />
    </EntityPage>
  );
}
