"use client";

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
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { FieldError } from "@/features/accounting/components/shared";
import type {
  Supplier,
  SupplierRequest,
} from "@/features/dealer-sales/services/dealer-sales.service";
import { normalizePhone } from "@/features/public-leads/lib/dealer-application";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

/** Default region of a national phone number (E.164 is stored). */
export const SUPPLIER_PHONE_REGION = "TR";

const EMAIL_PATTERN = /^[^@\s]+@[^@\s]+$/;

export type SupplierFormValues = {
  name: string;
  tax_no: string;
  phone: string;
  email: string;
  note: string;
  active: boolean;
};

export function supplierDefaults(s: Supplier | null): SupplierFormValues {
  return {
    name: s?.name ?? "",
    tax_no: s?.tax_no ?? "",
    phone: s?.phone_e164 ?? "",
    email: s?.email ?? "",
    note: s?.note ?? "",
    active: s?.active ?? true,
  };
}

type Translate = (key: string) => string;

/** Client checks before the request (Go validates again). */
export function validateSupplier(
  values: SupplierFormValues,
  t: Translate,
): { body: SupplierRequest | null; errors: Record<string, string> } {
  const errors: Record<string, string> = {};
  const name = values.name.trim();
  if (!name) errors.name = t("dealer_sales.validation.name");
  const phone = values.phone.trim()
    ? normalizePhone(values.phone, SUPPLIER_PHONE_REGION)
    : null;
  if (values.phone.trim() && !phone) {
    errors.phone_e164 = t("dealer_sales.validation.phone");
  }
  const email = values.email.trim();
  if (email && !EMAIL_PATTERN.test(email)) {
    errors.email = t("dealer_sales.validation.email");
  }
  if (Object.keys(errors).length > 0) return { body: null, errors };
  return {
    body: {
      name,
      tax_no: values.tax_no.trim() || null,
      phone_e164: phone,
      email: email || null,
      note: values.note.trim(),
      active: values.active,
    },
    errors,
  };
}

function SupplierForm({
  supplier,
  pending,
  onCancel,
  onSubmit,
}: {
  supplier: Supplier | null;
  pending?: boolean;
  onCancel: () => void;
  onSubmit: (body: SupplierRequest) => Promise<unknown>;
}) {
  const { t } = useLocale();
  const [values, setValues] = useState(() => supplierDefaults(supplier));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const set = <K extends keyof SupplierFormValues>(
    key: K,
    value: SupplierFormValues[K],
  ) => setValues((v) => ({ ...v, [key]: value }));

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const res = validateSupplier(values, t);
    setErrors(res.errors);
    if (!res.body) return;
    try {
      await onSubmit(res.body);
    } catch (error) {
      if (isApiError(error) && error.isValidation) {
        setErrors(error.fieldErrors());
      }
    }
  };

  const field = (
    key: "name" | "tax_no" | "email",
    errorKey: string,
    label: string,
    extra: { dir?: "ltr"; type?: string; maxLength: number },
  ) => (
    <div className="grid gap-1.5">
      <Label htmlFor={`supplier-${key}`}>{label}</Label>
      <Input
        id={`supplier-${key}`}
        name={key}
        value={values[key]}
        dir={extra.dir}
        type={extra.type}
        maxLength={extra.maxLength}
        aria-invalid={errors[errorKey] ? true : undefined}
        aria-describedby={
          errors[errorKey] ? `supplier-${key}-error` : undefined
        }
        onChange={(e) => set(key, e.target.value)}
      />
      <FieldError id={`supplier-${key}-error`} message={errors[errorKey]} />
    </div>
  );

  return (
    <form
      onSubmit={submit}
      noValidate
      className="space-y-4"
      data-testid="supplier-form"
    >
      {field("name", "name", t("dealer_sales.fields.name"), { maxLength: 200 })}
      {field("tax_no", "tax_no", t("dealer_sales.fields.tax_no"), {
        dir: "ltr",
        maxLength: 32,
      })}
      <div className="grid gap-1.5">
        <Label htmlFor="supplier-phone">{t("dealer_sales.fields.phone")}</Label>
        <Input
          id="supplier-phone"
          name="phone"
          type="tel"
          dir="ltr"
          value={values.phone}
          placeholder="+90 5XX XXX XX XX"
          aria-invalid={errors.phone_e164 ? true : undefined}
          aria-describedby={
            errors.phone_e164 ? "supplier-phone-error" : undefined
          }
          onChange={(e) => set("phone", e.target.value)}
          onBlur={() => {
            const e164 = normalizePhone(values.phone, SUPPLIER_PHONE_REGION);
            if (e164) set("phone", e164);
          }}
        />
        <FieldError id="supplier-phone-error" message={errors.phone_e164} />
      </div>
      {field("email", "email", t("dealer_sales.fields.email"), {
        dir: "ltr",
        type: "email",
        maxLength: 255,
      })}
      <div className="grid gap-1.5">
        <Label htmlFor="supplier-note">{t("dealer_sales.fields.note")}</Label>
        <Textarea
          id="supplier-note"
          value={values.note}
          maxLength={5000}
          rows={3}
          onChange={(e) => set("note", e.target.value)}
        />
      </div>
      {supplier ? (
        <div className="flex items-center justify-between gap-4">
          <Label htmlFor="supplier-active">
            {t("dealer_sales.fields.active")}
          </Label>
          <Switch
            id="supplier-active"
            checked={values.active}
            onCheckedChange={(v) => set("active", v)}
          />
        </div>
      ) : null}
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button
          type="submit"
          disabled={pending}
          data-testid="supplier-form-submit"
        >
          {t("common.save")}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function SupplierFormDialog({
  open,
  supplier,
  pending,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  supplier: Supplier | null;
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: SupplierRequest) => Promise<unknown>;
}) {
  const { t } = useLocale();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {supplier
              ? t("dealer_sales.suppliers.edit_title")
              : t("dealer_sales.suppliers.create_title")}
          </DialogTitle>
          <DialogDescription>
            {t("dealer_sales.suppliers.form_description")}
          </DialogDescription>
        </DialogHeader>
        {open ? (
          <SupplierForm
            key={supplier?.uuid ?? "new"}
            supplier={supplier}
            pending={pending}
            onCancel={() => onOpenChange(false)}
            onSubmit={onSubmit}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
