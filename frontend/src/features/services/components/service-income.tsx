"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { TrendingUp } from "lucide-react";
import { useCallback, useState, type FormEvent, type ReactNode } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  FieldError,
  NativeSelect,
} from "@/features/accounting/components/shared";
import { accountingKeys } from "@/features/accounting/hooks/use-accounting-access";
import { normalizeAmount } from "@/features/accounting/lib/form";
import {
  accountingService,
  type FinanceAccount,
} from "@/features/accounting/services/accounting.service";
import {
  SERVICE_INCOME_METHODS,
  serviceWizardKeys,
  serviceWizardService,
  type Service,
  type ServiceIncomeInput,
  type ServiceIncomePaymentMethod,
  type ServiceProfit,
} from "@/features/services/services/service-wizard.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Account type a payment method books into (cari books no account). */
const METHOD_ACCOUNT_TYPE: Record<ServiceIncomePaymentMethod, string | null> = {
  cash: "cash",
  card: "bank",
  cari: null,
};

/** Amount text of the profit card and the list columns. */
export function useServiceMoney() {
  const { format } = useLocale();
  return useCallback(
    (value: string | null | undefined) =>
      value === null || value === undefined || value === ""
        ? "—"
        : format.number(Number(value), {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          }),
    [format],
  );
}

/**
 * "Profit" card (TEC-343): revenue, purchase cost, gross profit and
 * margin. The backend nulls the cost, profit and margin without
 * pricing.purchase.read; those rows are then not shown at all.
 */
export function ServiceProfitCard({
  profit,
  actions,
}: {
  profit: ServiceProfit;
  actions?: ReactNode;
}) {
  const { t } = useLocale();
  const money = useServiceMoney();
  const rows: { key: string; label: string; value: string }[] = [
    {
      key: "revenue",
      label: t("services.profit.revenue"),
      value:
        profit.revenue === null
          ? t("services.profit.no_income")
          : money(profit.revenue),
    },
  ];
  if (profit.cost !== null) {
    rows.push({
      key: "cost",
      label: t("services.profit.cost"),
      value: money(profit.cost),
    });
  }
  if (profit.gross_profit !== null) {
    rows.push({
      key: "gross_profit",
      label: t("services.profit.gross_profit"),
      value: money(profit.gross_profit),
    });
  }
  if (profit.margin_pct !== null) {
    rows.push({
      key: "margin",
      label: t("services.profit.margin"),
      value: `%${money(profit.margin_pct)}`,
    });
  }
  return (
    <Card data-testid="detail-profit">
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="text-muted-foreground">
            <TrendingUp className="size-4" />
          </span>
          {t("services.profit.title")}
        </CardTitle>
        {actions}
      </CardHeader>
      <CardContent>
        <dl className="grid gap-3 sm:grid-cols-4">
          {rows.map((row) => (
            <div
              key={row.key}
              className="space-y-0.5"
              data-testid={`profit-${row.key}`}
            >
              <dt className="text-muted-foreground text-xs">{row.label}</dt>
              <dd className="text-sm font-medium tabular-nums" dir="ltr">
                {row.value}
              </dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  );
}

type IncomeFormValues = {
  amount: string;
  payment_method: ServiceIncomePaymentMethod;
  account_uuid: string;
  description: string;
};

/**
 * "Record income" form: amount, method (cash / card / cari) and the cash
 * or bank account of the method. "Cari" books the customer's cari, so the
 * account field is hidden and no account is sent.
 */
export function ServiceIncomeForm({
  service,
  accounts,
  pending,
  onCancel,
  onSubmit,
}: {
  service: Pick<Service, "is_warranty_reapply">;
  accounts: FinanceAccount[];
  pending?: boolean;
  onCancel: () => void;
  onSubmit: (input: ServiceIncomeInput) => Promise<unknown>;
}) {
  const { t } = useLocale();
  const [values, setValues] = useState<IncomeFormValues>({
    amount: "",
    payment_method: "cash",
    account_uuid: "",
    description: "",
  });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const accountType = METHOD_ACCOUNT_TYPE[values.payment_method];
  const options = accounts.filter((a) => a.active && a.type === accountType);
  // A single matching account is preselected.
  const accountUuid =
    values.account_uuid ||
    (options.length === 1 ? (options[0]?.uuid ?? "") : "");

  const set = <K extends keyof IncomeFormValues>(
    key: K,
    value: IncomeFormValues[K],
  ) => setValues((v) => ({ ...v, [key]: value }));

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const next: Record<string, string> = {};
    const amount = normalizeAmount(values.amount);
    if (!amount) next.amount = t("services.income.amount_invalid");
    if (accountType && !accountUuid) {
      next.account_uuid = t("services.income.account_required");
    }
    setErrors(next);
    if (Object.keys(next).length > 0 || !amount) return;
    const description = values.description.trim();
    try {
      await onSubmit({
        amount,
        payment_method: values.payment_method,
        ...(accountType ? { account_uuid: accountUuid } : {}),
        ...(description ? { description } : {}),
      });
    } catch (error) {
      if (isApiError(error) && error.isValidation) {
        setErrors(error.fieldErrors());
      }
    }
  };

  return (
    <form
      onSubmit={submit}
      noValidate
      className="space-y-4"
      data-testid="service-income-form"
    >
      {service.is_warranty_reapply ? (
        <Alert data-testid="service-income-warranty">
          <AlertDescription>
            {t("services.income.warranty_warning")}
          </AlertDescription>
        </Alert>
      ) : null}
      <div className="grid gap-1.5">
        <Label htmlFor="service-income-amount">
          {t("services.income.amount")}
        </Label>
        <Input
          id="service-income-amount"
          name="amount"
          inputMode="decimal"
          dir="ltr"
          autoComplete="off"
          className="text-end tabular-nums"
          value={values.amount}
          aria-invalid={errors.amount ? true : undefined}
          aria-describedby={
            errors.amount ? "service-income-amount-error" : undefined
          }
          onChange={(e) => set("amount", e.target.value)}
        />
        <FieldError id="service-income-amount-error" message={errors.amount} />
      </div>
      <NativeSelect
        id="service-income-method"
        name="payment_method"
        label={t("services.income.method")}
        value={values.payment_method}
        error={errors.payment_method}
        onChange={(v) =>
          setValues((prev) => ({
            ...prev,
            payment_method: v as ServiceIncomePaymentMethod,
            account_uuid: "",
          }))
        }
        options={SERVICE_INCOME_METHODS.map((m) => ({
          value: m,
          label: t(`services.income.methods.${m}`),
        }))}
      />
      {accountType ? (
        <NativeSelect
          id="service-income-account"
          name="account_uuid"
          label={t("services.income.account")}
          value={accountUuid}
          placeholder={t("services.income.pick_account")}
          error={errors.account_uuid}
          onChange={(v) => set("account_uuid", v)}
          options={options.map((a) => ({
            value: a.uuid,
            label: `${a.name} (${a.currency})`,
          }))}
        />
      ) : (
        <p
          className="text-muted-foreground text-xs"
          data-testid="service-income-cari-hint"
        >
          {t("services.income.cari_hint")}
        </p>
      )}
      <div className="grid gap-1.5">
        <Label htmlFor="service-income-description">
          {t("services.income.description")}
        </Label>
        <Textarea
          id="service-income-description"
          name="description"
          rows={2}
          maxLength={1000}
          value={values.description}
          onChange={(e) => set("description", e.target.value)}
        />
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button
          type="submit"
          disabled={pending}
          data-testid="service-income-submit"
        >
          {t("services.income.submit")}
        </Button>
      </DialogFooter>
    </form>
  );
}

function useRefreshService(service: Service, orgUuid: string) {
  const queryClient = useQueryClient();
  return async (next: Service) => {
    queryClient.setQueryData(serviceWizardKeys.service(service.uuid), next);
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: serviceWizardKeys.all }),
      queryClient.invalidateQueries({ queryKey: accountingKeys.all(orgUuid) }),
    ]);
  };
}

