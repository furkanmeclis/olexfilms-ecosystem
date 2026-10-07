"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback, useRef, useState, type FormEvent } from "react";

import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { FieldError } from "@/features/accounting/components/shared";
import { accountingKeys } from "@/features/accounting/hooks/use-accounting-access";
import {
  accountingService,
  type CariAccount,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Customer cari (TEC-342): search a customer the organization serves and
 * open its cari (POST /v1/accounting/cari answers the existing one when it
 * is already there). The caller then lands on the cari detail page with
 * its statement, collections and charges.
 */
export function CustomerCariDialog({
  orgUuid,
  open,
  onOpenChange,
  onOpened,
}: {
  orgUuid: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onOpened: (cari: CariAccount) => void;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const [customer, setCustomer] = useState<ComboboxOption | null>(null);
  const [error, setError] = useState<string | undefined>();
  const seen = useRef(new Map<string, ComboboxOption>());

  const loadCustomers = useCallback(async (q: string) => {
    const page = await accountingService.searchCustomers(q.trim());
    const options = page.items.map((c) => ({
      value: c.uuid,
      label: [c.name, c.surname].filter(Boolean).join(" ") || c.uuid,
      description: c.phone ?? c.email ?? undefined,
    }));
    for (const o of options) seen.current.set(o.value, o);
    return options;
  }, []);

  const save = useMutation({
    mutationFn: (uuid: string) => accountingService.openCustomerCari(uuid),
    onSuccess: async (cari) => {
      await queryClient.invalidateQueries({
        queryKey: accountingKeys.all(orgUuid),
      });
      appToast.success(t("accounting.customer_cari.done"));
      setCustomer(null);
      onOpenChange(false);
      onOpened(cari);
    },
    onError: (err: unknown) => {
      appToast.error(
        isApiError(err) && err.status === 404
          ? t("accounting.customer_cari.not_served")
          : isApiError(err)
            ? err.message
            : t("accounting.toast.failed"),
      );
    },
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!customer) {
      setError(t("accounting.validation.customer"));
      return;
    }
    setError(undefined);
    save.mutate(customer.value);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("accounting.customer_cari.title")}</DialogTitle>
          <DialogDescription>
            {t("accounting.customer_cari.description")}
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={submit}
          noValidate
          className="space-y-4"
          data-testid="customer-cari-form"
        >
          <div className="grid gap-1.5">
            <Label htmlFor="customer-cari-customer">
              {t("accounting.customer_cari.customer")}
            </Label>
            <AsyncCombobox
              id="customer-cari-customer"
              className="w-full"
              value={customer?.value ?? ""}
              initialOptions={customer ? [customer] : undefined}
              loadOptions={loadCustomers}
              onValueChange={(value) =>
                setCustomer(
                  value
                    ? (seen.current.get(value) ?? { value, label: value })
                    : null,
                )
              }
              placeholder={t("accounting.customer_cari.placeholder")}
              searchPlaceholder={t("accounting.customer_cari.search")}
              emptyText={t("accounting.customer_cari.none")}
            />
            <FieldError id="customer-cari-customer-error" message={error} />
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
              type="submit"
              disabled={save.isPending}
              data-testid="customer-cari-submit"
            >
              {t("accounting.customer_cari.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
