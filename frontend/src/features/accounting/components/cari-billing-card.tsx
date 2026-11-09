"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Receipt } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { accountingKeys } from "@/features/accounting/hooks/use-accounting-access";
import {
  accountingService,
  type CariBilling,
  type CariBillingInput,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Field = keyof Required<CariBillingInput>;

export const BILLING_FIELDS: Field[] = [
  "title",
  "tax_office",
  "tax_no",
  "address",
  "email",
  "iban",
];

/** Form values of a profile (null → ""). */
export function billingFormValues(b: CariBilling | undefined) {
  const out = {} as Record<Field, string>;
  for (const f of BILLING_FIELDS) out[f] = (b?.[f] as string | null) ?? "";
  return out;
}

/**
 * TEC-1070: billing details of a cari organization (legal title, tax
 * office / number, invoice address and e-mail, IBAN). The accounting of the
 * organization above fills them; a customer cari has none (422, hidden).
 */
export function CariBillingCard({
  orgUuid,
  cariUuid,
  canWrite,
}: {
  orgUuid: string;
  cariUuid: string;
  canWrite: boolean;
}) {
  const { t, format } = useLocale();
  const client = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [values, setValues] = useState<Record<Field, string>>(
    billingFormValues(undefined),
  );
  const [errors, setErrors] = useState<Partial<Record<Field, string>>>({});
  const query = useQuery({
    queryKey: accountingKeys.billing(orgUuid, cariUuid),
    queryFn: () => accountingService.getCariBilling(cariUuid),
    enabled: orgUuid !== "",
    retry: false,
  });
  const save = useMutation({
    mutationFn: () => accountingService.updateCariBilling(cariUuid, values),
    onSuccess: (data) => {
      client.setQueryData(accountingKeys.billing(orgUuid, cariUuid), data);
      appToast.success(t("accounting.billing.saved"));
      setEditing(false);
    },
    onError: (err) => {
      if (isApiError(err) && err.details.length) {
        const next: Partial<Record<Field, string>> = {};
        for (const d of err.details) {
          if (d.field && (BILLING_FIELDS as string[]).includes(d.field)) {
            next[d.field as Field] = t("accounting.billing.invalid");
          }
        }
        setErrors(next);
      }
      appToast.error(t("accounting.billing.save_failed"));
    },
  });

  if (query.isError && isApiError(query.error) && query.error.status === 422) {
    return null;
  }
  const b = query.data;
  const editable = canWrite && Boolean(b?.editable);

  return (
    <Card data-testid="cari-billing">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Receipt className="size-4" />
          {t("accounting.billing.title")}
        </CardTitle>
        {editable ? (
          <CardAction>
            <Button
              size="sm"
              variant="outline"
              data-testid="cari-billing-edit"
              onClick={() => {
                setValues(billingFormValues(b));
                setErrors({});
                setEditing(true);
              }}
            >
              <Pencil className="size-3.5" />
              {t("accounting.billing.edit")}
            </Button>
          </CardAction>
        ) : null}
      </CardHeader>
      <CardContent>
        {query.isPending ? (
          <Skeleton className="h-24" />
        ) : query.isError ? (
          <p className="text-destructive text-sm">
            {t("accounting.load_failed")}
          </p>
        ) : (
          <>
            <dl className="grid gap-3 text-sm sm:grid-cols-2">
              {BILLING_FIELDS.map((f) => (
                <div
                  key={f}
                  className={f === "address" ? "sm:col-span-2" : undefined}
                >
                  <dt className="text-muted-foreground text-xs">
                    {t(`accounting.billing.fields.${f}`)}
                  </dt>
                  <dd
                    className="font-medium break-words whitespace-pre-line"
                    dir={f === "iban" || f === "email" ? "ltr" : undefined}
                    data-testid={`cari-billing-${f}`}
                  >
                    {(b?.[f] as string | null) || "—"}
                  </dd>
                </div>
              ))}
            </dl>
            {b?.updated_at ? (
              <p className="text-muted-foreground mt-3 text-xs">
                {t("accounting.billing.updated", {
                  at: format.dateTime(b.updated_at),
                  by: b.updated_by ?? "—",
                })}
              </p>
            ) : null}
          </>
        )}
      </CardContent>
      <Dialog open={editing} onOpenChange={setEditing}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("accounting.billing.title")}</DialogTitle>
          </DialogHeader>
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate();
            }}
          >
            {BILLING_FIELDS.map((f) => (
              <div key={f} className="space-y-1">
                <Label htmlFor={`billing-${f}`}>
                  {t(`accounting.billing.fields.${f}`)}
                </Label>
                {f === "address" ? (
                  <Textarea
                    id={`billing-${f}`}
                    value={values[f]}
                    maxLength={2000}
                    onChange={(e) =>
                      setValues((v) => ({ ...v, [f]: e.target.value }))
                    }
                  />
                ) : (
                  <Input
                    id={`billing-${f}`}
                    value={values[f]}
                    type={f === "email" ? "email" : "text"}
                    dir={f === "iban" || f === "email" ? "ltr" : undefined}
                    aria-invalid={errors[f] ? true : undefined}
                    onChange={(e) =>
                      setValues((v) => ({ ...v, [f]: e.target.value }))
                    }
                  />
                )}
                {errors[f] ? (
                  <p className="text-destructive text-xs">{errors[f]}</p>
                ) : null}
              </div>
            ))}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setEditing(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                disabled={save.isPending}
                data-testid="cari-billing-save"
              >
                {t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