/** "Record income" dialog: loads the active accounts of the book. */
export function ServiceIncomeDialog({
  service,
  orgUuid,
  open,
  onOpenChange,
}: {
  service: Service;
  orgUuid: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const refresh = useRefreshService(service, orgUuid);
  const accounts = useQuery({
    queryKey: accountingKeys.accounts(orgUuid, { active: true }),
    queryFn: () => accountingService.listAccounts({ active: true }),
    enabled: open && Boolean(orgUuid),
  });
  const save = useMutation({
    mutationFn: (input: ServiceIncomeInput) =>
      serviceWizardService.recordIncome(service.uuid, input),
    onSuccess: async (result) => {
      await refresh(result.service);
      appToast.success(t("services.income.done"));
      onOpenChange(false);
    },
    onError: (error: unknown) => {
      appToast.error(
        isApiError(error) ? error.message : t("services.income.failed"),
      );
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("services.income.title")}</DialogTitle>
          <DialogDescription>
            {t("services.income.dialog_description", {
              no: service.service_no,
            })}
          </DialogDescription>
        </DialogHeader>
        {!open ? null : accounts.isLoading ? (
          <p className="text-muted-foreground text-sm">
            {t("services.income.loading")}
          </p>
        ) : (
          <ServiceIncomeForm
            service={service}
            accounts={accounts.data?.items ?? []}
            pending={save.isPending}
            onCancel={() => onOpenChange(false)}
            onSubmit={(input) => save.mutateAsync(input)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

/** "Reverse income": append-only reversal with an optional reason. */
export function ServiceIncomeReverseDialog({
  service,
  orgUuid,
  open,
  onOpenChange,
}: {
  service: Service;
  orgUuid: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const refresh = useRefreshService(service, orgUuid);
  const [reason, setReason] = useState("");
  const save = useMutation({
    mutationFn: () =>
      serviceWizardService.deleteIncome(service.uuid, reason.trim()),
    onSuccess: async (result) => {
      await refresh(result.service);
      appToast.success(t("services.income.reversed"));
      setReason("");
      onOpenChange(false);
    },
    onError: (error: unknown) => {
      appToast.error(
        isApiError(error) ? error.message : t("services.income.failed"),
      );
    },
  });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("services.income.reverse_title")}</DialogTitle>
          <DialogDescription>
            {t("services.income.reverse_description")}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-1.5">
          <Label htmlFor="service-income-reason">
            {t("services.income.reason")}
          </Label>
          <Textarea
            id="service-income-reason"
            rows={2}
            maxLength={1000}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={save.isPending}
            onClick={() => save.mutate()}
            data-testid="service-income-reverse-submit"
          >
            {t("services.income.reverse")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
