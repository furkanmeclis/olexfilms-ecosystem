"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
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
  newIdempotencyKey,
} from "@/features/accounting/components/shared";
import { accountingKeys } from "@/features/accounting/hooks/use-accounting-access";
import {
  fieldErrors,
  manualEntryFormSchema,
  manualEntryInput,
  type ManualEntryFormValues,
} from "@/features/accounting/lib/form";
import {
  accountingService,
  MANUAL_DIRECTIONS,
  type AccountingCategory,
  type CariAccount,
  type FinanceAccount,
  type FinanceEntryInput,
  type ManualDirection,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type ManualEntryFormProps = {
  accounts: FinanceAccount[];
  cari: CariAccount[];
  categories: AccountingCategory[];
  idempotencyKey: string;
  pending?: boolean;
  onCancel: () => void;
  onSubmit: (input: FinanceEntryInput) => Promise<unknown>;
};

/**
 * Manual income, expense or cari charge (POST /v1/accounting/entries): a
 * manual category of the direction, an amount in the book currency, a cash
 * or bank account and/or a cari (a customer cari included, TEC-342). A
 * charge books the cari only, so the account field is hidden.
 */
export function ManualEntryForm({
  accounts,
  cari,
  categories,
  idempotencyKey,
  pending,
  onCancel,
  onSubmit,
}: ManualEntryFormProps) {
  const { t } = useLocale();
  const active = accounts.filter((a) => a.active);
  const [values, setValues] = useState<ManualEntryFormValues>({
    direction: "income",
    category: "",
    amount: "",
    account_uuid: active.length === 1 ? active[0]!.uuid : "",
    cari_uuid: "",
    description: "",
  });
  const [errors, setErrors] = useState<Record<string, string>>({});

  const set = <K extends keyof ManualEntryFormValues>(
    key: K,
    value: ManualEntryFormValues[K],
  ) => setValues((v) => ({ ...v, [key]: value }));

  const categoryOptions = categories
    .filter((c) => c.manual && c.direction === values.direction)
    .map((c) => ({ value: c.key, label: c.label }));
  const charge = values.direction === "charge";

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const parsed = manualEntryFormSchema(t).safeParse(values);
    if (!parsed.success) {
      setErrors(fieldErrors(parsed.error));
      return;
    }
    setErrors({});
    try {
      await onSubmit(manualEntryInput(parsed.data, idempotencyKey));
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
      data-testid="manual-entry-form"
    >
      <NativeSelect
        id="manual-entry-direction"
        name="direction"
        label={t("accounting.fields.direction")}
        value={values.direction}
        onChange={(v) =>
          setValues((prev) => ({
            ...prev,
            direction: v as ManualDirection,
            category: "",
          }))
        }
        options={MANUAL_DIRECTIONS.map((d) => ({
          value: d,
          label: t(`accounting.directions.${d}`),
        }))}
      />
      <NativeSelect
        id="manual-entry-category"
        name="category"
        label={t("accounting.fields.category")}
        value={values.category}
        placeholder={t("accounting.manual.pick_category")}
        error={errors.category}
        onChange={(v) => set("category", v)}
        options={categoryOptions}
      />
      <div className="grid gap-1.5">
        <Label htmlFor="manual-entry-amount">
          {t("accounting.fields.amount")}
        </Label>
        <Input
          id="manual-entry-amount"
          name="amount"
          inputMode="decimal"
          dir="ltr"
          autoComplete="off"
          className="text-end tabular-nums"
          value={values.amount}
          aria-invalid={errors.amount ? true : undefined}
          aria-describedby={
            errors.amount ? "manual-entry-amount-error" : undefined
          }
          onChange={(e) => set("amount", e.target.value)}
        />
        <FieldError id="manual-entry-amount-error" message={errors.amount} />
      </div>
      {charge ? null : (
        <NativeSelect
          id="manual-entry-account"
          name="account_uuid"
          label={t("accounting.fields.account")}
          value={values.account_uuid}
          placeholder={t("accounting.manual.no_account")}
          error={errors.account_uuid}
          onChange={(v) => set("account_uuid", v)}
          options={active.map((a) => ({
            value: a.uuid,
            label: `${a.name} (${a.currency})`,
          }))}
        />
      )}
      <NativeSelect
        id="manual-entry-cari"
        name="cari_uuid"
        label={t("accounting.fields.cari")}
        value={values.cari_uuid}
        placeholder={
          charge
            ? t("accounting.settlement.pick_cari")
            : t("accounting.manual.no_cari")
        }
        error={errors.cari_uuid}
        onChange={(v) => set("cari_uuid", v)}
        options={cari.map((c) => ({
          value: c.uuid,
          label: c.counterparty.name,
        }))}
      />
      <div className="grid gap-1.5">
        <Label htmlFor="manual-entry-description">
          {t("accounting.fields.description")}
        </Label>
        <Textarea
          id="manual-entry-description"
          name="description"
          rows={3}
          maxLength={1000}
          value={values.description}
          aria-invalid={errors.description ? true : undefined}
          onChange={(e) => set("description", e.target.value)}
        />
        <FieldError
          id="manual-entry-description-error"
          message={errors.description}
        />
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button
          type="submit"
          disabled={pending}
          data-testid="manual-entry-submit"
        >
          {t("accounting.manual.submit")}
        </Button>
      </DialogFooter>
    </form>
  );
}

/**
 * Dialog wrapper: loads the active accounts, the active cari list and the
 * categories, and books the entry in the active book.
 */
export function ManualEntryDialog({
  orgUuid,
  open,
  onOpenChange,
}: {
  orgUuid: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  // One key per opened dialog: a retried submit books once.
  const [key, setKey] = useState(newIdempotencyKey);
  const enabled = open && Boolean(orgUuid);

  const accounts = useQuery({
    queryKey: accountingKeys.accounts(orgUuid, { active: true }),
    queryFn: () => accountingService.listAccounts({ active: true }),
    enabled,
  });
  const cari = useQuery({
    queryKey: accountingKeys.cariList(orgUuid, { active: true, limit: 100 }),
    queryFn: () => accountingService.listCari({ active: true, limit: 100 }),
    enabled,
  });
  const categories = useQuery({
    queryKey: accountingKeys.categories(orgUuid),
    queryFn: () => accountingService.listCategories(),
    enabled,
    staleTime: 10 * 60 * 1000,
  });

  const save = useMutation({
    mutationFn: (input: FinanceEntryInput) =>
      accountingService.createEntry(input),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: accountingKeys.all(orgUuid),
      });
      appToast.success(t("accounting.manual.done"));
      setKey(newIdempotencyKey());
      onOpenChange(false);
    },
    onError: (error: unknown) => {
      appToast.error(
        isApiError(error) ? error.message : t("accounting.toast.failed"),
      );
    },
  });

  const loading = accounts.isLoading || cari.isLoading || categories.isLoading;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("accounting.manual.title")}</DialogTitle>
          <DialogDescription>
            {t("accounting.manual.description")}
          </DialogDescription>
        </DialogHeader>
        {!open ? null : loading ? (
          <p className="text-muted-foreground text-sm">
            {t("accounting.loading")}
          </p>
        ) : (
          <ManualEntryForm
            key={key}
            accounts={accounts.data?.items ?? []}
            cari={cari.data?.items ?? []}
            categories={categories.data?.items ?? []}
            idempotencyKey={key}
            pending={save.isPending}
            onCancel={() => onOpenChange(false)}
            onSubmit={(input) => save.mutateAsync(input)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
